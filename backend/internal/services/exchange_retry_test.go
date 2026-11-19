package services

import (
	"caiyun/internal/models"
	"context"
	"errors"
	"testing"
)

func TestRunExchangeWithRetriesRetriesOnlyUnsubmittedFailure(t *testing.T) {
	called := 0
	beforeRetries := []int{}
	result, attempts, err := runExchangeWithRetries(
		context.Background(), 2,
		func(attempt int) error {
			beforeRetries = append(beforeRetries, attempt)
			return nil
		},
		func() exchangeAttemptResult {
			called++
			if called == 1 {
				return preflightExchangeResult("upstream timeout", false)
			}
			return acceptedExchangeResult("success", 34)
		},
	)
	if err != nil || !result.success || result.message != "success" || result.outcome != models.ExchangeOutcomeAccepted {
		t.Fatalf("result = %+v err=%v", result, err)
	}
	if called != 2 || attempts != 1 || len(beforeRetries) != 1 || beforeRetries[0] != 1 {
		t.Fatalf("attempts called=%d used=%d before=%v", called, attempts, beforeRetries)
	}
}

func TestRunExchangeWithRetriesNeverRepeatsSubmittedOutcome(t *testing.T) {
	for _, outcome := range []models.ExchangeOutcome{models.ExchangeOutcomeUnknown, models.ExchangeOutcomeRejected, models.ExchangeOutcomeAccepted} {
		called := 0
		result, attempts, err := runExchangeWithRetries(context.Background(), 3,
			func(int) error { t.Fatal("settled outcome retried"); return nil },
			func() exchangeAttemptResult {
				called++
				return newExchangeAttemptResult(outcome == models.ExchangeOutcomeAccepted, "result", 1, outcome)
			})
		if err != nil || called != 1 || attempts != 0 || result.outcome != outcome {
			t.Fatalf("outcome=%s result=%+v calls=%d attempts=%d err=%v", outcome, result, called, attempts, err)
		}
	}
}

func TestRunExchangeWithRetriesStopsWhenContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	called := 0
	_, attempts, err := runExchangeWithRetries(ctx, 3, nil, func() exchangeAttemptResult {
		called++
		cancel()
		return preflightExchangeResult("upstream timeout", false)
	})
	if !errors.Is(err, context.Canceled) || called != 1 || attempts != 1 {
		t.Fatalf("canceled calls=%d attempts=%d err=%v", called, attempts, err)
	}
}
