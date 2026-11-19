package repository

import (
	"caiyun/internal/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const ExchangeTaskReceiptKind = "exchange_task"

// ExchangeExecutionReceipt separates one command's completion from a task's
// future schedule. It also owns the submission boundary for monthly accounts.
type ExchangeExecutionReceipt struct {
	ID              uint              `gorm:"primarykey" json:"-"`
	OperationID     string            `gorm:"size:36;not null;uniqueIndex:uk_exchange_receipt_operation_resource,priority:1" json:"-"`
	ResourceID      uint              `gorm:"not null;uniqueIndex:uk_exchange_receipt_operation_resource,priority:2" json:"resource_id"`
	ResourceKind    string            `gorm:"size:24;not null;uniqueIndex:uk_exchange_receipt_operation_resource,priority:3" json:"resource_kind"`
	UserID          uint              `gorm:"not null;index" json:"user_id"`
	ProductID       uint              `gorm:"not null;default:0" json:"product_id"`
	Outcome         string            `gorm:"size:16;not null;default:''" json:"outcome"`
	Message         string            `gorm:"type:text" json:"message"`
	ExecutionTimeMs int               `gorm:"not null;default:0" json:"execution_time_ms"`
	ExecutionToken  string            `gorm:"size:36;not null;default:''" json:"-"`
	CreatedAt       time.Time         `json:"created_at"`
	UpdatedAt       time.Time         `json:"updated_at"`
	CompletedAt     *time.Time        `json:"completed_at,omitempty"`
	Operation       *models.Operation `gorm:"foreignKey:OperationID;references:ID;constraint:OnDelete:CASCADE" json:"-"`
}

func (ExchangeExecutionReceipt) TableName() string { return "exchange_execution_receipts" }

type ExchangeReceiptRepository struct{ db *gorm.DB }

func NewExchangeReceiptRepository(db *gorm.DB) *ExchangeReceiptRepository {
	return &ExchangeReceiptRepository{db: db}
}
func (r *ExchangeReceiptRepository) WithContext(ctx context.Context) *ExchangeReceiptRepository {
	return &ExchangeReceiptRepository{db: r.db.WithContext(ctx)}
}

func (r *ExchangeReceiptRepository) FindByOperationResource(operationID, kind string, resourceID uint) (*ExchangeExecutionReceipt, error) {
	var receipt ExchangeExecutionReceipt
	err := r.db.Where("operation_id = ? AND resource_kind = ? AND resource_id = ?", operationID, kind, resourceID).First(&receipt).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &receipt, err
}

// StartSubmission atomically marks a new operation item unknown before POST.
// Existing accepted/unknown rows are never cleared or claimed as new work.
func (r *ExchangeReceiptRepository) StartSubmission(receipt *ExchangeExecutionReceipt) error {
	if receipt == nil || receipt.OperationID == "" || receipt.ResourceID == 0 || receipt.ResourceKind == "" || receipt.UserID == 0 || receipt.Outcome == string(models.ExchangeOutcomeUnknown) {
		return fmt.Errorf("invalid exchange receipt state")
	}
	candidate := *receipt
	candidate.Outcome = string(models.ExchangeOutcomeUnknown)
	if err := r.db.Create(&candidate).Error; err != nil {
		return err
	}
	*receipt = candidate
	return nil
}

// Finalize saves a result and record atomically without repeating an upstream
// request. A receipt remains unknown unless the upstream result is definitive.
func (r *ExchangeReceiptRepository) Finalize(receipt *ExchangeExecutionReceipt, record *models.ExchangeRecord) error {
	if receipt == nil || record == nil || receipt.ID == 0 {
		return fmt.Errorf("exchange receipt is missing")
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		result := tx.Model(&ExchangeExecutionReceipt{}).Where("id = ? AND completed_at IS NULL", receipt.ID).Updates(map[string]interface{}{
			"outcome": receipt.Outcome, "message": strings.TrimSpace(receipt.Message), "execution_time_ms": receipt.ExecutionTimeMs,
			"execution_token": "", "completed_at": now, "updated_at": now,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return fmt.Errorf("exchange receipt ownership lost")
		}
		return tx.Create(record).Error
	})
}
