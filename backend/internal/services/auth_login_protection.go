package services

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/go-redis/redis/v8"
)

type loginLockStore interface {
	GetLoginFailure(keyHash string) (int, time.Time, error)
	RecordLoginFailure(keyHash string, maxAttempts int, window, lockTTL time.Duration) error
	ClearLoginFailure(keyHash string) error
}

type atomicLoginFailureCache interface {
	IncrementWithTTL(key string, window time.Duration) (int, error)
}

const (
	// Username and username+IP scopes stop focused credential stuffing quickly.
	loginUsernameMaxAttempts          = 5
	loginUsernameAndClientMaxAttempts = 5
	// A shared NAT address must tolerate failures from several unrelated users
	// before it is treated as a password-spraying source.
	loginClientIPMaxAttempts = 20
	// loginLockMaxAttempts is retained for compatibility with historical tests
	// and callers that exercise the username-level default directly.
	loginLockMaxAttempts = loginUsernameMaxAttempts
	// loginLockTTL 是锁定持续时间。
	loginLockTTL = 15 * time.Minute
	// loginFailWindow 是失败计数窗口。
	loginFailWindow = loginLockTTL
)

type loginLockKeys struct {
	Username          string
	ClientIP          string
	UsernameAndClient string
}

type loginLockScope struct {
	Key         string
	MaxAttempts int
}

// loginLockKeysFor scopes failures independently by account name, source IP,
// and their pair. The largest counter decides admission so password spraying,
// credential stuffing from one source, and single-account attacks are all
// constrained by the same policy.
func loginLockKeysFor(username, clientIP string) loginLockKeys {
	username = strings.ToLower(strings.TrimSpace(username))
	clientIP = strings.TrimSpace(clientIP)
	if username == "" {
		username = "unknown"
	}
	if clientIP == "" {
		clientIP = "unknown"
	}
	return loginLockKeys{
		Username:          "caiyun:login_fail:username:" + username,
		ClientIP:          "caiyun:login_fail:ip:" + clientIP,
		UsernameAndClient: "caiyun:login_fail:pair:" + username + ":" + clientIP,
	}
}

func (k loginLockKeys) all() []string {
	scopes := k.scopes()
	keys := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		keys = append(keys, scope.Key)
	}
	return keys
}

func (k loginLockKeys) scopes() []loginLockScope {
	return []loginLockScope{
		{Key: k.Username, MaxAttempts: loginUsernameMaxAttempts},
		{Key: k.ClientIP, MaxAttempts: loginClientIPMaxAttempts},
		{Key: k.UsernameAndClient, MaxAttempts: loginUsernameAndClientMaxAttempts},
	}
}

// loginLockKey is retained for callers and historical tests that inspect the
// original pair scope. New login flows must use loginLockKeysFor().
func loginLockKey(username, clientIP string) string {
	return loginLockKeysFor(username, clientIP).UsernameAndClient
}

func loginLockStoreKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// getLoginFailCount 返回当前用户名的连续登录失败次数。
// cache 未配置时返回 0，兼容本地调试。
func (s *AuthService) getLoginFailCount(key string) (int, error) {
	return s.getLoginFailCountWithLimit(key, loginLockMaxAttempts)
}

func (s *AuthService) getLoginFailCountWithLimit(key string, maxAttempts int) (int, error) {
	if maxAttempts <= 0 {
		maxAttempts = loginLockMaxAttempts
	}
	var cacheErr error
	if s.loginLockCache != nil {
		var count int
		err := s.loginLockCache.Get(key, &count)
		if err == nil {
			return count, nil
		}
		if !isLoginLockCacheMiss(err) {
			cacheErr = err
		}
	}
	if s.loginLockStore != nil {
		count, lockedUntil, err := s.loginLockStore.GetLoginFailure(loginLockStoreKey(key))
		if err != nil {
			return 0, err
		}
		if !lockedUntil.IsZero() && lockedUntil.After(time.Now()) && count < maxAttempts {
			return maxAttempts, nil
		}
		return count, nil
	}
	if cacheErr != nil {
		return 0, cacheErr
	}
	return 0, nil
}

func isLoginLockCacheMiss(err error) bool {
	return errors.Is(err, redis.Nil)
}

func (s *AuthService) recordLoginFailure(key string) error {
	return s.recordLoginFailureWithLimit(key, loginLockMaxAttempts)
}

func (s *AuthService) recordLoginFailureWithLimit(key string, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = loginLockMaxAttempts
	}
	if s.loginLockCache == nil && s.loginLockStore == nil {
		return nil
	}

	var failures []error
	if s.loginLockCache != nil {
		if atomicStore, ok := s.loginLockCache.(atomicLoginFailureCache); ok {
			if _, err := atomicStore.IncrementWithTTL(key, loginFailWindow); err == nil {
				return nil
			} else {
				failures = append(failures, err)
			}
		} else {
			// Legacy cache implementations are retained for tests and offline
			// callers; production Redis uses the atomic path above.
			var count int
			if err := s.loginLockCache.Get(key, &count); err != nil && !isLoginLockCacheMiss(err) {
				failures = append(failures, err)
			}
			count++
			if err := s.loginLockCache.Set(key, count, loginFailWindow); err == nil {
				return nil
			} else {
				failures = append(failures, err)
			}
		}
	}
	if s.loginLockStore != nil {
		if err := s.loginLockStore.RecordLoginFailure(loginLockStoreKey(key), maxAttempts, loginFailWindow, loginLockTTL); err == nil {
			return nil
		} else {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *AuthService) clearLoginFailure(key string) error {
	var failures []error
	if s.loginLockCache != nil {
		if err := s.loginLockCache.Del(key); err != nil {
			failures = append(failures, err)
		}
	}
	if s.loginLockStore != nil {
		if err := s.loginLockStore.ClearLoginFailure(loginLockStoreKey(key)); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *AuthService) getLoginFailCountForKeys(keys loginLockKeys) (int, error) {
	maxCount := 0
	for _, scope := range keys.scopes() {
		count, err := s.getLoginFailCountWithLimit(scope.Key, scope.MaxAttempts)
		if err != nil {
			return 0, err
		}
		if count > maxCount {
			maxCount = count
		}
	}
	return maxCount, nil
}

func (s *AuthService) isLoginLockedForKeys(keys loginLockKeys) (bool, error) {
	for _, scope := range keys.scopes() {
		count, err := s.getLoginFailCountWithLimit(scope.Key, scope.MaxAttempts)
		if err != nil {
			return false, err
		}
		if count >= scope.MaxAttempts {
			return true, nil
		}
	}
	return false, nil
}

func (s *AuthService) recordLoginFailureForKeys(keys loginLockKeys) error {
	var failures []error
	for _, scope := range keys.scopes() {
		if err := s.recordLoginFailureWithLimit(scope.Key, scope.MaxAttempts); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func (s *AuthService) clearLoginFailureForKeys(keys loginLockKeys) error {
	var failures []error
	for _, key := range keys.all() {
		if err := s.clearLoginFailure(key); err != nil {
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}
