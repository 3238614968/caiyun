package services

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"
)

func TestAuthorizationShouldRefresh(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)

	tests := []struct {
		name     string
		expireAt int64
		want     bool
	}{
		{name: "unknown", expireAt: 0, want: true},
		{name: "expired", expireAt: now.Add(-time.Minute).UnixMilli(), want: true},
		{name: "within skew", expireAt: now.Add(4 * 24 * time.Hour).UnixMilli(), want: true},
		{name: "outside skew", expireAt: now.Add(6 * 24 * time.Hour).UnixMilli(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := authorizationShouldRefresh(tt.expireAt, now); got != tt.want {
				t.Fatalf("authorizationShouldRefresh()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestJWTExpiresAtSupportsSecondsAndMilliseconds(t *testing.T) {
	for _, expiry := range []int64{1_800_000_000, 1_800_000_000_000} {
		payload := base64.RawURLEncoding.EncodeToString([]byte(fmt.Sprintf(`{"exp":%d}`, expiry)))
		got := jwtExpiresAt("header." + payload + ".signature")
		if got.Unix() != 1_800_000_000 {
			t.Fatalf("jwtExpiresAt(%d) = %s", expiry, got)
		}
	}
	if !jwtExpiresAt("invalid").IsZero() {
		t.Fatal("malformed JWT must not be treated as usable")
	}
}

func TestJWTUserDomainID(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"{\"userDomainId\":\"1039741969532555307\"}"}`))
	token := "header." + payload + ".sig"

	if got := jwtUserDomainID(token); got != "1039741969532555307" {
		t.Fatalf("jwtUserDomainID()=%q", got)
	}
}
