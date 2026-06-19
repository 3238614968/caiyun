package repository

import (
	"caiyun/internal/models"
	"context"
	"errors"
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
	return r.db.Create(user).Error
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

// FindByUsername 根据用户名查找用户
func (r *UserRepository) FindByUsername(username string) (*models.User, error) {
	var user models.User
	err := r.db.Where("username = ?", username).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// FindByEmail 根据邮箱查找用户
func (r *UserRepository) FindByEmail(email string) (*models.User, error) {
	var user models.User
	err := r.db.Where("email = ?", email).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update 更新用户
func (r *UserRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
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

// Delete 删除用户
func (r *UserRepository) Delete(id uint) error {
	return r.db.Delete(&models.User{}, id).Error
}

// List 列出所有用户
func (r *UserRepository) List(offset, limit int) ([]*models.User, int64, error) {
	var users []*models.User
	var total int64

	if err := r.db.Model(&models.User{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Offset(offset).Limit(limit).Find(&users).Error
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
	err := r.db.Model(&models.User{}).Where("username = ?", username).Count(&count).Error
	return count > 0, err
}

// ExistsByEmail 检查邮箱是否存在
func (r *UserRepository) ExistsByEmail(email string) (bool, error) {
	var count int64
	err := r.db.Model(&models.User{}).Where("email = ?", email).Count(&count).Error
	return count > 0, err
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
