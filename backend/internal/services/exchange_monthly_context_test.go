package services

import (
	"context"
	"errors"
	"testing"
)

func TestMonthlyExchangeRejectsMissingOperationBeforeAnyWork(t *testing.T) {
	service := &ExchangeService{}
	if err := service.ExecuteMonthlyExchangeContext(context.Background(), true); !errors.Is(err, ErrExchangeInvalidInput) {
		t.Fatalf("missing operation context = %v", err)
	}
}
