package repository

import (
	"context"
	"errors"
	"time"

	"caiyun/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrOperationExecutionLost = errors.New("operation execution ownership lost")

// OperationRepository persists asynchronous commands and owns all lifecycle
// compare-and-swap transitions used to make at-least-once queue delivery safe.
type OperationRepository struct {
	db *gorm.DB
}

func NewOperationRepository(db *gorm.DB) *OperationRepository {
	return &OperationRepository{db: db}
}

func (r *OperationRepository) WithContext(ctx context.Context) *OperationRepository {
	if ctx == nil {
		return r
	}
	return &OperationRepository{db: r.db.WithContext(ctx)}
}

// CreateOrGetByIdempotency returns created=false when the user already
// submitted the same idempotency key. A unique database constraint resolves
// races between API replicas; the second read handles the concurrent winner.
func (r *OperationRepository) CreateOrGetByIdempotency(operation *models.Operation) (*models.Operation, bool, error) {
	if operation == nil {
		return nil, false, errors.New("operation is nil")
	}
	var existing models.Operation
	err := r.db.Where("user_id = ? AND idempotency_key = ?", operation.UserID, operation.IdempotencyKey).First(&existing).Error
	if err == nil {
		return &existing, false, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, err
	}

	if err := r.db.Create(operation).Error; err == nil {
		return operation, true, nil
	}
	if err := r.db.Where("user_id = ? AND idempotency_key = ?", operation.UserID, operation.IdempotencyKey).First(&existing).Error; err != nil {
		return nil, false, err
	}
	return &existing, false, nil
}

func (r *OperationRepository) GetByID(id string) (*models.Operation, error) {
	var operation models.Operation
	if err := r.db.First(&operation, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &operation, nil
}

func (r *OperationRepository) GetByIDForUser(id string, userID uint) (*models.Operation, error) {
	var operation models.Operation
	if err := r.db.Where("id = ? AND user_id = ?", id, userID).First(&operation).Error; err != nil {
		return nil, err
	}
	return &operation, nil
}

// TryMarkRunning claims a queued command or a running command whose Worker
// lease has expired. The returned fencing token must accompany every later
// lifecycle write from that Worker.
func (r *OperationRepository) TryMarkRunning(id string, staleBefore, now time.Time) (bool, string, error) {
	executionToken := uuid.NewString()
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND (status = ? OR (status = ? AND updated_at < ?))",
			id, models.OperationQueued, models.OperationRunning, staleBefore).
		Updates(map[string]interface{}{
			"status":          models.OperationRunning,
			"execution_token": executionToken,
			"started_at":      now,
			"completed_at":    nil,
			"error_summary":   "",
			"attempt_count":   gorm.Expr("attempt_count + 1"),
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected != 1 {
		return false, "", nil
	}
	return true, executionToken, nil
}

func (r *OperationRepository) MarkQueued(id, executionToken, errorSummary string, now time.Time) error {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, models.OperationRunning, executionToken).
		Updates(map[string]interface{}{
			"status":          models.OperationQueued,
			"execution_token": "",
			"queued_at":       now,
			"error_summary":   errorSummary,
			"updated_at":      now,
		})
	return operationOwnershipResult(result)
}

func (r *OperationRepository) MarkSucceeded(id, executionToken string, now time.Time) error {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, models.OperationRunning, executionToken).
		Updates(map[string]interface{}{
			"status":          models.OperationSucceeded,
			"execution_token": "",
			"completed_at":    now,
			"error_summary":   "",
			"updated_at":      now,
		})
	return operationOwnershipResult(result)
}

func (r *OperationRepository) MarkFailed(id, executionToken, errorSummary string, now time.Time) error {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, models.OperationRunning, executionToken).
		Updates(map[string]interface{}{
			"status":          models.OperationFailed,
			"execution_token": "",
			"completed_at":    now,
			"error_summary":   errorSummary,
			"updated_at":      now,
		})
	return operationOwnershipResult(result)
}

// ReplayFailed returns a terminal failed command to the durable queued state.
// It is intentionally restricted to failed rows and clears the previous lease,
// so an administrator cannot resurrect a canceled/succeeded command or let an
// old Worker write through after manual dead-letter replay.
func (r *OperationRepository) ReplayFailed(id string, now time.Time) (bool, error) {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ?", id, models.OperationFailed).
		Updates(map[string]interface{}{
			"status":          models.OperationQueued,
			"execution_token": "",
			"queued_at":       now,
			"started_at":      nil,
			"completed_at":    nil,
			"attempt_count":   0,
			"error_summary":   "manually replayed from dead letter",
			"updated_at":      now,
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// RequeueStaleRunning is the database-side complement to queue PEL recovery.
// It uses one fenced transition per candidate so a concurrent RenewRunning or
// successful completion wins the race and is never overwritten.  The limit
// bounds one maintenance tick without relying on database-specific UPDATE
// LIMIT syntax.
func (r *OperationRepository) RequeueStaleRunning(staleBefore, now time.Time, limit int) (int, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var candidates []models.Operation
	if err := r.db.Where("status = ? AND updated_at < ?", models.OperationRunning, staleBefore).
		Order("updated_at ASC").Limit(limit).Find(&candidates).Error; err != nil {
		return 0, err
	}

	recovered := 0
	for _, candidate := range candidates {
		result := r.db.Model(&models.Operation{}).
			Where("id = ? AND status = ? AND updated_at < ?", candidate.ID, models.OperationRunning, staleBefore).
			Updates(map[string]interface{}{
				"status":          models.OperationQueued,
				"execution_token": "",
				"queued_at":       now,
				"started_at":      nil,
				"completed_at":    nil,
				"error_summary":   "worker lease expired; queued for recovery",
				"updated_at":      now,
			})
		if result.Error != nil {
			return recovered, result.Error
		}
		if result.RowsAffected == 1 {
			recovered++
		}
	}
	return recovered, nil
}

func (r *OperationRepository) RenewRunning(id, executionToken string, now time.Time) (bool, error) {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, models.OperationRunning, executionToken).
		Update("updated_at", now)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

func (r *OperationRepository) Cancel(id string, userID uint, now time.Time) (bool, error) {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND user_id = ? AND status = ?", id, userID, models.OperationQueued).
		Updates(map[string]interface{}{
			"status":       models.OperationCanceled,
			"completed_at": now,
			"updated_at":   now,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *OperationRepository) SetResourceID(id, executionToken string, resourceID uint) error {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ? AND execution_token = ? AND resource_id = 0",
			id, models.OperationRunning, executionToken).
		Update("resource_id", resourceID)
	return operationOwnershipResult(result)
}

func (r *OperationRepository) ListQueuedBefore(before time.Time, limit int) ([]*models.Operation, error) {
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	var operations []*models.Operation
	err := r.db.Where("status = ? AND updated_at <= ?", models.OperationQueued, before).
		Order("updated_at ASC").Limit(limit).Find(&operations).Error
	return operations, err
}

func operationOwnershipResult(result *gorm.DB) error {
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrOperationExecutionLost
	}
	return nil
}
