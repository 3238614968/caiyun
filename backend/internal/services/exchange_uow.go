package services

import (
	"context"
	"errors"

	"caiyun/internal/repository"
)

var ErrUnitOfWorkUnavailable = errors.New("unit of work unavailable")

// withinTransaction clones the service and replaces every database repository
// with the transaction-bound instance supplied by the Unit of Work.
func (s *ExchangeService) withinTransaction(ctx context.Context, fn func(*ExchangeService) error) error {
	if s == nil || s.unitOfWork == nil {
		return ErrUnitOfWorkUnavailable
	}
	if fn == nil {
		return errors.New("transaction callback is nil")
	}
	return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
		clone := *s
		clone.productRepo = repos.Product
		clone.exchangeAccountRepo = repos.ExchangeAccount
		clone.exchangeTaskRepo = repos.ExchangeTask
		clone.accountRepo = repos.Account
		clone.configRepo = repos.SystemConfig
		clone.exchangeRecordRepo = repos.ExchangeRecord
		clone.taskLogRepo = repos.TaskLog
		return fn(&clone)
	})
}
