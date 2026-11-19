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

// exchangeAttempt is one upstream exchange invocation. The same structured
// result is returned to the caller; unknown submissions are never flattened
// into an ordinary retryable failure.
type exchangeAttempt func() exchangeAttemptResult

// runExchangeWithRetries retries only pre-submission failures. Once an attempt
// has a durable outcome, this function returns it without another upstream call.
func runExchangeWithRetries(
	ctx context.Context,
	maxRetries int,
	beforeRetry func(attempt int) error,
	attemptExchange exchangeAttempt,
) (result exchangeAttemptResult, attemptsUsed int, err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if maxRetries < 0 {
		maxRetries = 0
	}
	for attempt := 0; attempt <= maxRetries; attempt++ {
		attemptsUsed = attempt
		if err := ctx.Err(); err != nil {
			return preflightExchangeResult(err.Error(), false), attemptsUsed, err
		}
		if attempt > 0 && beforeRetry != nil {
			if err := beforeRetry(attempt); err != nil {
				return preflightExchangeResult(err.Error(), false), attemptsUsed, err
			}
		}
		if attemptExchange == nil {
			return preflightExchangeResult("抢兑执行器未配置", false), attemptsUsed, nil
		}
		result = attemptExchange()
		if !result.retryable() {
			return result, attemptsUsed, nil
		}
	}
	return result, attemptsUsed, nil
}
