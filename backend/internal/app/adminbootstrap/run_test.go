//go:build cgo

package adminbootstrap

import (
	"context"
	"testing"

	"caiyun/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func bootstrapTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestEnsureAdminIsIdempotent(t *testing.T) {
	db := bootstrapTestDB(t)
	if err := ensureAdmin(context.Background(), db, "admin", "admin@example.com", "AdminPass123"); err != nil {
		t.Fatal(err)
	}
	if err := ensureAdmin(context.Background(), db, "admin", "admin@example.com", "DifferentPass123"); err != nil {
		t.Fatalf("second bootstrap should be idempotent: %v", err)
	}

	var user models.User
	if err := db.Where("username = ?", "admin").First(&user).Error; err != nil {
		t.Fatal(err)
	}
	if user.Role != "admin" {
		t.Fatalf("role = %q, want admin", user.Role)
	}
}

func TestEnsureAdminDoesNotPromoteExistingUser(t *testing.T) {
	db := bootstrapTestDB(t)
	if err := db.Create(&models.User{Username: "admin", NormalizedUsername: "admin", Password: "hash", Role: "user"}).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureAdmin(context.Background(), db, "admin", "admin@example.com", "AdminPass123"); err == nil {
		t.Fatal("expected existing user conflict")
	}
}
