//go:build cgo
// +build cgo

package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type exchangeCountingTransport struct {
	base    http.RoundTripper
	posts   *atomic.Int32
	before  func(*http.Request)
	postErr error
}

func (t *exchangeCountingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.before != nil {
		t.before(req)
	}
	if req.URL.Path == "/ycloud/signin/page/exchangeV3" {
		t.posts.Add(1)
		if t.postErr != nil {
			return nil, t.postErr
		}
	}
	return t.base.RoundTrip(req)
}

func newExchangeSubmissionTestRepository(t *testing.T) (*repository.ExchangeTaskRepository, *models.ExchangeTask, *gorm.DB) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:exchange-submission-"+strings.ReplaceAll(t.Name(), "/", "-")+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.ExchangeTask{}, &models.ExchangeRecord{}, &models.SystemConfig{}); err != nil {
		t.Fatal(err)
	}
	task := &models.ExchangeTask{UserID: 7, ExchangeAccountID: 8, ProductID: 9, PrizeID: "12345", PrizeName: "mock", Status: string(models.ExchangeTaskRunning), ExecutionToken: "owner", Outcome: string(models.ExchangeOutcomeNotSubmitted)}
	if err := db.Create(task).Error; err != nil {
		t.Fatal(err)
	}
	return repository.NewExchangeTaskRepository(db), task, db
}

func TestSubmissionGuardFailurePreventsUpstreamRequest(t *testing.T) {
	var posts atomic.Int32
	server := newMockExchangeTLSServer(t, 200, `{"msg":"success","code":0,"result":{"oid":"order","prizeId":12345}}`)
	defer server.Close()
	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")
	session := newMockExchangeSession(t, server)
	target, _ := url.Parse(server.URL)
	base := session.client.Transport.(*exchangeRewriteRoundTripper).base
	session.client.Transport = &exchangeCountingTransport{base: &exchangeRewriteRoundTripper{base: base, targetURL: target}, posts: &posts}
	result := executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture"}, session, func(context.Context) error {
		return errors.New("db unavailable")
	})
	if result.outcome != models.ExchangeOutcomeNotSubmitted || !strings.Contains(result.message, "未提交兑换") {
		t.Fatalf("guard failure = %+v", result)
	}
	if posts.Load() != 0 {
		t.Fatalf("guard failure sent %d upstream submissions", posts.Load())
	}
}

func TestSubmittedFailuresAreUnknownAndNotRetried(t *testing.T) {
	for _, tc := range []struct {
		name string
		body string
		code int
	}{
		{name: "malformed json", body: "not-json", code: 200},
		{name: "http error", body: `{"msg":"busy"}`, code: 500},
		{name: "missing receipt", body: `{"msg":"success","result":{}}`, code: 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := newMockExchangeTLSServer(t, tc.code, tc.body)
			defer server.Close()
			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			t.Setenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY", "true")
			calls := 0
			var posts atomic.Int32
			guard := func(context.Context) error { return nil }
			result, _, err := runExchangeWithRetries(context.Background(), 3, nil, func() exchangeAttemptResult {
				calls++
				session := newMockExchangeSession(t, server)
				target, _ := url.Parse(server.URL)
				base := session.client.Transport.(*exchangeRewriteRoundTripper).base
				session.client.Transport = &exchangeCountingTransport{base: &exchangeRewriteRoundTripper{base: base, targetURL: target}, posts: &posts}
				return executeExchangeOnce("12345", &exchangeAuthContext{jwtToken: "fixture"}, session, guard)
			})
			if err != nil || calls != 1 || posts.Load() != 1 || result.outcome != models.ExchangeOutcomeUnknown || !strings.Contains(result.message, "兑换结果待确认") {
				t.Fatalf("result=%+v calls=%d posts=%d err=%v", result, calls, posts.Load(), err)
			}
		})
	}
}

func TestExchangeRetryDoesNotSubmitAgainAfterCancellation(t *testing.T) {
	repo, task, db := newExchangeSubmissionTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	guard := newExchangeSubmissionGuard(repo, task.ID, task.ExecutionToken)
	result, _, err := runExchangeWithRetries(ctx, 3, nil, func() exchangeAttemptResult {
		if err := guard(ctx); err != nil {
			t.Fatal(err)
		}
		cancel()
		return unknownExchangeResult("connection lost", 0)
	})
	if err != nil || result.outcome != models.ExchangeOutcomeUnknown {
		t.Fatalf("canceled result = %+v err=%v", result, err)
	}
	if err := repo.SettleOwnedUnknown(task.ID, task.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	var settled models.ExchangeTask
	if err := db.First(&settled, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if settled.Status != string(models.ExchangeTaskFailed) || settled.Outcome != string(models.ExchangeOutcomeUnknown) || settled.AttemptedCount != 1 {
		t.Fatalf("settled task = %+v", settled)
	}
	if err := repo.MarkSubmissionOwned(task.ID, task.ExecutionToken); !errors.Is(err, repository.ErrExchangeTaskExecutionLost) {
		t.Fatalf("second submission allowed: %v", err)
	}
}

func TestExchangeRedeliveryLeavesActiveOwnerInControl(t *testing.T) {
	repo, task, _ := newExchangeSubmissionTestRepository(t)
	if err := repo.MarkSubmissionOwned(task.ID, task.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	service := NewExchangeService(nil, nil, repo, nil, nil, nil, nil, nil, nil)
	if err := service.ExecuteExchangeTaskContext(t.Context(), task.ID, task.UserID); !errors.Is(err, ErrExchangeTaskConflict) {
		t.Fatalf("active redelivery = %v", err)
	}
	current, err := repo.GetByID(task.ID)
	if err != nil || current.ExecutionToken != task.ExecutionToken || current.AttemptedCount != 0 || len(current.Records) != 0 {
		t.Fatalf("redelivery changed the active task: %+v, %v", current, err)
	}
	settlement := repository.ExchangeSettlement{Outcome: models.ExchangeOutcomeAccepted, Status: models.ExchangeTaskPending, Message: "accepted"}
	if err := repo.FinalizeOwned(task, task.ExecutionToken, settlement); err != nil {
		t.Fatal(err)
	}
	current, err = repo.GetByID(task.ID)
	if err != nil || current.SuccessCount != 1 || current.AttemptedCount != 1 || len(current.Records) != 1 || current.Outcome != "accepted" {
		t.Fatalf("owner could not save the actual result: %+v, %v", current, err)
	}
}

func expireExchangeTestLease(t *testing.T, db *gorm.DB, task *models.ExchangeTask) {
	t.Helper()
	if err := db.Model(task).UpdateColumn("updated_at", time.Now().Add(-exchangeTaskRunningTimeoutFromEnv()-time.Minute)).Error; err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryRejectsStaleSnapshotAfterRenewal(t *testing.T) {
	repo, task, db := newExchangeSubmissionTestRepository(t)
	if err := repo.MarkSubmissionOwned(task.ID, task.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	expireExchangeTestLease(t, db, task)
	snapshot, err := repo.GetByID(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	cutoff := time.Now().Add(-exchangeTaskRunningTimeoutFromEnv())
	if renewed, err := repo.RenewRunning(task.ID, task.ExecutionToken, time.Now()); err != nil || !renewed {
		t.Fatalf("renewed=%v err=%v", renewed, err)
	}
	if err := repo.FinalizeStaleSubmission(snapshot, cutoff, ""); !errors.Is(err, repository.ErrExchangeTaskExecutionLost) {
		t.Fatalf("stale snapshot settlement = %v", err)
	}
	current, err := repo.GetByID(task.ID)
	if err != nil || current.Status != "running" || current.ExecutionToken != task.ExecutionToken || current.AttemptedCount != 0 || len(current.Records) != 0 {
		t.Fatalf("renewed lease was settled: %+v err=%v", current, err)
	}
}

func TestExchangeUnknownOperationRedeliveryOnlySettlesTask(t *testing.T) {
	repo, task, db := newExchangeSubmissionTestRepository(t)
	if err := repo.MarkSubmissionOwned(task.ID, task.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	expireExchangeTestLease(t, db, task)
	service := NewExchangeService(nil, nil, repo, nil, nil, nil, nil, nil, nil)
	if err := service.ExecuteExchangeTaskContext(context.Background(), task.ID, task.UserID); !errors.Is(err, ErrExchangeOutcomeUnknown) {
		t.Fatalf("redelivery error = %v", err)
	}
	var current models.ExchangeTask
	if err := db.First(&current, task.ID).Error; err != nil {
		t.Fatal(err)
	}
	if current.Status != string(models.ExchangeTaskFailed) || current.Outcome != string(models.ExchangeOutcomeUnknown) || current.AttemptedCount != 1 {
		t.Fatalf("redelivered task = %+v", current)
	}
	var records []models.ExchangeRecord
	if err := db.Find(&records).Error; err != nil || len(records) != 1 || records[0].Outcome != string(models.ExchangeOutcomeUnknown) {
		t.Fatalf("redelivery records = %+v err=%v", records, err)
	}
}

func TestBatchExecutionReturnsNoOutcomeForUnownedOrActiveTasks(t *testing.T) {
	repo, task, db := newExchangeSubmissionTestRepository(t)
	if err := repo.MarkSubmissionOwned(task.ID, task.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	service := NewExchangeService(nil, nil, repo, nil, repository.NewSystemConfigRepository(db), nil, nil, nil, nil)
	for _, userID := range []uint{task.UserID + 1, task.UserID} {
		results := service.BatchExecuteExchangeTasksContext(t.Context(), []uint{task.ID}, userID)
		if len(results) != 1 || results[0].Success || results[0].Outcome != "" || results[0].Message == "" {
			t.Fatalf("unexpected batch result: %+v", results)
		}
	}
	current, err := repo.GetByID(task.ID)
	if err != nil || current.ExecutionToken != task.ExecutionToken || current.AttemptedCount != 0 || len(current.Records) != 0 {
		t.Fatalf("batch changed the active task: %+v, %v", current, err)
	}
}

func TestFinalPostDisconnectWithoutGuardIsNotRetried(t *testing.T) {
	var posts atomic.Int32
	session := simulatedWarmSession(t, acceptedTestResponse, &posts, nil)
	session.http.client.Transport.(*exchangeCountingTransport).postErr = io.ErrUnexpectedEOF
	result, _, err := runExchangeWithRetries(t.Context(), 3, nil, func() exchangeAttemptResult {
		return executeExchangeOnce("12345", session.auth, session.http)
	})
	if err != nil || result.success || result.outcome != models.ExchangeOutcomeUnknown || posts.Load() != 1 {
		t.Fatalf("ambiguous transport result=%+v posts=%d err=%v", result, posts.Load(), err)
	}
}

func TestExchangeManualUnknownSettlementHasTerminalPublicError(t *testing.T) {
	err := newExchangeOutcomeUnknownError("request may have been submitted")
	if !errors.Is(err, ErrExchangeOutcomeUnknown) || !isTerminalOperationErrorForTest(err) {
		t.Fatalf("unknown was not terminal: %v", err)
	}
}

func isTerminalOperationErrorForTest(err error) bool {
	return errors.Is(err, ErrExchangeOutcomeUnknown)
}

func TestExchangeLeaseHeartbeatStopsAtContext(t *testing.T) {
	repo, task, _ := newExchangeSubmissionTestRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	stop := startExchangeLeaseHeartbeat(ctx, repo, task.ID, task.ExecutionToken, time.Millisecond)
	cancel()
	if stop() {
		t.Fatal("healthy heartbeat was reported lost")
	}
}
