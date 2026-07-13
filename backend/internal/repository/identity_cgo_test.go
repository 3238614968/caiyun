//go:build cgo

package repository

import (
	"errors"
	"testing"

	"caiyun/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func openIdentityTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	return db
}

func TestUserRepositoryNormalizedIdentityAndDuplicateMapping(t *testing.T) {
	repo := NewUserRepository(openIdentityTestDB(t))
	alice := &models.User{Username: "  Alice  ", Password: "hash", Email: "Local@EXAMPLE.COM"}
	if err := repo.Create(alice); err != nil {
		t.Fatalf("create alice: %v", err)
	}
	if alice.Username != "Alice" || alice.NormalizedUsername != "alice" {
		t.Fatalf("unexpected username normalization: %#v", alice)
	}
	if alice.NormalizedEmail == nil || *alice.NormalizedEmail != "Local@example.com" {
		t.Fatalf("unexpected email normalization: %#v", alice.NormalizedEmail)
	}
	if _, err := repo.FindByUsername(" ALICE "); err != nil {
		t.Fatalf("find normalized username: %v", err)
	}
	if _, err := repo.FindByEmail("Local@Example.Com"); err != nil {
		t.Fatalf("find normalized email: %v", err)
	}

	if err := repo.Create(&models.User{Username: "ALICE", Password: "hash", Email: "other@example.com"}); !errors.Is(err, ErrDuplicateUsername) {
		t.Fatalf("duplicate username error = %v", err)
	}
	if err := repo.Create(&models.User{Username: "bob", Password: "hash", Email: "Local@Example.COM"}); !errors.Is(err, ErrDuplicateEmail) {
		t.Fatalf("duplicate email error = %v", err)
	}

	for _, username := range []string{"no-email-1", "no-email-2"} {
		user := &models.User{Username: username, Password: "hash", Email: "  "}
		if err := repo.Create(user); err != nil {
			t.Fatalf("create %s: %v", username, err)
		}
		if user.NormalizedEmail != nil {
			t.Fatalf("%s normalized empty email = %#v", username, user.NormalizedEmail)
		}
	}
}

func TestUnitOfWorkRollbackAndCommit(t *testing.T) {
	db := openIdentityTestDB(t)
	uow := NewUnitOfWork(db)
	rollbackErr := errors.New("rollback")
	err := uow.WithinTransaction(t.Context(), func(repos TransactionRepositories) error {
		if err := repos.User.Create(&models.User{Username: "rolled-back", Password: "hash"}); err != nil {
			return err
		}
		return rollbackErr
	})
	if !errors.Is(err, rollbackErr) {
		t.Fatalf("transaction error = %v", err)
	}
	var count int64
	if err := db.Model(&models.User{}).Where("normalized_username = ?", "rolled-back").Count(&count).Error; err != nil {
		t.Fatalf("count rollback user: %v", err)
	}
	if count != 0 {
		t.Fatalf("rollback user count = %d", count)
	}

	if err := uow.WithinTransaction(t.Context(), func(repos TransactionRepositories) error {
		return repos.User.Create(&models.User{Username: "committed", Password: "hash"})
	}); err != nil {
		t.Fatalf("commit transaction: %v", err)
	}
	if err := db.Model(&models.User{}).Where("normalized_username = ?", "committed").Count(&count).Error; err != nil {
		t.Fatalf("count committed user: %v", err)
	}
	if count != 1 {
		t.Fatalf("committed user count = %d", count)
	}
}
