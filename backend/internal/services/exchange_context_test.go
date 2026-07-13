package services

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExecuteExchangeTaskContextStopsBeforeRepositoryOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := &ExchangeService{}
	err := service.ExecuteExchangeTaskContext(ctx, 1, 1)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("ExecuteExchangeTaskContext() error = %v, want context.Canceled", err)
	}
}

func TestExchangeRequestControllerWaitContextCancellation(t *testing.T) {
	controller := &exchangeRequestController{
		enabled: true,
		tokens:  make(chan struct{}),
		stopCh:  make(chan struct{}),
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := controller.WaitContext(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("WaitContext() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("WaitContext() took %s after cancellation", elapsed)
	}
}

func TestSleepExchangeContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := sleepExchangeContext(ctx, time.Minute)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("sleepExchangeContext() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("sleepExchangeContext() took %s after cancellation", elapsed)
	}
}
