package services

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"caiyun/internal/models"
)

func TestUsablePreviousJWTAvoidsDisablingOnFailedPreRefresh(t *testing.T) {
	now := time.Now().Truncate(time.Second)
	token := func(exp time.Time) string {
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, exp.Unix())))
		return "header." + payload + ".signature"
	}
	account := &models.Account{ID: 7, JWTToken: token(now.Add(time.Hour)), ExpireAt: now.Add(24 * time.Hour).UnixMilli()}
	tm := &TokenManager{}
	tm.tokenCache.Store(account.ID, &TokenInfo{JWTToken: account.JWTToken, ExpiresAt: now.Add(time.Minute)})
	fallback := tm.usablePreviousJWT(account, now)
	if fallback == nil || fallback.JWTToken != account.JWTToken || !fallback.ExpiresAt.After(now.Add(2*time.Minute)) {
		t.Fatalf("valid JWT was not retained after failed pre-refresh: %+v", fallback)
	}
	account.JWTToken = token(now.Add(-time.Minute))
	tm.tokenCache.Delete(account.ID)
	if got := tm.usablePreviousJWT(account, now); got != nil {
		t.Fatalf("expired JWT was incorrectly reused: %+v", got)
	}
}

func TestPreRefreshExpiredTokensCapsScanBatch(t *testing.T) {
	t.Setenv("TOKEN_PREREFRESH_MAX_SCAN", "3")
	tm := &TokenManager{preRefreshChan: make(chan uint, 16)}
	expiresAt := time.Now().Add(2 * time.Minute)
	for i := uint(1); i <= 10; i++ {
		tm.tokenCache.Store(i, &TokenInfo{JWTToken: "jwt", ExpiresAt: expiresAt, HealthStatus: "healthy"})
	}

	tm.preRefreshExpiredTokens()
	if got := len(tm.preRefreshChan); got != 3 {
		t.Fatalf("queued pre-refresh count = %d, want 3", got)
	}
}

func TestTokenHealthScanPublishesImmutableSnapshot(t *testing.T) {
	mgr := &TokenManager{}
	old := &TokenInfo{JWTToken: "jwt", ExpiresAt: time.Now().Add(-time.Minute), LastRefresh: time.Now(), HealthStatus: "healthy"}
	mgr.tokenCache.Store(uint(7), old)
	mgr.checkAllTokensHealth()
	if old.HealthStatus != "healthy" {
		t.Fatalf("health scan mutated a shared token pointer: %+v", old)
	}
	stored, ok := mgr.tokenCache.Load(uint(7))
	if !ok || stored.(*TokenInfo) == old || stored.(*TokenInfo).HealthStatus != "error" {
		t.Fatalf("health scan did not publish a new error snapshot: %+v", stored)
	}
}

func TestTokenCacheRejectsChangedAuthorization(t *testing.T) {
	account := &models.Account{Auth: "Basic new-auth", IsActive: true}
	if tokenCacheMatchesAccount(&TokenInfo{Auth: "Basic old-auth"}, account) {
		t.Fatal("old cached authorization must be refreshed after account update")
	}
	if !tokenCacheMatchesAccount(&TokenInfo{Auth: "Basic new-auth"}, account) {
		t.Fatal("current cached authorization should remain usable")
	}
}

func TestTokenRefreshLockRenewIntervalIsBounded(t *testing.T) {
	if got := tokenRefreshLockRenewInterval(30 * time.Second); got != 10*time.Second {
		t.Fatalf("renew interval = %s, want 10s", got)
	}
	if got := tokenRefreshLockRenewInterval(500 * time.Millisecond); got != time.Second {
		t.Fatalf("small TTL renew interval = %s, want lower bound 1s", got)
	}
}

func TestTokenPreRefreshMaxScanFromEnvFallsBackToDefault(t *testing.T) {
	t.Run("invalid", func(t *testing.T) {
		t.Setenv("TOKEN_PREREFRESH_MAX_SCAN", "invalid")
		if got := tokenPreRefreshMaxScanFromEnv(); got != defaultTokenPreRefreshMaxScan {
			t.Fatalf("tokenPreRefreshMaxScanFromEnv() = %d, want %d", got, defaultTokenPreRefreshMaxScan)
		}
	})

	t.Run("empty", func(t *testing.T) {
		t.Setenv("TOKEN_PREREFRESH_MAX_SCAN", "")
		if got := tokenPreRefreshMaxScanFromEnv(); got != defaultTokenPreRefreshMaxScan {
			t.Fatalf("tokenPreRefreshMaxScanFromEnv() without env = %d, want %d", got, defaultTokenPreRefreshMaxScan)
		}
	})
}

func TestSanitizeAuthValuePreservesUnicodeButRemovesControls(t *testing.T) {
	input := "  Bearer 值abc" + "\r\n\t" + "XYZ" + string(rune(0)) + "  "
	got := sanitizeAuthValue(input)
	want := "Bearer 值abcXYZ"
	if got != want {
		t.Fatalf("sanitizeAuthValue() = %q, want %q", got, want)
	}
}
