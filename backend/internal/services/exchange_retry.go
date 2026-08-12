package services

import (
	"caiyun/internal/constants"
	"context"
)

func effectiveExchangeMaxRetries(configured int) int {
	if configured > 0 {
		return configured
	}
	return constants.DefaultMaxRetries
}

// exchangeAttempt is one upstream exchange invocation.  The callback returns
// success, a user-facing result, and execution duration in milliseconds.
type exchangeAttempt func() (bool, string, int)

// runExchangeWithRetries is the shared retry state machine used by manual and
// scheduled exchange execution.  It deliberately owns only retry policy;
// callers retain persistence and their fenced-token updates in beforeRetry.
func runExchangeWithRetries(
	ctx context.Context,
	maxRetries int,
	beforeRetry func(attempt int) error,
	attemptExchange exchangeAttempt,
) (success bool, message string, execTime int, attemptsUsed int, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		attemptsUsed = attempt
		if err := ctx.Err(); err != nil {
			return false, err.Error(), 0, attemptsUsed, err
		}
		if attempt > 0 && beforeRetry != nil {
			if err := beforeRetry(attempt); err != nil {
				return false, err.Error(), 0, attemptsUsed, err
			}
		}
		if attemptExchange == nil {
			return false, "抢兑执行器未配置", 0, attemptsUsed, nil
		}
		success, message, execTime = attemptExchange()
		if success || !isExchangeRetryableFailure(message) {
			return success, message, execTime, attemptsUsed, nil
		}
	}
	return success, message, execTime, attemptsUsed, nil
}
