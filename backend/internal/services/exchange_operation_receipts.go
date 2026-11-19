package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"fmt"
)

const exchangeAccountReceiptKind = "exchange_account"

func (s *ExchangeService) completedTaskResult(ctx context.Context, operationID string, taskID uint) (bool, error) {
	if operationID == "" {
		return false, nil
	}
	if s.exchangeReceiptRepo == nil {
		return false, s.operationReceiptUnavailable(operationID)
	}
	receipt, err := s.exchangeReceiptRepo.WithContext(ctx).FindByOperationResource(operationID, repository.ExchangeTaskReceiptKind, taskID)
	if err != nil || receipt == nil || receipt.CompletedAt == nil {
		return false, err
	}
	return true, taskOutcomeError(models.ExchangeOutcome(receipt.Outcome), receipt.Message)
}

func taskOutcomeError(outcome models.ExchangeOutcome, message string) error {
	switch outcome {
	case models.ExchangeOutcomeAccepted:
		return nil
	case models.ExchangeOutcomeUnknown:
		return newExchangeOutcomeUnknownError(message)
	default:
		return fmt.Errorf("%w: %s", ErrExchangeExecutionFailed, message)
	}
}

func exchangeOperationContextID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	operation, _ := ctx.Value(taskOperationKey{}).(taskOperation)
	return operation.id
}

func (s *ExchangeService) operationReceiptUnavailable(operationID string) error {
	if operationID == "" {
		return nil
	}
	return fmt.Errorf("操作 %s 的回执存储不可用", operationID)
}
