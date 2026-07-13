package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
	"unicode/utf8"

	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrOperationNotFound            = errors.New("operation not found")
	ErrOperationIdempotencyConflict = errors.New("idempotency key already used by a different operation")
	ErrOperationNotCancelable       = errors.New("operation is not queued and cannot be canceled")
)

const maxOperationErrorSummaryBytes = 512

type AccountTaskOperationPayload struct {
	AccountID uint   `json:"account_id"`
	TaskType  string `json:"task_type"`
}

type AccountTaskBatchOperationPayload struct {
	AccountIDs []uint `json:"account_ids"`
}

type ExchangeTaskOperationPayload struct {
	TaskID uint `json:"task_id"`
}

type ExchangeTaskBatchOperationPayload struct {
	TaskIDs []uint `json:"task_ids"`
}

type ImmediateExchangeOperationPayload struct {
	ExchangeRuleID uint `json:"exchange_rule_id,omitempty"`
	AccountID      uint `json:"account_id,omitempty"`
	ProductID      uint `json:"product_id"`
}

type SubmitOperationRequest struct {
	UserID         uint
	OperationType  string
	AccountID      uint
	ResourceID     uint
	Payload        interface{}
	IdempotencyKey string
}

// OperationService couples the durable database outbox row with the reliable
// Redis queue. A queued row is authoritative: if the immediate Redis dispatch
// fails, the Worker reconciliation loop republishes it later.
type OperationService struct {
	repo      *repository.OperationRepository
	taskQueue queue.ReliableTaskQueue
	now       func() time.Time
}

func NewOperationService(repo *repository.OperationRepository, taskQueue queue.ReliableTaskQueue) *OperationService {
	return &OperationService{repo: repo, taskQueue: taskQueue, now: time.Now}
}

// Submit persists the command before publishing it. dispatchErr is returned
// separately because the operation remains durably queued and is still safe to
// acknowledge with HTTP 202; callers should log it for operational visibility.
func (s *OperationService) Submit(ctx context.Context, request SubmitOperationRequest) (operation *models.Operation, created bool, dispatchErr error, err error) {
	if s == nil || s.repo == nil || s.taskQueue == nil {
		return nil, false, nil, errors.New("operation service is not configured")
	}
	if request.UserID == 0 {
		return nil, false, nil, errors.New("operation user id is required")
	}
	request.OperationType = strings.TrimSpace(request.OperationType)
	if request.OperationType == "" {
		return nil, false, nil, errors.New("operation type is required")
	}
	payload, err := json.Marshal(request.Payload)
	if err != nil {
		return nil, false, nil, fmt.Errorf("encode operation payload: %w", err)
	}
	idempotencyKey := strings.TrimSpace(request.IdempotencyKey)
	if idempotencyKey == "" {
		idempotencyKey = uuid.NewString()
	}
	if len(idempotencyKey) > 191 {
		return nil, false, nil, errors.New("idempotency key is too long")
	}
	now := s.now().UTC()
	candidate := &models.Operation{
		ID:             uuid.NewString(),
		UserID:         request.UserID,
		OperationType:  request.OperationType,
		Status:         models.OperationQueued,
		AccountID:      request.AccountID,
		ResourceID:     request.ResourceID,
		Payload:        string(payload),
		IdempotencyKey: idempotencyKey,
		QueuedAt:       now,
	}
	repo := s.repo.WithContext(ctx)
	operation, created, err = repo.CreateOrGetByIdempotency(candidate)
	if err != nil {
		return nil, false, nil, fmt.Errorf("persist operation: %w", err)
	}
	if operation.OperationType != candidate.OperationType || operation.Payload != candidate.Payload || operation.AccountID != candidate.AccountID {
		return nil, false, nil, ErrOperationIdempotencyConflict
	}
	if operation.Status == models.OperationQueued {
		dispatchErr = s.enqueue(operation)
	}
	return operation, created, dispatchErr, nil
}

func (s *OperationService) GetForUser(ctx context.Context, id string, userID uint) (*models.Operation, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("operation service is not configured")
	}
	operation, err := s.repo.WithContext(ctx).GetByIDForUser(strings.TrimSpace(id), userID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOperationNotFound
	}
	return operation, err
}

func (s *OperationService) Get(ctx context.Context, id string) (*models.Operation, error) {
	if s == nil || s.repo == nil {
		return nil, errors.New("operation service is not configured")
	}
	operation, err := s.repo.WithContext(ctx).GetByID(strings.TrimSpace(id))
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, ErrOperationNotFound
	}
	return operation, err
}

func (s *OperationService) Cancel(ctx context.Context, id string, userID uint) (*models.Operation, error) {
	if _, err := s.GetForUser(ctx, id, userID); err != nil {
		return nil, err
	}
	canceled, err := s.repo.WithContext(ctx).Cancel(strings.TrimSpace(id), userID, s.now().UTC())
	if err != nil {
		return nil, err
	}
	if !canceled {
		return nil, ErrOperationNotCancelable
	}
	return s.GetForUser(ctx, id, userID)
}

func (s *OperationService) TryStart(ctx context.Context, id string, leaseTimeout time.Duration) (*models.Operation, bool, error) {
	if leaseTimeout <= 0 {
		leaseTimeout = queue.DefaultVisibilityDelay
	}
	now := s.now().UTC()
	claimed, err := s.repo.WithContext(ctx).TryMarkRunning(strings.TrimSpace(id), now.Add(-leaseTimeout), now)
	if err != nil {
		return nil, false, err
	}
	operation, err := s.Get(ctx, id)
	return operation, claimed, err
}

func (s *OperationService) MarkRetryQueued(ctx context.Context, id string, cause error) error {
	return s.repo.WithContext(ctx).MarkQueued(strings.TrimSpace(id), operationErrorSummary(cause), s.now().UTC())
}

func (s *OperationService) MarkSucceeded(ctx context.Context, id string) error {
	return s.repo.WithContext(ctx).MarkSucceeded(strings.TrimSpace(id), s.now().UTC())
}

func (s *OperationService) MarkFailed(ctx context.Context, id string, cause error) error {
	return s.repo.WithContext(ctx).MarkFailed(strings.TrimSpace(id), operationErrorSummary(cause), s.now().UTC())
}

func (s *OperationService) SetResourceID(ctx context.Context, id string, resourceID uint) error {
	if resourceID == 0 {
		return errors.New("resource id is required")
	}
	return s.repo.WithContext(ctx).SetResourceID(strings.TrimSpace(id), resourceID)
}

// RedispatchQueued is the transactional-outbox reconciler. It makes database
// commit + Redis publish eventually reliable without running business work in
// the API process.
func (s *OperationService) RedispatchQueued(ctx context.Context, olderThan time.Duration, limit int) (int, error) {
	if olderThan < 0 {
		olderThan = 0
	}
	operations, err := s.repo.WithContext(ctx).ListQueuedBefore(s.now().UTC().Add(-olderThan), limit)
	if err != nil {
		return 0, err
	}
	dispatched := 0
	for _, operation := range operations {
		if err := s.enqueue(operation); err != nil {
			log.Printf("operation outbox redispatch failed: operation_id=%s err=%v", operation.ID, err)
			continue
		}
		dispatched++
	}
	return dispatched, nil
}

func (s *OperationService) enqueue(operation *models.Operation) error {
	if operation == nil {
		return errors.New("operation is nil")
	}
	return s.taskQueue.EnqueueMessage(&queue.TaskMessage{
		AccountID:      operation.AccountID,
		UserID:         operation.UserID,
		TaskType:       operation.OperationType,
		OperationID:    operation.ID,
		OperationType:  operation.OperationType,
		CreatedAt:      operation.QueuedAt.Unix(),
		IdempotencyKey: "operation:" + operation.ID,
	})
}

func operationErrorSummary(err error) string {
	if err == nil {
		return ""
	}
	summary := strings.TrimSpace(err.Error())
	if len(summary) <= maxOperationErrorSummaryBytes {
		return summary
	}
	data := []byte(summary)
	data = data[:maxOperationErrorSummaryBytes]
	for len(data) > 0 && !utf8.Valid(data) {
		data = data[:len(data)-1]
	}
	return string(data)
}
