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
		// 默认 lead=72h：进入窗口即视为需要续期。
		{name: "inside lead window", expireAt: now.Add(2 * 24 * time.Hour).UnixMilli(), want: true},
		{name: "unexpired", expireAt: now.Add(4 * 24 * time.Hour).UnixMilli(), want: false},
		{name: "outside skew", expireAt: now.Add(6 * 24 * time.Hour).UnixMilli(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CAIYUN_AUTH_REFRESH_LEAD_HOURS", "")
			if got := authorizationShouldRefresh(tt.expireAt, now); got != tt.want {
				t.Fatalf("authorizationShouldRefresh()=%v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuthorizationRefreshLeadConfig(t *testing.T) {
	now := time.Now()
	twoDaysAway := now.Add(48 * time.Hour).UnixMilli()

	t.Run("lead disabled keeps old behavior", func(t *testing.T) {
		t.Setenv("CAIYUN_AUTH_REFRESH_LEAD_HOURS", "0")
		if authorizationShouldRefresh(twoDaysAway, now) {
			t.Fatal("lead=0 时未到期凭据不应触发续期")
		}
	})
	t.Run("custom lead hours honored", func(t *testing.T) {
		t.Setenv("CAIYUN_AUTH_REFRESH_LEAD_HOURS", "100")
		if !authorizationShouldRefresh(twoDaysAway, now) {
			t.Fatal("lead=100h 时 48h 后到期的凭据应触发续期")
		}
	})
}

func TestAuthorizationShouldRotateEarly(t *testing.T) {
	now := time.UnixMilli(1_700_000_000_000)

	tests := []struct {
		name     string
		expireAt int64
		want     bool
	}{
		// 陈旧/缺失元数据（JWT 兑换成功证明凭据可用）不作为主动轮换依据。
		{name: "stale past expiry", expireAt: now.Add(-time.Minute).UnixMilli(), want: false},
		{name: "unknown expiry", expireAt: 0, want: false},
		// 未来 lead 窗口内（默认 72h）才主动轮换。
		{name: "inside lead window", expireAt: now.Add(2 * 24 * time.Hour).UnixMilli(), want: true},
		{name: "far from expiry", expireAt: now.Add(10 * 24 * time.Hour).UnixMilli(), want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("CAIYUN_AUTH_REFRESH_LEAD_HOURS", "")
			if got := authorizationShouldRotateEarly(tt.expireAt, now); got != tt.want {
				t.Fatalf("authorizationShouldRotateEarly()=%v, want %v", got, tt.want)
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
