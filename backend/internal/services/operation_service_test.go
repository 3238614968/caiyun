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
	if err := service.MarkSucceeded(ctx, op.ID); err != nil {
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

func TestOperationOutboxRedispatchesAfterInitialQueueFailure(t *testing.T) {
	service, q, _ := newOperationServiceTest(t)
	q.enqueueErr = errors.New("redis unavailable")
	op, created, dispatchErr, err := service.Submit(context.Background(), SubmitOperationRequest{UserID: 3, OperationType: models.OperationTypeExchangeMonthly, Payload: struct{}{}, IdempotencyKey: "outbox"})
	if err != nil || !created || dispatchErr == nil || op.Status != models.OperationQueued {
		t.Fatalf("op=%+v created=%t dispatch=%v err=%v", op, created, dispatchErr, err)
	}
	q.enqueueErr = nil
	service.now = func() time.Time { return op.QueuedAt.Add(time.Minute) }
	count, err := service.RedispatchQueued(context.Background(), 0, 10)
	if err != nil || count != 1 || len(q.messages) != 1 {
		t.Fatalf("count=%d messages=%d err=%v", count, len(q.messages), err)
	}
}
