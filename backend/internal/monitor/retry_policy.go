package monitor

import "errors"

func isNonRetryableTaskError(err error) bool {
	var policy interface{ Retryable() bool }
	return errors.As(err, &policy) && !policy.Retryable()
}
