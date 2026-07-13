package services

import (
	"testing"
	"time"
)

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
