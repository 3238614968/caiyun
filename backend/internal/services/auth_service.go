package services

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"caiyun/internal/models"
	"caiyun/pkg/jwt"
)

var (
	// 注意：ErrUserNotFound 已在 admin_service.go 中定义
	ErrInvalidCredentials         = errors.New("用户名或密码错误")
	ErrUserExists                 = errors.New("用户已存在")
	ErrEmailExists                = errors.New("邮箱已被注册")
	ErrWeakPassword               = errors.New("密码强度不足")
	ErrInvalidRecoveryInfo        = errors.New("用户名或邮箱不匹配")
	ErrEmailServiceDisabled       = errors.New("邮箱服务未配置")
	ErrResetCodeTooFrequent       = errors.New("验证码发送过于频繁")
	ErrInvalidResetCode           = errors.New("验证码错误或已过期")
	ErrAccountLocked              = errors.New("登录失败次数过多，请稍后再试")
	ErrLoginProtectionUnavailable = errors.New("登录保护服务暂不可用")
	ErrInvalidRefreshToken        = errors.New("刷新凭证无效或已过期")
	ErrRefreshTokenReuse          = errors.New("检测到刷新凭证重放，会话已撤销")
)

type SMTPConfig struct {
	Host     string
	Port     string
	Username string
	Password string
	From     string
	FromName string
	UseTLS   bool
}

type PasswordResetConfig struct {
	SMTP         SMTPConfig
	CodeTTL      time.Duration
	SendCooldown time.Duration
	MaxAttempts  int
}

type passwordResetCache interface {
	Set(key string, value interface{}, expiration time.Duration) error
	Get(key string, dest interface{}) error
	Del(keys ...string) error
}

type passwordResetLockCache interface {
	SetNX(key string, value interface{}, expiration time.Duration) (bool, error)
	DelIfValue(key, value string) (bool, error)
}

type authUserRepository interface {
	Create(user *models.User) error
	FindByID(id uint) (*models.User, error)
	FindByUsername(username string) (*models.User, error)
	Update(user *models.User) error
	UpdatePasswordAndRevokeSessions(userID uint, hashedPassword string) error
	ExistsByUsername(username string) (bool, error)
	ExistsByEmail(email string) (bool, error)
}

type refreshSessionRepository interface {
	Create(ctx context.Context, session *models.RefreshSession) error
	Rotate(ctx context.Context, oldTokenHash string, replacement *models.RefreshSession, now time.Time) (*models.RefreshSession, error)
	IsActive(ctx context.Context, sessionID string, userID uint, now time.Time) (bool, error)
	Revoke(ctx context.Context, sessionID string, userID uint, now time.Time) error
	RevokeAll(ctx context.Context, userID uint, now time.Time) error
}

// AuthServiceOption configures optional authentication capabilities without
// breaking the legacy constructors used by tests and offline tools.
type AuthServiceOption func(*AuthService)

// WithRefreshSessionRepository enables rotating, server-revocable refresh
// sessions. Access tokens remain short lived while refresh credentials are
// stored only as SHA-256 digests.
func WithRefreshSessionRepository(repo refreshSessionRepository, refreshExpiry time.Duration) AuthServiceOption {
	return func(service *AuthService) {
		service.sessionRepo = repo
		if refreshExpiry > 0 {
			service.refreshExpiry = refreshExpiry
		}
	}
}

type AuthService struct {
	userRepo       authUserRepository
	jwtMgr         *jwt.Manager
	jwtExpiry      time.Duration
	resetConfig    PasswordResetConfig
	resetCodeCache passwordResetCache
	loginLockCache passwordResetCache
	loginLockStore loginLockStore
	sessionRepo    refreshSessionRepository
	refreshExpiry  time.Duration
	resetCodeMu    sync.Mutex
}

func NewAuthService(
	userRepo authUserRepository,
	jwtMgr *jwt.Manager,
	jwtExpiry time.Duration,
) *AuthService {
	return NewAuthServiceWithPasswordReset(userRepo, jwtMgr, jwtExpiry, PasswordResetConfig{})
}

func NewAuthServiceWithPasswordReset(
	userRepo authUserRepository,
	jwtMgr *jwt.Manager,
	jwtExpiry time.Duration,
	resetConfig PasswordResetConfig,
) *AuthService {
	return NewAuthServiceWithPasswordResetCache(userRepo, jwtMgr, jwtExpiry, resetConfig, nil)
}

func NewAuthServiceWithPasswordResetCache(
	userRepo authUserRepository,
	jwtMgr *jwt.Manager,
	jwtExpiry time.Duration,
	resetConfig PasswordResetConfig,
	resetCodeCache passwordResetCache,
	options ...AuthServiceOption,
) *AuthService {
	if resetConfig.CodeTTL <= 0 {
		resetConfig.CodeTTL = 10 * time.Minute
	}
	if resetConfig.SendCooldown <= 0 {
		resetConfig.SendCooldown = time.Minute
	}
	if resetConfig.MaxAttempts <= 0 {
		resetConfig.MaxAttempts = 5
	}
	var dbLoginLockStore loginLockStore
	if store, ok := userRepo.(loginLockStore); ok {
		dbLoginLockStore = store
	}

	service := &AuthService{
		userRepo:       userRepo,
		jwtMgr:         jwtMgr,
		jwtExpiry:      jwtExpiry,
		resetConfig:    resetConfig,
		resetCodeCache: resetCodeCache,
		loginLockCache: resetCodeCache, // 登录锁定复用同一 Redis 缓存
		loginLockStore: dbLoginLockStore,
		refreshExpiry:  30 * 24 * time.Hour,
	}
	for _, option := range options {
		if option != nil {
			option(service)
		}
	}
	return service
}

type passwordResetCodeRecord struct {
	CodeHash  string
	ExpiresAt time.Time
	SentAt    time.Time
	Attempts  int
}

func passwordResetKey(username, email string) string {
	return "caiyun:password_reset:" + strings.ToLower(strings.TrimSpace(username)) + ":" + strings.ToLower(strings.TrimSpace(email))
}

func (s *AuthService) isEmailServiceEnabled() bool {
	smtpConfig := s.resetConfig.SMTP
	return strings.TrimSpace(smtpConfig.Host) != "" &&
		strings.TrimSpace(smtpConfig.Port) != "" &&
		strings.TrimSpace(smtpConfig.From) != ""
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Username string `json:"username" binding:"required,min=3,max=50"`
	Password string `json:"password" binding:"required,min=8"`
	Email    string `json:"email" binding:"omitempty,email"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	Token            string       `json:"-"`
	ExpiresAt        int64        `json:"expires_at"`
	RefreshToken     string       `json:"-"`
	RefreshExpiresAt int64        `json:"refresh_expires_at,omitempty"`
	User             *models.User `json:"user"`
}

// SessionMetadata helps users identify a session but is never used as an
// authentication factor.
type SessionMetadata struct {
	DeviceInfo string
	ClientIP   string
}
