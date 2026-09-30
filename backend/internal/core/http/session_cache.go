package http

import (
	"crypto/sha256"
	"fmt"
	"time"
)

// PrepareSessionOnce shares initialization across API wrappers belonging to
// this account client. JWT/SSO changes produce a new key; errors are not cached.
func (c *Client) PrepareSessionOnce(scope string, initialize func() error) error {
	identity := sha256.Sum256([]byte(c.jwtToken + "\x00" + c.ssoToken))
	key := fmt.Sprintf("%s:%x", scope, identity)
	c.sessionMu.Lock()
	defer c.sessionMu.Unlock()
	if until := c.preparedSessions[key]; time.Now().Before(until) {
		return nil
	}
	if err := initialize(); err != nil {
		return err
	}
	if c.preparedSessions == nil {
		c.preparedSessions = make(map[string]time.Time)
	}
	if len(c.preparedSessions) >= 64 {
		for old := range c.preparedSessions {
			delete(c.preparedSessions, old)
		}
	}
	c.preparedSessions[key] = time.Now().Add(10 * time.Minute)
	return nil
}
