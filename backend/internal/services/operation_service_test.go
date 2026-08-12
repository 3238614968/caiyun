//go:build cgo

package services

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type operationQueueStub struct {
	messages   []*queue.TaskMessage
	enqueueErr error
}

type operationEventPublisherStub struct {
	userIDs []uint
	updates []models.OperationUpdate
}

func (p *operationEventPublisherStub) PublishOperationUpdated(_ context.Context, userID uint, update models.OperationUpdate) {
	p.userIDs = append(p.userIDs, userID)
	p.updates = append(p.updates, update)
}

func (q *operationQueueStub) Enqueue(a, u uint, t string) error {
	return q.EnqueueMessage(&queue.TaskMessage{AccountID: a, UserID: u, TaskType: t})
}
func (q *operationQueueStub) EnqueueMessage(m *queue.TaskMessage) error {
	if q.enqueueErr != nil {
		return q.enqueueErr
	}
	copy := *m
	q.messages = append(q.messages, &copy)
	return nil
}
func (*operationQueueStub) EnqueueBatch([]*models.Account, string) error { return nil }
func (*operationQueueStub) Dequeue(time.Duration) (*queue.TaskMessage, error) {
	return nil, queue.ErrQueueTimeout
}
func (*operationQueueStub) Ack(*queue.TaskMessage) error                           { return nil }
func (*operationQueueStub) Requeue(*queue.TaskMessage) error                       { return nil }
func (*operationQueueStub) RequeueDelayed(*queue.TaskMessage, time.Duration) error { return nil }
func (*operationQueueStub) RenewVisibility(*queue.TaskMessage) (bool, error)       { return true, nil }
func (*operationQueueStub) DeadLetter(*queue.TaskMessage, string) error            { return nil }
func (*operationQueueStub) RecoverStaleProcessing(time.Duration) (int, error)      { return 0, nil }
func (*operationQueueStub) PromoteDueDelayed(int64) (int, error)                   { return 0, nil }
func (*operationQueueStub) GetQueueLength() (int64, error)                         { return 0, nil }
func (*operationQueueStub) GetProcessingLength() (int64, error)                    { return 0, nil }
func (*operationQueueStub) GetDelayedLength() (int64, error)                       { return 0, nil }
func (*operationQueueStub) GetDeadLetterLength() (int64, error)                    { return 0, nil }
func (*operationQueueStub) Clear() error                                           { return nil }

func newOperationServiceTest(t *testing.T) (*OperationService, *operationQueueStub, *gorm.DB) {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", uuid.NewString())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Operation{}); err != nil {
		t.Fatal(err)
	}
	q := &operationQueueStub{}
	service := NewOperationService(repository.NewOperationRepository(db), q)
	return service, q, db
}

func TestOperationSubmitIsDurableAndIdempotent(t *testing.T) {
	service, q, db := newOperationServiceTest(t)
	ctx := context.Background()
	req := SubmitOperationRequest{UserID: 7, OperationType: models.OperationTypeAccountTask, AccountID: 11, Payload: AccountTaskOperationPayload{AccountID: 11, TaskType: "all_tasks"}, IdempotencyKey: "request-1"}
	first, created, dispatchErr, err := service.Submit(ctx, req)
	if err != nil || dispatchErr != nil || !created {
		t.Fatalf("first submit created=%t dispatch=%v err=%v", created, dispatchErr, err)
	}
	if first.Status != models.OperationQueued || len(q.messages) != 1 || q.messages[0].OperationID != first.ID {
		t.Fatalf("operation=%+v messages=%+v", first, q.messages)
	}
	second, created, _, err := service.Submit(ctx, req)
	if err != nil || created || second.ID != first.ID {
		t.Fatalf("second submit created=%t op=%+v err=%v", created, second, err)
	}
	var count int64
	if err := db.Model(&models.Operation{}).Count(&count).Error; err != nil || count != 1 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	conflict := req
	conflict.AccountID = 12
	conflict.Payload = AccountTaskOperationPayload{AccountID: 12, TaskType: "all_tasks"}
	if _, _, _, err := service.Submit(ctx, conflict); !errors.Is(err, ErrOperationIdempotencyConflict) {
		t.Fatalf("conflict err=%v", err)
	}
}

func TestOperationLifecycleUsesCompareAndSwap(t *testing.T) {
	service, _, _ := newOperationServiceTest(t)
	ctx := context.Background()
	op, _, _, err := service.Submit(ctx, SubmitOperationRequest{UserID: 9, OperationType: models.OperationTypeExchangeTask, ResourceID: 5, Payload: ExchangeTaskOperationPayload{TaskID: 5}, IdempotencyKey: "lifecycle"})
	if err != nil {
		t.Fatal(err)
	}
	running, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || running.Status != models.OperationRunning || running.AttemptCount != 1 {
		t.Fatalf("running=%+v claimed=%t err=%v", running, claimed, err)
	}
	_, claimed, err = service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || claimed {
		t.Fatalf("duplicate claim=%t err=%v", claimed, err)
	}
	if err := service.MarkSucceeded(ctx, op.ID, running.ExecutionToken); err != nil {
		t.Fatal(err)
	}
	finished, err := service.GetForUser(ctx, op.ID, 9)
	if err != nil || finished.Status != models.OperationSucceeded || finished.CompletedAt == nil {
		t.Fatalf("finished=%+v err=%v", finished, err)
	}
	if _, err := service.Cancel(ctx, op.ID, 9); !errors.Is(err, ErrOperationNotCancelable) {
		t.Fatalf("cancel terminal err=%v", err)
	}
}

func TestOperationLifecyclePublishesSafeStateTransitions(t *testing.T) {
	service, _, _ := newOperationServiceTest(t)
	publisher := &operationEventPublisherStub{}
	service.SetEventPublisher(publisher)
	ctx := context.Background()

	op, created, _, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         21,
		OperationType:  models.OperationTypeExchangeMonthly,
		Payload:        struct{}{},
		IdempotencyKey: "event-lifecycle",
	})
	if err != nil || !created {
		t.Fatalf("submit created=%t err=%v", created, err)
	}
	running, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("claim=%t operation=%+v err=%v", claimed, running, err)
	}
	if err := service.MarkFailed(ctx, op.ID, running.ExecutionToken, errors.New("upstream secret details")); err != nil {
		t.Fatal(err)
	}

	if len(publisher.updates) != 3 {
		t.Fatalf("event count=%d, want queued/running/failed", len(publisher.updates))
	}
	statuses := []models.OperationStatus{models.OperationQueued, models.OperationRunning, models.OperationFailed}
	for index, want := range statuses {
		got := publisher.updates[index]
		if publisher.userIDs[index] != 21 || got.OperationID != op.ID || got.Status != want {
			t.Fatalf("event[%d]=user:%d update:%+v", index, publisher.userIDs[index], got)
		}
	}
	failed := publisher.updates[2]
	if failed.ErrorSummary != "操作执行失败，请稍后重试" {
		t.Fatalf("unsafe error leaked in event: %q", failed.ErrorSummary)
	}
}

func TestOperationLifecycleFencingTokenRejectsStaleWorker(t *testing.T) {
	service, _, _ := newOperationServiceTest(t)
	ctx := context.Background()
	now := time.Date(2026, 7, 24, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	op, _, _, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         11,
		OperationType:  models.OperationTypeAccountTask,
		AccountID:      12,
		Payload:        AccountTaskOperationPayload{AccountID: 12, TaskType: "all_tasks"},
		IdempotencyKey: "fencing-token",
	})
	if err != nil {
		t.Fatal(err)
	}
	first, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("first claim = (%+v, %t, %v)", first, claimed, err)
	}
	now = now.Add(2 * time.Minute)
	second, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || second.ExecutionToken == first.ExecutionToken {
		t.Fatalf("second claim = (%+v, %t, %v)", second, claimed, err)
	}
	if err := service.MarkSucceeded(ctx, op.ID, first.ExecutionToken); !errors.Is(err, repository.ErrOperationExecutionLost) {
		t.Fatalf("stale completion error = %v, want ErrOperationExecutionLost", err)
	}
	running, err := service.Get(ctx, op.ID)
	if err != nil || running.Status != models.OperationRunning || running.ExecutionToken != second.ExecutionToken {
		t.Fatalf("stale Worker overwrote operation: %+v err=%v", running, err)
	}
	if err := service.MarkSucceeded(ctx, op.ID, second.ExecutionToken); err != nil {
		t.Fatalf("new Worker completion: %v", err)
	}
}

func TestOperationOutboxRedispatchesAfterInitialQueueFailure(t *testing.T) {
	service, q, _ := newOperationServiceTest(t)
	q.enqueueErr = errors.New("redis unavailable")
	op, created, dispatchErr, err := service.Submit(context.Background(), SubmitOperationRequest{UserID: 3, OperationType: models.OperationTypeExchangeMonthly, Payload: struct{}{}, IdempotencyKey: "outbox"})
	if err != nil || !created || dispatchErr == nil || op.Status != models.OperationQueued {
		t.Fatalf("op=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}
	q.enqueueErr = nil
	service.now = func() time.Time { return time.Date(2100, time.January, 1, 0, 0, 0, 0, time.UTC) }
	count, err := service.RedispatchQueued(context.Background(), 0, 10)
	if err != nil || count != 1 || len(q.messages) != 1 {
		t.Fatalf("count=%d messages=%d err=%v", count, len(q.messages), err)
	}
}

func TestOperationAttemptBudgetSurvivesRedispatchAndStaleRecovery(t *testing.T) {
	service, q, _ := newOperationServiceTest(t)
	ctx := context.Background()
	now := time.Date(2026, time.August, 5, 12, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }

	op, created, dispatchErr, err := service.Submit(ctx, SubmitOperationRequest{
		UserID:         66,
		OperationType:  models.OperationTypeAccountTask,
		AccountID:      67,
		Payload:        AccountTaskOperationPayload{AccountID: 67, TaskType: "all_tasks"},
		IdempotencyKey: "durable-attempt-budget",
	})
	if err != nil || dispatchErr != nil || !created {
		t.Fatalf("Submit() operation=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}

	first, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || first.AttemptCount != 1 {
		t.Fatalf("first claim operation=%+v claimed=%t err=%v", first, claimed, err)
	}
	if err := service.MarkRetryQueued(ctx, op.ID, first.ExecutionToken, errors.New("transient upstream error")); err != nil {
		t.Fatalf("MarkRetryQueued(): %v", err)
	}

	// The durable AttemptCount remains authoritative and is mirrored into the
	// fresh outbox delivery for backoff and observability.
	now = now.Add(time.Second)
	dispatched, err := service.RedispatchQueued(ctx, 0, 10)
	if err != nil || dispatched != 1 || len(q.messages) != 2 {
		t.Fatalf("RedispatchQueued() dispatched=%d messages=%d err=%v", dispatched, len(q.messages), err)
	}
	if q.messages[1].RetryCount != 1 {
		t.Fatalf("redispatch RetryCount=%d, want durable attempt 1", q.messages[1].RetryCount)
	}

	second, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || second.AttemptCount != 2 {
		t.Fatalf("second claim operation=%+v claimed=%t err=%v", second, claimed, err)
	}

	// Simulate a process dying with no remaining Redis PEL entry.  The durable
	// stale-running sweep requeues it without resetting the historical budget.
	now = now.Add(2 * time.Minute)
	recovered, err := service.RecoverStaleRunning(ctx, time.Minute, 10)
	if err != nil || recovered != 1 {
		t.Fatalf("RecoverStaleRunning() recovered=%d err=%v", recovered, err)
	}
	recoveredOperation, err := service.Get(ctx, op.ID)
	if err != nil || recoveredOperation.Status != models.OperationQueued || recoveredOperation.AttemptCount != 2 || recoveredOperation.ExecutionToken != "" {
		t.Fatalf("stale recovery operation=%+v err=%v", recoveredOperation, err)
	}

	dispatched, err = service.RedispatchQueued(ctx, 0, 10)
	if err != nil || dispatched != 1 || len(q.messages) != 3 {
		t.Fatalf("second RedispatchQueued() dispatched=%d messages=%d err=%v", dispatched, len(q.messages), err)
	}
	if q.messages[2].RetryCount != 2 {
		t.Fatalf("recovered redispatch RetryCount=%d, want durable attempt 2", q.messages[2].RetryCount)
	}
	third, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed || third.AttemptCount != 3 {
		t.Fatalf("third claim operation=%+v claimed=%t err=%v", third, claimed, err)
	}
}

func TestOperationDeadLetterReplayOnlyRequeuesFailedOperation(t *testing.T) {
	service, q, _ := newOperationServiceTest(t)
	ctx := context.Background()
	now := time.Date(2026, time.July, 26, 10, 0, 0, 0, time.UTC)
	service.now = func() time.Time { return now }
	op, _, _, err := service.Submit(ctx, SubmitOperationRequest{
		UserID: 31, OperationType: models.OperationTypeAccountTask, AccountID: 32,
		Payload: AccountTaskOperationPayload{AccountID: 32, TaskType: "all_tasks"}, IdempotencyKey: "dead-letter-replay",
	})
	if err != nil {
		t.Fatal(err)
	}
	running, claimed, err := service.TryStart(ctx, op.ID, time.Minute)
	if err != nil || !claimed {
		t.Fatalf("TryStart() operation=%+v claimed=%t err=%v", running, claimed, err)
	}
	if err := service.MarkFailed(ctx, op.ID, running.ExecutionToken, errors.New("upstream failed")); err != nil {
		t.Fatal(err)
	}
	now = now.Add(time.Second)
	replayed, err := service.ReplayFailed(ctx, op.ID)
	if err != nil || replayed.Status != models.OperationQueued || replayed.ExecutionToken != "" || replayed.CompletedAt != nil {
		t.Fatalf("ReplayFailed() operation=%+v err=%v", replayed, err)
	}
	if len(q.messages) != 2 || q.messages[1].OperationID != op.ID {
		t.Fatalf("queue messages=%+v, want replay dispatch", q.messages)
	}
	if q.messages[0].IdempotencyKey == q.messages[1].IdempotencyKey {
		t.Fatalf("replay must use a fresh durable queue epoch: first=%q replay=%q", q.messages[0].IdempotencyKey, q.messages[1].IdempotencyKey)
	}
	if _, err := service.ReplayFailed(ctx, op.ID); !errors.Is(err, ErrOperationNotReplayable) {
		t.Fatalf("second ReplayFailed() error=%v, want ErrOperationNotReplayable", err)
	}
}
