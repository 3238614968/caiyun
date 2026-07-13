package repository

import (
	"context"
	"fmt"

	"gorm.io/gorm"
)

// TransactionRepositories contains repository instances bound to one database
// transaction. Services must only use the instances passed to the callback;
// mixing them with repositories created from the root connection would escape
// the transaction and break atomicity.
type TransactionRepositories struct {
	User            *UserRepository
	Account         *AccountRepository
	TaskLog         *TaskLogRepository
	CloudStats      *CloudStatsRepository
	Product         *ProductRepository
	ExchangeAccount *ExchangeAccountRepository
	ExchangeTask    *ExchangeTaskRepository
	ExchangeRecord  *ExchangeRecordRepository
	SystemConfig    *SystemConfigRepository
	AuditLog        *AuditLogRepository
	WSMessage       *WSMessageRepository
	Operation       *OperationRepository
	RefreshSession  *RefreshSessionRepository
}

// UnitOfWork is the transaction boundary exposed to the service layer. It
// deliberately exposes repositories instead of *gorm.DB so business services
// cannot issue infrastructure-specific SQL.
type UnitOfWork interface {
	WithinTransaction(ctx context.Context, fn func(TransactionRepositories) error) error
}

type gormUnitOfWork struct {
	db *gorm.DB
}

// NewUnitOfWork creates a GORM-backed Unit of Work.
func NewUnitOfWork(db *gorm.DB) UnitOfWork {
	return &gormUnitOfWork{db: db}
}

// NewUnitOfWorkFromExchangeAccountRepository keeps legacy service constructors
// source-compatible while still giving every ExchangeService a real transaction
// boundary.
func NewUnitOfWorkFromExchangeAccountRepository(repo *ExchangeAccountRepository) UnitOfWork {
	if repo == nil {
		return nil
	}
	return NewUnitOfWork(repo.db)
}

// NewUnitOfWorkFromExchangeTaskRepository is used by the worker scheduler.
func NewUnitOfWorkFromExchangeTaskRepository(repo *ExchangeTaskRepository) UnitOfWork {
	if repo == nil {
		return nil
	}
	return NewUnitOfWork(repo.db)
}

// NewUnitOfWorkFromUserRepository is used by administrative user lifecycle
// operations.
func NewUnitOfWorkFromUserRepository(repo *UserRepository) UnitOfWork {
	if repo == nil {
		return nil
	}
	return NewUnitOfWork(repo.db)
}

// NewUnitOfWorkFromAccountRepository is used for atomic account credential
// updates that must remain synchronized with derived exchange credentials.
func NewUnitOfWorkFromAccountRepository(repo *AccountRepository) UnitOfWork {
	if repo == nil {
		return nil
	}
	return NewUnitOfWork(repo.db)
}

func (u *gormUnitOfWork) WithinTransaction(ctx context.Context, fn func(TransactionRepositories) error) error {
	if u == nil || u.db == nil {
		return fmt.Errorf("unit of work database is nil")
	}
	if fn == nil {
		return fmt.Errorf("unit of work callback is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	return u.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(newTransactionRepositories(tx))
	})
}

func newTransactionRepositories(tx *gorm.DB) TransactionRepositories {
	return TransactionRepositories{
		User:            NewUserRepository(tx),
		Account:         NewAccountRepository(tx),
		TaskLog:         NewTaskLogRepository(tx),
		CloudStats:      NewCloudStatsRepository(tx),
		Product:         NewProductRepository(tx),
		ExchangeAccount: NewExchangeAccountRepository(tx),
		ExchangeTask:    NewExchangeTaskRepository(tx),
		ExchangeRecord:  NewExchangeRecordRepository(tx),
		SystemConfig:    NewSystemConfigRepository(tx),
		AuditLog:        NewAuditLogRepository(tx),
		WSMessage:       NewWSMessageRepository(tx),
		Operation:       NewOperationRepository(tx),
		RefreshSession:  NewRefreshSessionRepository(tx),
	}
}
