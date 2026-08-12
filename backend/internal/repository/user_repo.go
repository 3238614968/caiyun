package repository

import (
	"caiyun/internal/models"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type UserRepository struct {
	db *gorm.DB
}

type loginFailLockRecord struct {
	KeyHash     string     `gorm:"column:key_hash;primaryKey"`
	FailCount   int        `gorm:"column:fail_count"`
	LockedUntil *time.Time `gorm:"column:locked_until"`
	ExpiresAt   time.Time  `gorm:"column:expires_at"`
	CreatedAt   time.Time  `gorm:"column:created_at"`
	UpdatedAt   time.Time  `gorm:"column:updated_at"`
}

func (loginFailLockRecord) TableName() string {
	return "login_fail_locks"
}

func NewUserRepository(db *gorm.DB) *UserRepository {
	return &UserRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *UserRepository) WithContext(ctx context.Context) *UserRepository {
	if ctx == nil {
		return r
	}
	return &UserRepository{db: r.db.WithContext(ctx)}
}

// Create 创建用户
func (r *UserRepository) Create(user *models.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	prepareUserIdentity(user)
	return mapUserIdentityWriteError(r.db.Create(user).Error)
}

// FindByID 根据ID查找用户
func (r *UserRepository) FindByID(id uint) (*models.User, error) {
	var user models.User
	err := r.db.First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByIDForUpdate returns one user while holding an exclusive row lock until
// the current transaction ends.  It is intentionally kept separate from
// FindByID so ordinary reads do not accidentally turn into locking reads.
//
// The lock clause is emitted for MySQL/InnoDB. SQLite, which is used by the
// focused service tests, safely ignores FOR UPDATE and still runs the enclosing
// transaction atomically.
func (r *UserRepository) FindByIDForUpdate(id uint) (*models.User, error) {
	var user models.User
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).First(&user, id).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByUsername 根据用户名查找用户
func (r *UserRepository) FindByUsername(username string) (*models.User, error) {
	var user models.User
	err := r.db.Where("normalized_username = ?", NormalizeUsername(username)).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByEmail 根据邮箱查找用户
func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	normalized := NormalizeEmail(email)
	if normalized == nil {
		return nil, gorm.ErrRecordNotFound
	}
	var user models.User
	err := r.db.Where("normalized_email = ?", *normalized).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(user *models.User) error {
	if user == nil {
		return fmt.Errorf("user is nil")
	}
	prepareUserIdentity(user)
	return mapUserIdentityWriteError(r.db.Save(user).Error)
}

// UpdatePasswordAndRevokeSessions 更新用户密码并递增会话版本，使旧 JWT 立即失效。
func (r *UserRepository) UpdatePasswordAndRevokeSessions(userID uint, hashedPassword string) error {
	return r.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"password":      hashedPassword,
			"token_version": gorm.Expr("token_version + ?", 1),
		}).Error
}

// UpdateRoleAndRevokeSessions 更新用户角色并递增会话版本，使旧 JWT 立即失效。
func (r *UserRepository) UpdateRoleAndRevokeSessions(userID uint, role string) error {
	return r.db.Model(&models.User{}).
		Where("id = ?", userID).
		Updates(map[string]interface{}{
			"role":          role,
			"token_version": gorm.Expr("token_version + ?", 1),
		}).Error
}

// UpdateRoleAndRevokeSessionsIfCurrentRole updates a role only when it still
// has the expected previous value. The conditional write is a final fence for
// callers that made an authorization decision from a locked row: a stale
// decision cannot overwrite a role changed by another transaction.
func (r *UserRepository) UpdateRoleAndRevokeSessionsIfCurrentRole(userID uint, currentRole, nextRole string) (bool, error) {
	result := r.db.Model(&models.User{}).
		Where("id = ? AND role = ?", userID, currentRole).
		Updates(map[string]interface{}{
			"role":          nextRole,
			"token_version": gorm.Expr("token_version + ?", 1),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected == 1, nil
}

// CountByRole 统计指定角色用户数量。
func (r *UserRepository) CountByRole(role string) (int64, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("role = ?", role).Count(&count).Error
	return count, err
}

// LockByRoleForUpdate locks all non-deleted users with a role in a stable
// order. Administrative role changes call this before looking up their target,
// so concurrent demotions/deletions serialize on the same admin-row set rather
// than each transaction observing an independently stale admin count.
func (r *UserRepository) LockByRoleForUpdate(role string) ([]*models.User, error) {
	var users []*models.User
	err := r.db.Clauses(clause.Locking{Strength: "UPDATE"}).
		Where("role = ?", role).
		Order("id ASC").
		Find(&users).Error
	return users, err
}

// Delete 删除用户
func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&models.User{}, id).Error
}

// List 列出所有用户
func (r *UserRepository) List(offset, limit int) ([]*models.User, int64, error) {
	return r.Search("", offset, limit)
}

// Search lists users by username/email. SQL LIKE metacharacters are escaped so
// an administrator's input remains a literal keyword rather than a wildcard.
func (r *UserRepository) Search(keyword string, offset, limit int) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64
	query := r.db.Model(&models.User{})
	keyword = strings.TrimSpace(keyword)
	if keyword != "" {
		escaped := strings.NewReplacer("\\", "\\\\", "%", "\\%", "_", "\\_").Replace(keyword)
		pattern := "%" + escaped + "%"
		query = query.Where("username LIKE ? OR email LIKE ?", pattern, pattern)
	}
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Order("created_at DESC, id DESC").Offset(offset).Limit(limit).Find(&users).Error
	return users, total, err
}

// FindByRole 根据角色查找用户
func (r *UserRepository) FindByRole(role string) ([]*models.User, error) {
	var users []*models.User
	err := r.db.Where("role = ?", role).Find(&users).Error
	return users, err
}

// ExistsByUsername 检查用户名是否存在
func (r *UserRepository) ExistsByUsername(username string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("normalized_username = ?", NormalizeUsername(username)).Count(&count).Error
	return count > 0, err
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(email string) (bool, error) {
	normalized := NormalizeEmail(email)
	if normalized == nil {
		return false, nil
	}
	var count int64
	err := r.db.Model(&models.User{}).Where("normalized_email = ?", *normalized).Count(&count).Error
	return count > 0, err
}

func prepareUserIdentity(user *models.User) {
	user.Username = strings.TrimSpace(user.Username)
	user.NormalizedUsername = NormalizeUsername(user.Username)
	user.Email = strings.TrimSpace(user.Email)
	user.NormalizedEmail = NormalizeEmail(user.Email)
}

// AnonymizeAndDelete removes reusable identity data and revokes all token
// versions before soft-deleting the user. It must be called inside a Unit of
// Work together with dependent-data cleanup.
func (r *UserRepository) AnonymizeAndDelete(id uint) error {
	stamp := fmt.Sprintf("deleted-%d-%d", id, time.Now().UnixNano())
	digest := sha256.Sum256([]byte(stamp))
	anonymizedUsername := fmt.Sprintf("deleted-%d-%s", id, hex.EncodeToString(digest[:6]))
	result := r.db.Model(&models.User{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"username":            anonymizedUsername,
			"normalized_username": NormalizeUsername(anonymizedUsername),
			"email":               "",
			"normalized_email":    nil,
			"password":            hex.EncodeToString(digest[:]),
			"token_version":       gorm.Expr("token_version + ?", 1),
		})
	if result.Error != nil {
		return mapUserIdentityWriteError(result.Error)
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return r.db.Delete(&models.User{}, id).Error
}

func (r *UserRepository) GetLoginFailure(keyHash string) (int, time.Time, error) {
	now := time.Now()
	var record loginFailLockRecord
	err := r.db.Where("key_hash = ? AND expires_at > ?", keyHash, now).First(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, time.Time{}, nil
	}
	if err != nil {
		return 0, time.Time{}, err
	}
	lockedUntil := time.Time{}
	if record.LockedUntil != nil {
		lockedUntil = *record.LockedUntil
	}
	return record.FailCount, lockedUntil, nil
}

func (r *UserRepository) RecordLoginFailure(keyHash string, maxAttempts int, window, lockTTL time.Duration) error {
	if maxAttempts <= 0 {
		maxAttempts = 5
	}
	if window <= 0 {
		window = 15 * time.Minute
	}
	if lockTTL <= 0 {
		lockTTL = window
	}

	return r.db.Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		var record loginFailLockRecord
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("key_hash = ?", keyHash).First(&record).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return tx.Create(newLoginFailLockRecord(keyHash, now, maxAttempts, window, lockTTL)).Error
		}
		if err != nil {
			return err
		}
		if record.ExpiresAt.Before(now) {
			reset := newLoginFailLockRecord(keyHash, now, maxAttempts, window, lockTTL)
			record.FailCount = reset.FailCount
			record.LockedUntil = reset.LockedUntil
			record.ExpiresAt = reset.ExpiresAt
			return tx.Save(&record).Error
		}

		record.FailCount++
		record.ExpiresAt = now.Add(window)
		if record.FailCount >= maxAttempts {
			value := now.Add(lockTTL)
			record.LockedUntil = &value
			record.ExpiresAt = value
		}
		return tx.Save(&record).Error
	})
}

func newLoginFailLockRecord(keyHash string, now time.Time, maxAttempts int, window, lockTTL time.Duration) *loginFailLockRecord {
	failCount := 1
	expiresAt := now.Add(window)
	var lockedUntil *time.Time
	if failCount >= maxAttempts {
		value := now.Add(lockTTL)
		lockedUntil = &value
		expiresAt = value
	}
	return &loginFailLockRecord{
		KeyHash:     keyHash,
		FailCount:   failCount,
		LockedUntil: lockedUntil,
		ExpiresAt:   expiresAt,
	}
}

func (r *UserRepository) ClearLoginFailure(keyHash string) error {
	return r.db.Delete(&loginFailLockRecord{}, "key_hash = ?", keyHash).Error
}
