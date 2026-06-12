package services

import (
	"encoding/base64"
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

func TestJWTUserDomainID(t *testing.T) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"{\"userDomainId\":\"1039741969532555307\"}"}`))
	token := "header." + payload + ".sig"

	if got := jwtUserDomainID(token); got != "1039741969532555307" {
		t.Fatalf("jwtUserDomainID()=%q", got)
	}
}
