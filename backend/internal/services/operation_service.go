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
	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/internal/repository"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var (
	ErrOperationNotFound            = errors.New("operation not found")
	ErrOperationIdempotencyConflict = errors.New("idempotency key already used by a different operation")
	ErrOperationNotCancelable       = errors.New("operation is not queued and cannot be canceled")
	ErrOperationNotReplayable       = errors.New("operation is not failed and cannot be replayed")
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
	repo           *repository.OperationRepository
	taskQueue      queue.ReliableTaskQueue
	eventPublisher OperationEventPublisher
	metrics        *monitor.Metrics
	now            func() time.Time
}

func NewOperationService(repo *repository.OperationRepository, taskQueue queue.ReliableTaskQueue) *OperationService {
	return &OperationService{repo: repo, taskQueue: taskQueue, now: time.Now}
}

// SetEventPublisher wires an optional transport adapter at process startup.
// Publishing is best-effort: the persisted Operation row remains the source of
// truth and SSE replay can compensate for a transient transport failure.
func (s *OperationService) SetEventPublisher(publisher OperationEventPublisher) {
	if s == nil {
		return
	}
	s.eventPublisher = publisher
}

// SetMetrics attaches the process-local Prometheus collector at composition
// time. The same durable transitions are observed by API and Worker metrics.
func (s *OperationService) SetMetrics(metrics *monitor.Metrics) {
	if s == nil {
		return
	}
	s.metrics = metrics
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
	if created {
		s.recordOperationTransition(operation.Status)
		s.publishOperationUpdated(ctx, operation)
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
	operation, err := s.GetForUser(ctx, id, userID)
	if err == nil {
		s.recordOperationTransition(operation.Status)
		s.publishOperationUpdated(ctx, operation)
	}
	return operation, err
}

func (s *OperationService) TryStart(ctx context.Context, id string, leaseTimeout time.Duration) (*models.Operation, bool, error) {
	if leaseTimeout <= 0 {
		leaseTimeout = queue.DefaultVisibilityDelay
	}
	now := s.now().UTC()
	claimed, executionToken, err := s.repo.WithContext(ctx).TryMarkRunning(strings.TrimSpace(id), now.Add(-leaseTimeout), now)
	if err != nil {
		return nil, false, err
	}
	operation, err := s.Get(ctx, id)
	if err == nil && claimed && operation.ExecutionToken != executionToken {
		return nil, false, repository.ErrOperationExecutionLost
	}
	if err == nil && claimed {
		s.recordOperationTransition(operation.Status)
		s.publishOperationUpdated(ctx, operation)
	}
	return operation, claimed, err
}

func (s *OperationService) MarkRetryQueued(ctx context.Context, id, executionToken string, cause error) error {
	if err := s.repo.WithContext(ctx).MarkQueued(strings.TrimSpace(id), strings.TrimSpace(executionToken), operationErrorSummary(cause), s.now().UTC()); err != nil {
		return err
	}
	s.publishOperationByID(ctx, id)
	s.recordOperationTransition(models.OperationQueued)
	return nil
}

func (s *OperationService) MarkSucceeded(ctx context.Context, id, executionToken string) error {
	if err := s.repo.WithContext(ctx).MarkSucceeded(strings.TrimSpace(id), strings.TrimSpace(executionToken), s.now().UTC()); err != nil {
		return err
	}
	s.publishOperationByID(ctx, id)
	s.recordOperationTransition(models.OperationSucceeded)
	return nil
}

func (s *OperationService) MarkFailed(ctx context.Context, id, executionToken string, cause error) error {
	if err := s.repo.WithContext(ctx).MarkFailed(strings.TrimSpace(id), strings.TrimSpace(executionToken), operationErrorSummary(cause), s.now().UTC()); err != nil {
		return err
	}
	s.publishOperationByID(ctx, id)
	s.recordOperationTransition(models.OperationFailed)
	return nil
}

// ReplayFailed is the approval-gated dead-letter transition. MySQL changes
// first; if the subsequent Redis publish fails, the regular queued-operation
// outbox reconciler safely redelivers it later.
func (s *OperationService) ReplayFailed(ctx context.Context, id string) (*models.Operation, error) {
	if s == nil || s.repo == nil || s.taskQueue == nil {
		return nil, errors.New("operation service is not configured")
	}
	replayed, err := s.repo.WithContext(ctx).ReplayFailed(strings.TrimSpace(id), s.now().UTC())
	if err != nil {
		return nil, err
	}
	if !replayed {
		return nil, ErrOperationNotReplayable
	}
	operation, err := s.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	// The row is already durable and queued. Returning the dispatch error makes
	// the partial result observable while RedispatchQueued preserves delivery.
	if err := s.enqueue(operation); err != nil {
		return operation, fmt.Errorf("redispatch replayed operation: %w", err)
	}
	s.recordOperationTransition(models.OperationQueued)
	s.publishOperationUpdated(ctx, operation)
	return operation, nil
}

func (s *OperationService) RenewLease(ctx context.Context, id, executionToken string) (bool, error) {
	return s.repo.WithContext(ctx).RenewRunning(strings.TrimSpace(id), strings.TrimSpace(executionToken), s.now().UTC())
}

func (s *OperationService) SetResourceID(ctx context.Context, id, executionToken string, resourceID uint) error {
	if resourceID == 0 {
		return errors.New("resource id is required")
	}
	if err := s.repo.WithContext(ctx).SetResourceID(strings.TrimSpace(id), strings.TrimSpace(executionToken), resourceID); err != nil {
		return err
	}
	s.publishOperationByID(ctx, id)
	return nil
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

// RecoverStaleRunning turns commands whose fencing lease has expired back
// into the durable queued state.  This is a second recovery path in addition
// to Redis PEL recovery: it closes the gap where a previous delivery was
// accidentally deleted or Redis lost pending state while the Worker crashed.
// The repository transition is conditional on the stale lease, so a healthy
// Worker renewing updated_at cannot be displaced.
func (s *OperationService) RecoverStaleRunning(ctx context.Context, leaseTimeout time.Duration, limit int) (int, error) {
	if s == nil || s.repo == nil {
		return 0, errors.New("operation service is not configured")
	}
	if leaseTimeout <= 0 {
		leaseTimeout = queue.DefaultVisibilityDelay
	}
	now := s.now().UTC()
	recovered, err := s.repo.WithContext(ctx).RequeueStaleRunning(now.Add(-leaseTimeout), now, limit)
	if err != nil {
		return 0, err
	}
	if recovered > 0 {
		s.recordOperationTransition(models.OperationQueued)
	}
	return recovered, nil
}

func (s *OperationService) enqueue(operation *models.Operation) error {
	if operation == nil {
		return errors.New("operation is nil")
	}
	return s.taskQueue.EnqueueMessage(&queue.TaskMessage{
		AccountID:     operation.AccountID,
		UserID:        operation.UserID,
		TaskType:      operation.OperationType,
		OperationID:   operation.ID,
		OperationType: operation.OperationType,
		CreatedAt:     operation.QueuedAt.Unix(),
		RetryCount:    operation.AttemptCount,
		// Include the durable queued_at epoch. Each failed -> queued transition
		// gets one delivery while repeated outbox scans for the same transition
		// remain deduplicated by Redis.
		IdempotencyKey: fmt.Sprintf("operation:%s:%d", operation.ID, operation.QueuedAt.UTC().UnixNano()),
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

func (s *OperationService) publishOperationByID(ctx context.Context, id string) {
	operation, err := s.Get(ctx, id)
	if err != nil {
		log.Printf("load operation for event failed: operation_id=%s err=%v", strings.TrimSpace(id), err)
		return
	}
	s.publishOperationUpdated(ctx, operation)
}

func (s *OperationService) publishOperationUpdated(ctx context.Context, operation *models.Operation) {
	if s == nil || s.eventPublisher == nil || operation == nil || operation.UserID == 0 {
		return
	}
	s.eventPublisher.PublishOperationUpdated(ctx, operation.UserID, operationUpdate(operation))
}

func (s *OperationService) recordOperationTransition(status models.OperationStatus) {
	if s == nil || s.metrics == nil {
		return
	}
	s.metrics.RecordOperationTransition(string(status))
}
