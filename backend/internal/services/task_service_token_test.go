package services

import (
	"testing"

	"caiyun/internal/core/auth"
	"caiyun/internal/models"
)

func TestManagedRefreshUpdatesRawTaskToken(t *testing.T) {
	freshAuth := auth.GenerateAuth("fresh-token", "13800000001", "mobile")
	account := &models.Account{Auth: auth.GenerateAuth("old-token", "13800000001", "mobile"), Token: "old-token", JWTToken: "old-jwt"}
	sso := applyManagedTokenInfo(account, &TokenInfo{Auth: freshAuth, JWTToken: "fresh-jwt", SSOToken: "fresh-sso"})
	if sso != "fresh-sso" || account.Token != "fresh-token" || account.JWTToken != "fresh-jwt" {
		t.Fatalf("managed refresh left stale account fields: token=%q jwt=%q sso=%q", account.Token, account.JWTToken, sso)
	}
	runner := &TaskRunner{account: account}
	if got := runner.getRawAccountToken(); got != "fresh-token" {
		t.Fatalf("task runner raw token = %q, want fresh-token", got)
	}
}

func TestRawTaskTokenPrefersAuthOverStaleField(t *testing.T) {
	account := &models.Account{Auth: auth.GenerateAuth("new-token", "13800000001", "mobile"), Token: "stale-token"}
	if got := (&TaskRunner{account: account}).getRawAccountToken(); got != "new-token" {
		t.Fatalf("raw token = %q, want new-token", got)
	}
}

func TestMalformedManagedAuthDoesNotReuseOldRawToken(t *testing.T) {
	account := &models.Account{Auth: auth.GenerateAuth("old-token", "13800000001", "mobile"), Token: "old-token"}
	applyManagedTokenInfo(account, &TokenInfo{Auth: "invalid-new-auth", JWTToken: "new-jwt"})
	if got := (&TaskRunner{account: account}).getRawAccountToken(); got != "" {
		t.Fatalf("malformed new authorization reused old token %q", got)
	}
}
