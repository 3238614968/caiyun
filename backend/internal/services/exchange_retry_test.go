package services

import (
	"context"
	"errors"
	"testing"
)

func TestRunExchangeWithRetriesRetriesTransientFailure(t *testing.T) {
	called := 0
	beforeRetries := []int{}
	success, message, execTime, attempts, err := runExchangeWithRetries(
		context.Background(),
		2,
		func(attempt int) error {
			beforeRetries = append(beforeRetries, attempt)
			return nil
		},
		func() (bool, string, int) {
			called++
			if called == 1 {
				return false, "upstream timeout", 12
			}
			return true, "success", 34
		},
	)
	if err != nil || !success || message != "success" || execTime != 34 {
		t.Fatalf("runExchangeWithRetries() = (%t, %q, %d, %d, %v)", success, message, execTime, attempts, err)
	}
	if called != 2 || attempts != 1 {
		t.Fatalf("attempts called=%d used=%d, want 2/1", called, attempts)
	}
	if len(beforeRetries) != 1 || beforeRetries[0] != 1 {
		t.Fatalf("before retry calls = %#v, want [1]", beforeRetries)
	}
}

func TestRunExchangeWithRetriesDoesNotRetryTerminalFailure(t *testing.T) {
	called := 0
	success, _, _, attempts, err := runExchangeWithRetries(
		context.Background(),
		3,
		func(int) error { t.Fatal("terminal failure retried"); return nil },
		func() (bool, string, int) {
			called++
			return false, "奖品已兑完", 1
		},
	)
	if err != nil || success || called != 1 || attempts != 0 {
		t.Fatalf("terminal failure result = success=%t calls=%d attempts=%d err=%v", success, called, attempts, err)
	}
}

func TestRunExchangeWithRetriesStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := 0
	_, _, _, attempts, err := runExchangeWithRetries(
		ctx,
		3,
		func(int) error { return nil },
		func() (bool, string, int) {
			called++
			cancel()
			return false, "upstream timeout", 1
		},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled retry loop error = %v, want context.Canceled", err)
	}
	if called != 1 || attempts != 1 {
		t.Fatalf("canceled loop calls=%d attempts=%d, want 1/1", called, attempts)
	}
}
