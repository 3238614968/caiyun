//go:build cgo

package services

import (
	"fmt"
	"testing"

	"caiyun/internal/models"
	"caiyun/internal/repository"
)

func TestAccountAuthorizationRefreshRollsBackBothCredentialTables(t *testing.T) {
	f := newAdminCleanupFixture(t)
	accountRepo := repository.NewAccountRepository(f.db)
	exchangeRepo := repository.NewExchangeAccountRepository(f.db)
	service := NewAccountService(accountRepo, repository.NewUserRepository(f.db), nil, nil, exchangeRepo)

	trigger := fmt.Sprintf(
		"CREATE TRIGGER fail_exchange_auth_update BEFORE UPDATE OF auth ON exchange_rules WHEN OLD.account_id = %d BEGIN SELECT RAISE(ABORT, 'forced exchange auth failure'); END",
		f.account.ID,
	)
	if err := f.db.Exec(trigger).Error; err != nil {
		t.Fatalf("create rollback trigger: %v", err)
	}

	updated := *f.account
	updated.Auth = "new-account-auth"
	updated.Token = "new-account-token"
	updated.JWTToken = "new-account-jwt"
	if err := service.persistAuthorizationRefreshContext(t.Context(), &updated); err == nil {
		t.Fatal("persistAuthorizationRefreshContext() error = nil, want forced rollback")
	}

	var account models.Account
	if err := f.db.First(&account, f.account.ID).Error; err != nil {
		t.Fatalf("reload account: %v", err)
	}
	if account.Auth != "secret-auth" || account.Token != "secret-token" || account.JWTToken != "secret-jwt" {
		t.Fatalf("account credentials partially committed: auth=%q token=%q jwt=%q", account.Auth, account.Token, account.JWTToken)
	}

	var rule models.ExchangeAccount
	if err := f.db.First(&rule, f.rule.ID).Error; err != nil {
		t.Fatalf("reload exchange account: %v", err)
	}
	if rule.Auth != "rule-auth" || rule.Token != "rule-token" || rule.JWTToken != "rule-jwt" {
		t.Fatalf("exchange credentials changed after rollback: auth=%q token=%q jwt=%q", rule.Auth, rule.Token, rule.JWTToken)
	}
}
