//go:build cgo

package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"testing"
)

func TestAccountRemoveAndReaddKeepsIDLogsStatisticsAndRules(t *testing.T) {
	f := newAdminCleanupFixture(t)
	if err := f.db.Model(&models.Account{}).Where("id = ?", f.account.ID).Update("cloud_count", 1234).Error; err != nil {
		t.Fatal(err)
	}
	service := NewAccountService(repository.NewAccountRepository(f.db), repository.NewUserRepository(f.db), nil, nil)
	provider := &stubTokenProvider{}
	service.SetTokenProvider(provider)
	if err := service.DeleteAccount(f.user.ID, f.account.ID); err != nil {
		t.Fatal(err)
	}
	account, err := service.CreateAccount(f.user.ID, &CreateAccountRequest{Phone: f.account.Phone, Auth: "new-login-auth"})
	if err != nil {
		t.Fatal(err)
	}
	if account.ID != f.account.ID || account.CloudCount != 1234 || !account.IsActive || account.DeletedAt.Valid {
		t.Fatalf("login recreated identity instead of restoring history: %+v", account)
	}
	assertRowCount(t, f.db, &models.Account{}, "id = ?", []interface{}{f.account.ID}, 1)
	assertRowCount(t, f.db, &models.TaskLog{}, "account_id = ?", []interface{}{f.account.ID}, 1)
	assertRowCount(t, f.db, &models.CloudStats{}, "account_id = ?", []interface{}{f.account.ID}, 1)
	assertRowCount(t, f.db, &models.ExchangeTask{}, "id = ?", []interface{}{f.task.ID}, 1)
	var rule models.ExchangeAccount
	if err := f.db.First(&rule, f.rule.ID).Error; err != nil {
		t.Fatal(err)
	}
	if rule.Auth != "new-login-auth" || rule.JWTToken != "" {
		t.Fatal("restored login was not synchronized to the original rule")
	}
	if len(provider.cleared) != 2 {
		t.Fatalf("cache invalidations=%v", provider.cleared)
	}
}

func TestAccountLoginReplacesUnreadableOldCredentials(t *testing.T) {
	f := newAdminCleanupFixture(t)
	if err := f.db.Model(&models.Account{}).Where("id = ?", f.account.ID).
		UpdateColumns(map[string]interface{}{"auth": "enc:v1:ZmFrZQ", "token": "enc:v1:ZmFrZQ", "jwt_token": "enc:v1:ZmFrZQ"}).Error; err != nil {
		t.Fatal(err)
	}
	service := NewAccountService(repository.NewAccountRepository(f.db), repository.NewUserRepository(f.db), nil, nil)
	account, err := service.CreateAccount(f.user.ID, &CreateAccountRequest{Phone: f.account.Phone, Auth: "fresh-auth"})
	if err != nil || account.ID != f.account.ID || account.Auth != "fresh-auth" {
		t.Fatalf("credential replacement required decrypting obsolete data: %+v %v", account, err)
	}
}

func TestAccountLoginRollsBackWhenRuleSyncFails(t *testing.T) {
	f := newAdminCleanupFixture(t)
	if err := f.db.Exec("CREATE TRIGGER fail_login_sync BEFORE UPDATE OF auth ON exchange_rules BEGIN SELECT RAISE(ABORT, 'forced login sync failure'); END").Error; err != nil {
		t.Fatal(err)
	}
	service := NewAccountService(repository.NewAccountRepository(f.db), repository.NewUserRepository(f.db), nil, nil)
	if _, err := service.CreateAccount(f.user.ID, &CreateAccountRequest{Phone: f.account.Phone, Auth: "fresh-auth"}); err == nil {
		t.Fatal("partial login sync accepted")
	}
	var account models.Account
	if err := f.db.First(&account, f.account.ID).Error; err != nil {
		t.Fatal(err)
	}
	if account.Auth != "secret-auth" {
		t.Fatal("failed rule sync committed account credentials")
	}
}
