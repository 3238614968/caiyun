package adminbootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/security"
	"caiyun/internal/services"
	"caiyun/pkg/database"

	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Run creates the first administrator without exposing an HTTP-only privilege
// escalation path. It is intentionally idempotent: an existing administrator
// makes the command succeed without changing credentials.
func Run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) != 0 {
		return fmt.Errorf("bootstrap-admin 不支持位置参数")
	}
	bootstrap.LoadEnvFile()
	if err := security.ValidateFieldCryptoConfig(); err != nil {
		return fmt.Errorf("数据加密配置校验失败: %w", err)
	}
	closeLogger := bootstrap.ConfigureStandardLogger("admin-bootstrap")
	defer closeLogger()

	username := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_USERNAME"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	email := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_EMAIL"))
	if username == "" || password == "" {
		return errors.New("必须设置 BOOTSTRAP_ADMIN_USERNAME 和 BOOTSTRAP_ADMIN_PASSWORD")
	}
	if err := services.ValidatePasswordStrength(username, password); err != nil {
		return fmt.Errorf("管理员密码不符合策略: %w", err)
	}

	db, err := database.NewMySQL(database.Config{
		Host:            bootstrap.GetEnv("DB_HOST", "localhost"),
		Port:            bootstrap.GetEnv("DB_PORT", "3306"),
		User:            bootstrap.GetEnv("DB_USER", "caiyun_app"),
		Password:        bootstrap.GetEnv("DB_PASSWORD", ""),
		DBName:          bootstrap.GetEnv("DB_NAME", "caiyun"),
		MaxIdleConns:    2,
		MaxOpenConns:    4,
		ConnMaxLifetime: time.Hour,
		ConnMaxIdleTime: 10 * time.Minute,
	})
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	defer func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr == nil {
			_ = sqlDB.Close()
		}
	}()

	if err := ensureAdmin(ctx, db, username, email, password); err != nil {
		return err
	}
	log.Printf("管理员初始化完成: username=%s", username)
	return nil
}

func ensureAdmin(ctx context.Context, db *gorm.DB, username, email, password string) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var admins int64
		if err := tx.Model(&models.User{}).Where("role = ?", "admin").Count(&admins).Error; err != nil {
			return fmt.Errorf("查询管理员失败: %w", err)
		}

		var existing models.User
		err := tx.Where("normalized_username = ?", repository.NormalizeUsername(username)).First(&existing).Error
		switch {
		case err == nil:
			if existing.Role == "admin" {
				return nil
			}
			return fmt.Errorf("管理员用户名 %q 已存在但不是管理员；请更换 BOOTSTRAP_ADMIN_USERNAME", username)
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return fmt.Errorf("查询管理员用户名失败: %w", err)
		}
		if admins > 0 {
			return fmt.Errorf("系统已有管理员，拒绝自动创建第二个 bootstrap 管理员")
		}

		hashed, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
		if err != nil {
			return fmt.Errorf("生成管理员密码失败: %w", err)
		}
		user := &models.User{
			Username: username,
			Password: string(hashed),
			Email:    email,
			Role:     "admin",
		}
		if err := repository.NewUserRepository(tx).Create(user); err != nil {
			return fmt.Errorf("创建管理员失败: %w", err)
		}
		return nil
	})
}
