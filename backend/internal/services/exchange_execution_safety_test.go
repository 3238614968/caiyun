//go:build cgo

package services

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type availableExchangeLocks struct{}

func (availableExchangeLocks) SetNX(string, interface{}, time.Duration) (bool, error) {
	return true, nil
}
func (availableExchangeLocks) DelIfValue(string, string) (bool, error) { return true, nil }

func pendingLongTermTask(t *testing.T) (*repository.ExchangeTaskRepository, *models.ExchangeTask, *gorm.DB) {
	t.Helper()
	repo, task, db := newExchangeSubmissionTestRepository(t)
	account := &models.ExchangeAccount{ID: task.ExchangeAccountID, UserID: task.UserID, AccountID: 20, IsActive: true}
	if err := db.Create(account).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(task).Updates(map[string]interface{}{
		"task_type": string(models.ExchangeTaskLongTerm), "status": string(models.ExchangeTaskPending), "execution_token": "",
	}).Error; err != nil {
		t.Fatal(err)
	}
	task, err := repo.GetByID(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	return repo, task, db
}

func simulatedWarmSession(t *testing.T, reply string, posts *atomic.Int32, before func(*http.Request)) *exchangePreparedSession {
	t.Helper()
	server := newMockExchangeTLSServer(t, http.StatusOK, reply)
	t.Cleanup(server.Close)
	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")
	session := newMockExchangeSession(t, server)
	session.loggedIn = true
	session.client.Transport = &exchangeCountingTransport{base: session.client.Transport, posts: posts, before: before}
	return &exchangePreparedSession{auth: &exchangeAuthContext{jwtToken: "local-session"}, http: session}
}

const acceptedTestResponse = `{"msg":"success","code":0,"result":{"oid":"local-order","prizeId":12345}}`

func TestMonthlyPolicyRecheckedBeforeSubmission(t *testing.T) {
	for _, test := range []struct {
		name, disableKey string
		scheduled        bool
	}{
		{"automatic switch", "exchange_monthly_enabled", true},
		{"global switch", "exchange_enabled", false},
		{"config unavailable", "", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, _, db := newExchangeSubmissionTestRepository(t)
			configs := repository.NewSystemConfigRepository(db)
			if err := configs.Set("exchange_monthly_enabled", "true"); err != nil {
				t.Fatal(err)
			}
			service := &ExchangeService{configRepo: configs}
			guard := service.monthlySubmissionGuard(test.scheduled)
			if err := guard(t.Context()); err != nil {
				t.Fatal(err)
			}
			var posts atomic.Int32
			prepared := simulatedWarmSession(t, acceptedTestResponse, &posts, func(req *http.Request) {
				if req.URL.Path != "/ycloud/signin/page/receivePrizeDetails" {
					return
				}
				if test.disableKey == "" {
					if err := db.Migrator().DropTable(&models.SystemConfig{}); err != nil {
						t.Fatal(err)
					}
				} else if err := configs.Set(test.disableKey, "false"); err != nil {
					t.Fatal(err)
				}
			})
			result := executeExchangeOnce("12345", prepared.auth, prepared.http, guard)
			if result.guardErr == nil || result.outcome != models.ExchangeOutcomeNotSubmitted || posts.Load() != 0 {
				t.Fatalf("policy changed before submission: %+v posts=%d", result, posts.Load())
			}
		})
	}
}

func TestSchedulerPauseAfterWarmupPreventsSubmission(t *testing.T) {
	for _, stage := range []string{"before dispatch", "before submission", "config unavailable at submission"} {
		t.Run(stage, func(t *testing.T) {
			repo, task, db := pendingLongTermTask(t)
			configs := repository.NewSystemConfigRepository(db)
			if err := configs.Set("exchange_enabled", "true"); err != nil {
				t.Fatal(err)
			}
			scheduler := NewExchangeScheduler(repo, repository.NewExchangeAccountRepository(db), nil, nil, configs, nil, nil)
			scheduler.SetLeaseStore(availableExchangeLocks{})
			defer scheduler.Stop()
			var posts atomic.Int32
			var preflight bool
			session := simulatedWarmSession(t, acceptedTestResponse, &posts, func(req *http.Request) {
				if req.URL.Path != "/ycloud/signin/page/receivePrizeDetails" {
					return
				}
				preflight = true
				if stage == "config unavailable at submission" {
					if err := db.Migrator().DropTable(&models.SystemConfig{}); err != nil {
						t.Fatal(err)
					}
				} else if err := configs.Set("exchange_enabled", "false"); err != nil {
					t.Fatal(err)
				}
			})
			scheduler.storeWarmSession(task.ID, task.ExchangeAccount.AccountID, session)
			if stage == "before dispatch" {
				if err := configs.Set("exchange_enabled", "false"); err != nil {
					t.Fatal(err)
				}
				scheduler.preparedQueue = []*models.ExchangeTask{task}
				var taskQueries int
				if err := db.Callback().Query().Before("gorm:query").Register("test:count-task-queries", func(query *gorm.DB) {
					if query.Statement.Table == "exchange_tasks" {
						taskQueries++
					}
				}); err != nil {
					t.Fatal(err)
				}
				slot := time.Now().In(cstZone).Add(-time.Minute)
				scheduler.prepareQueueForSlot(slot)
				scheduler.executeExchangeForSlot(slot)
				scheduler.executionWG.Wait()
				if taskQueries != 0 {
					t.Fatalf("disabled warmup/dispatch queried exchange tasks %d times", taskQueries)
				}
			} else {
				result := scheduler.executeTask(t.Context(), task)
				if !preflight || !result.skipped || result.guardErr == nil || result.persistenceErr != nil {
					t.Fatalf("did not stop at the submission boundary: %+v", result)
				}
			}
			current, err := repo.GetByID(task.ID)
			if err != nil || current.Status != "pending" || current.Outcome != "not_submitted" || current.ExecutionToken != "" || current.AttemptedCount != 0 || posts.Load() != 0 || len(current.Records) != 0 {
				t.Fatalf("paused task consumed an attempt: %+v posts=%d err=%v", current, posts.Load(), err)
			}
		})
	}
}

func TestLongTermTaskAtomicOperationReceipt(t *testing.T) {
	for _, test := range []struct {
		name, reply string
		wantErr     error
		failReceipt bool
	}{
		{name: "accepted", reply: acceptedTestResponse},
		{name: "rejected", reply: `{"msg":"sold out"}`, wantErr: ErrExchangeExecutionFailed},
		{name: "unknown", reply: `{}`, wantErr: ErrExchangeOutcomeUnknown},
		{name: "receipt write failure", reply: acceptedTestResponse, failReceipt: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			repo, task, db := pendingLongTermTask(t)
			if err := db.AutoMigrate(&repository.ExchangeExecutionReceipt{}); err != nil {
				t.Fatal(err)
			}
			receipts := repository.NewExchangeReceiptRepository(db)
			service := NewExchangeService(nil, repository.NewExchangeAccountRepository(db), repo, nil, nil, nil, nil, nil, nil)
			service.SetReceiptRepository(receipts)
			service.SetLockStore(availableExchangeLocks{})
			operationContext := func() (context.Context, string) {
				operationID := uuid.NewString()
				if err := db.Create(&models.Operation{ID: operationID, IdempotencyKey: operationID, UserID: task.UserID, OperationType: models.OperationTypeExchangeTask}).Error; err != nil {
					t.Fatal(err)
				}
				return WithTaskOperation(t.Context(), operationID, 1), operationID
			}
			ctx, operationID := operationContext()
			if test.failReceipt {
				if err := db.Exec(`CREATE TRIGGER fail_receipt BEFORE INSERT ON exchange_execution_receipts BEGIN SELECT RAISE(ABORT, 'receipt unavailable'); END`).Error; err != nil {
					t.Fatal(err)
				}
			}
			var posts atomic.Int32
			session := simulatedWarmSession(t, test.reply, &posts, nil)
			execErr := service.executeTaskWithSession(ctx, task, session)
			current, err := repo.GetByID(task.ID)
			if err != nil || posts.Load() != 1 {
				t.Fatalf("first submission: posts=%d execErr=%v readErr=%v", posts.Load(), execErr, err)
			}
			receipt, err := receipts.FindByOperationResource(operationID, repository.ExchangeTaskReceiptKind, task.ID)
			if err != nil {
				t.Fatal(err)
			}
			if test.failReceipt {
				if execErr == nil || !strings.Contains(execErr.Error(), "receipt unavailable") || receipt != nil || current.AttemptedCount != 0 || current.SuccessCount != 0 || current.Status != "running" || current.Outcome != "unknown" || current.ExecutionToken == "" || len(current.Records) != 0 {
					t.Fatalf("partial settlement after receipt failure: task=%+v receipt=%+v err=%v", current, receipt, execErr)
				}
				if err := service.ExecuteExchangeTaskContext(ctx, task.ID, task.UserID); !errors.Is(err, ErrExchangeTaskConflict) || posts.Load() != 1 {
					t.Fatalf("recovery settled a live lease: posts=%d err=%v", posts.Load(), err)
				}
				expireExchangeTestLease(t, db, task)
				if err := service.ExecuteExchangeTaskContext(ctx, task.ID, task.UserID); err == nil || !strings.Contains(err.Error(), "receipt unavailable") || posts.Load() != 1 {
					t.Fatalf("recovery ignored receipt failure: posts=%d err=%v", posts.Load(), err)
				}
				if err := db.Exec(`DROP TRIGGER fail_receipt`).Error; err != nil {
					t.Fatal(err)
				}
				if err := service.ExecuteExchangeTaskContext(ctx, task.ID, task.UserID); !errors.Is(err, ErrExchangeOutcomeUnknown) || posts.Load() != 1 {
					t.Fatalf("ambiguous submission was replayed: posts=%d err=%v", posts.Load(), err)
				}
				receipt, err = receipts.FindByOperationResource(operationID, repository.ExchangeTaskReceiptKind, task.ID)
				if err != nil || receipt == nil || receipt.CompletedAt == nil || receipt.Outcome != "unknown" {
					t.Fatalf("missing recovery receipt: %+v err=%v", receipt, err)
				}
				return
			}
			if !errors.Is(execErr, test.wantErr) || receipt == nil || receipt.CompletedAt == nil || current.AttemptedCount != 1 || len(current.Records) != 1 || receipt.Outcome != current.Records[0].Outcome || receipt.Outcome != test.name {
				t.Fatalf("inconsistent settlement: task=%+v receipt=%+v err=%v", current, receipt, execErr)
			}
			if err := service.executeTaskWithSession(ctx, current, session); !errors.Is(err, test.wantErr) || posts.Load() != 1 {
				t.Fatalf("redelivery: posts=%d err=%v", posts.Load(), err)
			}
			if test.name == "accepted" {
				if started, _, err := repo.TryMarkForManualExecution(current.ID, current.UserID, current.Status, current.UpdatedAt, operationID); err != nil || started {
					t.Fatalf("overlapping delivery claimed completed operation: started=%t err=%v", started, err)
				}
				nextCtx, _ := operationContext()
				if err := service.executeTaskWithSession(nextCtx, current, session); err != nil || posts.Load() != 2 {
					t.Fatalf("independent operation blocked: posts=%d err=%v", posts.Load(), err)
				}
			}
		})
	}
}
