package repository

import (
	"context"
	"errors"
	"time"

	"caiyun/internal/models"

	"gorm.io/gorm"
)

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
// lease has expired. RowsAffected is the sole ownership decision.
func (r *OperationRepository) TryMarkRunning(id string, staleBefore, now time.Time) (bool, error) {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND (status = ? OR (status = ? AND updated_at < ?))",
			id, models.OperationQueued, models.OperationRunning, staleBefore).
		Updates(map[string]interface{}{
			"status":        models.OperationRunning,
			"started_at":    now,
			"completed_at":  nil,
			"error_summary": "",
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"updated_at":    now,
		})
	return result.RowsAffected == 1, result.Error
}

func (r *OperationRepository) MarkQueued(id, errorSummary string, now time.Time) error {
	return r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ?", id, models.OperationRunning).
		Updates(map[string]interface{}{
			"status":        models.OperationQueued,
			"queued_at":     now,
			"error_summary": errorSummary,
			"updated_at":    now,
		}).Error
}

func (r *OperationRepository) MarkSucceeded(id string, now time.Time) error {
	result := r.db.Model(&models.Operation{}).
		Where("id = ? AND status = ?", id, models.OperationRunning).
		Updates(map[string]interface{}{
			"status":        models.OperationSucceeded,
			"completed_at":  now,
			"error_summary": "",
			"updated_at":    now,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errors.New("operation is no longer running")
	}
	return nil
}

func (r *OperationRepository) MarkFailed(id, errorSummary string, now time.Time) error {
	return r.db.Model(&models.Operation{}).
		Where("id = ? AND status IN ?", id, []models.OperationStatus{models.OperationQueued, models.OperationRunning}).
		Updates(map[string]interface{}{
			"status":        models.OperationFailed,
			"completed_at":  now,
			"error_summary": errorSummary,
			"updated_at":    now,
		}).Error
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

func (r *OperationRepository) SetResourceID(id string, resourceID uint) error {
	return r.db.Model(&models.Operation{}).
		Where("id = ? AND resource_id = 0", id).
		Update("resource_id", resourceID).Error
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
