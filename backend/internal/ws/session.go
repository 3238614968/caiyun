package ws

import (
	"context"
	"time"
)

const sessionRecheckInterval = 5 * time.Second

// SessionAuthorization binds a connection to its access-token expiry and
// server-side session. Validate must read authoritative session/user state so
// revocations on another API instance apply to existing connections too.
type SessionAuthorization struct {
	ExpiresAt time.Time
	Validate  func(context.Context) bool

	recheckInterval time.Duration
}

func connectionAuthorization(authorizations []SessionAuthorization) *SessionAuthorization {
	if len(authorizations) == 0 {
		// Compatibility for trusted in-process callers. HTTP routes always bind
		// the authenticated session explicitly.
		return nil
	}
	return &authorizations[0]
}

func (s *SessionAuthorization) valid(ctx context.Context) bool {
	if s == nil {
		return true
	}
	if s.ExpiresAt.IsZero() || !time.Now().Before(s.ExpiresAt) || s.Validate == nil {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return ctx.Err() == nil && s.Validate(ctx) && time.Now().Before(s.ExpiresAt)
}

// timers also close idle connections, without waiting for a new application
// event. Every data write independently validates to avoid leaking an event
// during the periodic recheck window.
func (s *SessionAuthorization) timers() (<-chan time.Time, <-chan time.Time, func()) {
	if s == nil {
		return nil, nil, func() {}
	}
	interval := s.recheckInterval
	if interval <= 0 {
		interval = sessionRecheckInterval
	}
	recheck := time.NewTicker(interval)
	expiry := time.NewTimer(time.Until(s.ExpiresAt))
	return expiry.C, recheck.C, func() { expiry.Stop(); recheck.Stop() }
}
