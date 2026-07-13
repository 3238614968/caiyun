package models

import (
	"testing"

	"caiyun/internal/security"
)

func TestAccountSensitiveFieldHooks(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	security.ResetFieldCryptoForTests()

	account := &Account{Auth: "plain-auth", Token: "plain-token", JWTToken: "plain-jwt"}
	if err := account.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave() error = %v", err)
	}
	if account.Auth == "plain-auth" || account.Token == "plain-token" || account.JWTToken == "plain-jwt" {
		t.Fatalf("expected encrypted values after BeforeSave, got auth=%q token=%q jwt=%q", account.Auth, account.Token, account.JWTToken)
	}
	if !security.IsEncryptedValue(account.Auth) || !security.IsEncryptedValue(account.Token) || !security.IsEncryptedValue(account.JWTToken) {
		t.Fatalf("expected encrypted prefixes after BeforeSave")
	}

	if err := account.AfterSave(nil); err != nil {
		t.Fatalf("AfterSave() error = %v", err)
	}
	if account.Auth != "plain-auth" || account.Token != "plain-token" || account.JWTToken != "plain-jwt" {
		t.Fatalf("expected plaintext after AfterSave, got auth=%q token=%q jwt=%q", account.Auth, account.Token, account.JWTToken)
	}
}

func TestExchangeRuleSensitiveFieldHooks(t *testing.T) {
	t.Setenv("DATA_ENCRYPTION_KEY", "0123456789abcdef0123456789abcdef")
	security.ResetFieldCryptoForTests()

	rule := &ExchangeRule{Auth: "rule-auth", Token: "rule-token", JWTToken: "rule-jwt"}
	if err := rule.BeforeSave(nil); err != nil {
		t.Fatalf("BeforeSave() error = %v", err)
	}
	if rule.Auth == "rule-auth" || rule.Token == "rule-token" || rule.JWTToken == "rule-jwt" {
		t.Fatalf("expected encrypted values after BeforeSave")
	}

	if err := rule.AfterFind(nil); err != nil {
		t.Fatalf("AfterFind() error = %v", err)
	}
	if rule.Auth != "rule-auth" || rule.Token != "rule-token" || rule.JWTToken != "rule-jwt" {
		t.Fatalf("expected plaintext after AfterFind, got auth=%q token=%q jwt=%q", rule.Auth, rule.Token, rule.JWTToken)
	}
}
