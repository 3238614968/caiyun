package models

import "time"

// OperationStatus is the durable lifecycle of an asynchronous command.
type OperationStatus string

const (
	OperationQueued    OperationStatus = "queued"
	OperationRunning   OperationStatus = "running"
	OperationSucceeded OperationStatus = "succeeded"
	OperationFailed    OperationStatus = "failed"
	OperationCanceled  OperationStatus = "canceled"
)

const (
	OperationTypeAccountTask       = "account.task.execute"
	OperationTypeAccountTaskBatch  = "account.tasks.batch"
	OperationTypeExchangeTask      = "exchange.task.execute"
	OperationTypeExchangeTaskBatch = "exchange.tasks.batch"
	OperationTypeExchangeImmediate = "exchange.immediate"
	OperationTypeExchangeMonthly   = "exchange.monthly.execute"
)

// Operation is both the client-visible status record and the durable outbox
// source for an asynchronous command. The Redis message only carries the ID;
// the Worker always reloads the authoritative payload from this row.
type Operation struct {
	ID             string          `gorm:"primaryKey;size:36" json:"id"`
	UserID         uint            `gorm:"not null;index;uniqueIndex:uq_operations_user_idempotency" json:"-"`
	OperationType  string          `gorm:"column:operation_type;size:64;not null;index" json:"type"`
	Status         OperationStatus `gorm:"size:16;not null;index" json:"status"`
	ExecutionToken string          `gorm:"size:36;not null;default:''" json:"-"`
	AccountID      uint            `gorm:"not null;default:0;index" json:"account_id,omitempty"`
	ResourceID     uint            `gorm:"not null;default:0;index" json:"resource_id,omitempty"`
	Payload        string          `gorm:"type:longtext;not null" json:"-"`
	IdempotencyKey string          `gorm:"size:191;not null;uniqueIndex:uq_operations_user_idempotency" json:"-"`
	AttemptCount   int             `gorm:"not null;default:0" json:"attempt_count"`
	ErrorSummary   string          `gorm:"size:512;not null;default:''" json:"error_summary,omitempty"`
	QueuedAt       time.Time       `gorm:"not null;index" json:"queued_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `gorm:"index" json:"updated_at"`
}

func (Operation) TableName() string { return "operations" }

func (o *Operation) Terminal() bool {
	if o == nil {
		return false
	}
	switch o.Status {
	case OperationSucceeded, OperationFailed, OperationCanceled:
		return true
	default:
		return false
	}
}
