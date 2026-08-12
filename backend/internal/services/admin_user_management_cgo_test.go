//go:build cgo

package services

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"caiyun/internal/models"
	"caiyun/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type adminRoleFixture struct {
	db      *gorm.DB
	service *AdminService
	admins  []*models.User
	user    *models.User
}

func newAdminRoleFixture(t *testing.T, adminCount int) *adminRoleFixture {
	t.Helper()
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&models.User{}); err != nil {
		t.Fatalf("migrate users: %v", err)
	}
	userRepo := repository.NewUserRepository(db)
	admins := make([]*models.User, 0, adminCount)
	for i := 0; i < adminCount; i++ {
		admin := &models.User{Username: fmt.Sprintf("role-admin-%d", i), Password: "hash", Role: "admin"}
		if err := userRepo.Create(admin); err != nil {
			t.Fatalf("create admin %d: %v", i, err)
		}
		admins = append(admins, admin)
	}
	user := &models.User{Username: "role-user", Password: "hash", Role: "user"}
	if err := userRepo.Create(user); err != nil {
		t.Fatalf("create standard user: %v", err)
	}
	return &adminRoleFixture{
		db:      db,
		service: NewAdminService(userRepo, repository.NewAccountRepository(db), repository.NewTaskLogRepository(db), repository.NewTaskConfigRepository(db)),
		admins:  admins,
		user:    user,
	}
}

func (f *adminRoleFixture) adminCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := f.db.Model(&models.User{}).Where("role = ?", "admin").Count(&count).Error; err != nil {
		t.Fatalf("count admins: %v", err)
	}
	return count
}

func TestAdminUpdateUserRoleRejectsLastAdministratorWithinTransaction(t *testing.T) {
	f := newAdminRoleFixture(t, 1)
	err := f.service.UpdateUserRole(f.admins[0].ID, f.user.ID, &UpdateUserRoleRequest{Role: "user"})
	if !errors.Is(err, ErrCannotRemoveLastAdmin) {
		t.Fatalf("UpdateUserRole() error = %v, want ErrCannotRemoveLastAdmin", err)
	}
	if got := f.adminCount(t); got != 1 {
		t.Fatalf("admin count after rejected demotion = %d, want 1", got)
	}

	var persisted models.User
	if err := f.db.First(&persisted, f.admins[0].ID).Error; err != nil {
		t.Fatalf("load administrator after rejected demotion: %v", err)
	}
	if persisted.Role != "admin" || persisted.TokenVersion != 0 {
		t.Fatalf("rejected demotion changed user: %#v", persisted)
	}
}

func TestAdminUpdateUserRoleKeepsSessionRevocationInsideCommittedRoleChange(t *testing.T) {
	f := newAdminRoleFixture(t, 2)
	target := f.admins[0]
	actor := f.admins[1]
	if err := f.service.UpdateUserRole(target.ID, actor.ID, &UpdateUserRoleRequest{Role: "user"}); err != nil {
		t.Fatalf("UpdateUserRole() error: %v", err)
	}
	if got := f.adminCount(t); got != 1 {
		t.Fatalf("admin count after demotion = %d, want 1", got)
	}

	var persisted models.User
	if err := f.db.First(&persisted, target.ID).Error; err != nil {
		t.Fatalf("load demoted user: %v", err)
	}
	if persisted.Role != "user" || persisted.TokenVersion != 1 {
		t.Fatalf("role/session update was not committed atomically: %#v", persisted)
	}

	err := f.service.UpdateUserRole(actor.ID, f.user.ID, &UpdateUserRoleRequest{Role: "user"})
	if !errors.Is(err, ErrCannotRemoveLastAdmin) {
		t.Fatalf("second demotion error = %v, want ErrCannotRemoveLastAdmin", err)
	}
	if got := f.adminCount(t); got != 1 {
		t.Fatalf("admin count after second demotion = %d, want 1", got)
	}
}

func TestAdminDeleteUserContextRejectsLastAdministratorWithinTransaction(t *testing.T) {
	f := newAdminRoleFixture(t, 1)
	err := f.service.DeleteUserContext(t.Context(), f.admins[0].ID, f.user.ID)
	if !errors.Is(err, ErrCannotRemoveLastAdmin) {
		t.Fatalf("DeleteUserContext() error = %v, want ErrCannotRemoveLastAdmin", err)
	}
	if got := f.adminCount(t); got != 1 {
		t.Fatalf("admin count after rejected delete = %d, want 1", got)
	}
	if err := f.db.First(&models.User{}, f.admins[0].ID).Error; err != nil {
		t.Fatalf("last administrator was deleted: %v", err)
	}
}
