package services

import (
	"context"

	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/security/authcache"
	"caiyun/pkg/jwt"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/smtp"
	"strings"
	"time"
	"unicode"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	// 注意：ErrUserNotFound 已在 admin_service.go 中定义
	ErrInvalidCredentials   = errors.New("用户名或密码错误")
	ErrUserExists           = errors.New("用户已存在")
	ErrEmailExists          = errors.New("邮箱已被注册")
	ErrWeakPassword         = errors.New("密码强度不足")
	ErrInvalidRecoveryInfo  = errors.New("用户名或邮箱不匹配")
	ErrEmailServiceDisabled = errors.New("邮箱服务未配置")
	ErrResetCodeTooFrequent = errors.New("验证码发送过于频繁")
	ErrInvalidResetCode     = errors.New("验证码错误或已过期")
	ErrAccountLocked        = errors.New("登录失败次数过多，请稍后再试")
	ErrInvalidRefreshToken  = errors.New("刷新凭证无效或已过期")
	ErrRefreshTokenReuse    = errors.New("检测到刷新凭证重放，会话已撤销")
)

const (
	// loginLockMaxAttempts 是连续登录失败达到该阈值后锁定账号。
	loginLockMaxAttempts = 5
	// loginLockTTL 是锁定持续时间。
	loginLockTTL = 15 * time.Minute
	// loginFailWindow 是失败计数窗口。
	loginFailWindow = loginLockTTL
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

type authUserRepository interface {
	Create(user *models.User) error
	FindByID(id uint) (*models.User, error)
	FindByUsername(username string) (*models.User, error)
	Update(user *models.User) error
	UpdatePasswordAndRevokeSessions(userID uint, hashedPassword string) error
	ExistsByUsername(username string) (bool, error)
	ExistsByEmail(email string) (bool, error)
}

type loginLockStore interface {
	GetLoginFailure(keyHash string) (int, time.Time, error)
	RecordLoginFailure(keyHash string, maxAttempts int, window, lockTTL time.Duration) error
	ClearLoginFailure(keyHash string) error
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
	Password string `json:"password" binding:"required,min=6"`
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
}

// Register 用户注册（保留兼容；HTTP 调用应使用 RegisterContext）。
func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
	return s.RegisterContext(context.Background(), req, SessionMetadata{})
}

// RegisterContext registers a user and creates a server-revocable session.
func (s *AuthService) RegisterContext(ctx context.Context, req *RegisterRequest, metadata SessionMetadata) (*AuthResponse, error) {
	if err := validatePasswordStrength(req.Username, req.Password); err != nil {
		return nil, err
	}

	// 检查用户名是否已存在
	exists, err := s.userRepo.ExistsByUsername(req.Username)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrUserExists
	}

	// 检查邮箱是否已存在
	if req.Email != "" {
		exists, err = s.userRepo.ExistsByEmail(req.Email)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrEmailExists
		}
	}

	// 加密密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// 创建用户
	user := &models.User{
		Username: req.Username,
		Password: string(hashedPassword),
		Email:    req.Email,
		Role:     "user",
	}

	if err := s.userRepo.Create(user); err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateUsername):
			return nil, ErrUserExists
		case errors.Is(err, repository.ErrDuplicateEmail):
			return nil, ErrEmailExists
		case errors.Is(err, repository.ErrDuplicateIdentity):
			// The database constraint is authoritative. An unclassified identity
			// conflict must still be returned as a stable public conflict rather
			// than leaking the driver error produced by a concurrent registration.
			return nil, ErrUserExists
		default:
			return nil, err
		}
	}

	return s.issueAuthSession(ctx, user, metadata)
}

func validatePasswordStrength(_ string, password string) error {
	if len([]rune(password)) < 6 {
		return fmt.Errorf("%w：长度至少 6 个字符", ErrWeakPassword)
	}
	var hasLetter, hasDigit bool
	for _, r := range password {
		if unicode.IsLetter(r) {
			hasLetter = true
		}
		if unicode.IsDigit(r) {
			hasDigit = true
		}
	}
	if !hasLetter || !hasDigit {
		return fmt.Errorf("%w：需同时包含字母和数字", ErrWeakPassword)
	}
	return nil
}

// Login 用户登录（保留兼容；HTTP 调用应使用 LoginContext）。
func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	return s.LoginContext(context.Background(), req, SessionMetadata{})
}

// LoginContext authenticates credentials and creates a rotating refresh session.
func (s *AuthService) LoginContext(ctx context.Context, req *LoginRequest, metadata SessionMetadata) (*AuthResponse, error) {
	// 登录失败锁定：优先检查 Redis 计数；Redis 不可用时降级到数据库表 login_fail_locks。
	usernameKey := loginLockKey(req.Username)
	if locked, _ := s.getLoginFailCount(usernameKey); locked >= loginLockMaxAttempts {
		return nil, ErrAccountLocked
	}

	// 查找用户
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		// 不暴露用户是否存在：仍递增失败计数，防止通过响应差异枚举账号。
		s.recordLoginFailure(usernameKey)
		return nil, ErrInvalidCredentials
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		s.recordLoginFailure(usernameKey)
		return nil, ErrInvalidCredentials
	}

	// 登录成功：清除失败计数。
	s.clearLoginFailure(usernameKey)

	return s.issueAuthSession(ctx, user, metadata)
}

func loginLockKey(username string) string {
	return "caiyun:login_fail:" + strings.ToLower(strings.TrimSpace(username))
}

func loginLockStoreKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])
}

// getLoginFailCount 返回当前用户名的连续登录失败次数。
// cache 未配置时返回 0，兼容本地调试。
func (s *AuthService) getLoginFailCount(key string) (int, error) {
	if s.loginLockCache != nil {
		var count int
		if err := s.loginLockCache.Get(key, &count); err == nil {
			return count, nil
		}
		// Redis key 不存在或 Redis 异常时继续尝试数据库降级。
	}
	if s.loginLockStore != nil {
		count, lockedUntil, err := s.loginLockStore.GetLoginFailure(loginLockStoreKey(key))
		if err != nil {
			return 0, err
		}
		if !lockedUntil.IsZero() && lockedUntil.After(time.Now()) && count < loginLockMaxAttempts {
			return loginLockMaxAttempts, nil
		}
		return count, nil
	}
	return 0, nil
}

func (s *AuthService) recordLoginFailure(key string) {
	if s.loginLockCache != nil {
		count, _ := s.getLoginFailCount(key)
		count++
		// 锁定窗口内累加；窗口过后 key 自动过期归零。
		ttl := loginFailWindow
		if count >= loginLockMaxAttempts {
			ttl = loginLockTTL
		}
		if err := s.loginLockCache.Set(key, count, ttl); err == nil {
			return
		}
		// Redis 写入失败时降级到数据库。
	}
	if s.loginLockStore != nil {
		_ = s.loginLockStore.RecordLoginFailure(loginLockStoreKey(key), loginLockMaxAttempts, loginFailWindow, loginLockTTL)
	}
}

func (s *AuthService) clearLoginFailure(key string) {
	if s.loginLockCache != nil {
		_ = s.loginLockCache.Del(key)
	}
	if s.loginLockStore != nil {
		_ = s.loginLockStore.ClearLoginFailure(loginLockStoreKey(key))
	}
}

// RefreshToken preserves the legacy service API for non-HTTP callers that do
// not have a refresh credential. Production HTTP refresh uses RefreshWithToken.
func (s *AuthService) RefreshToken(userID uint) (*AuthResponse, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}
	return s.issueAuthSession(context.Background(), user, SessionMetadata{})
}

// RefreshWithToken atomically rotates a refresh credential and issues an
// access JWT bound to the replacement sid. Presenting a consumed credential
// causes the repository to revoke all active sessions for that user.
func (s *AuthService) RefreshWithToken(ctx context.Context, rawRefreshToken string, metadata SessionMetadata) (*AuthResponse, error) {
	if s.sessionRepo == nil {
		return nil, ErrInvalidRefreshToken
	}
	normalized, err := normalizeRefreshCredential(rawRefreshToken)
	if err != nil {
		return nil, ErrInvalidRefreshToken
	}

	newRaw, newHash, err := newRefreshCredential()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	replacement := &models.RefreshSession{
		ID:               uuid.NewString(),
		RefreshTokenHash: newHash,
		DeviceInfo:       normalizeDeviceInfo(metadata.DeviceInfo),
		ExpiresAt:        now.Add(s.refreshExpiry),
	}
	current, err := s.sessionRepo.Rotate(ctx, hashRefreshCredential(normalized), replacement, now)
	if err != nil {
		switch {
		case errors.Is(err, repository.ErrRefreshSessionReused):
			return nil, ErrRefreshTokenReuse
		case errors.Is(err, repository.ErrRefreshSessionNotFound),
			errors.Is(err, repository.ErrRefreshSessionRevoked),
			errors.Is(err, repository.ErrRefreshSessionExpired),
			errors.Is(err, repository.ErrRefreshSessionVersionChanged):
			return nil, ErrInvalidRefreshToken
		default:
			return nil, fmt.Errorf("rotate refresh session: %w", err)
		}
	}

	user, err := s.userRepo.FindByID(current.UserID)
	if err != nil {
		return nil, fmt.Errorf("load refresh session user: %w", err)
	}
	if user.TokenVersion != replacement.TokenVersion {
		_ = s.sessionRepo.RevokeAll(ctx, user.ID, now)
		return nil, ErrInvalidRefreshToken
	}

	accessToken, err := s.jwtMgr.GenerateAccessToken(
		user.ID,
		user.Username,
		user.Role,
		user.TokenVersion,
		replacement.ID,
		s.jwtExpiry,
	)
	if err != nil {
		_ = s.sessionRepo.Revoke(ctx, replacement.ID, user.ID, now)
		return nil, err
	}
	user.Password = ""
	return &AuthResponse{
		Token:            accessToken,
		ExpiresAt:        now.Add(s.jwtExpiry).Unix(),
		RefreshToken:     newRaw,
		RefreshExpiresAt: replacement.ExpiresAt.Unix(),
		User:             user,
	}, nil
}

// RevokeSession immediately invalidates both the refresh credential and every
// access JWT carrying this sid (middleware checks session activity).
func (s *AuthService) RevokeSession(ctx context.Context, userID uint, sessionID string) error {
	if s.sessionRepo == nil || userID == 0 || strings.TrimSpace(sessionID) == "" {
		return nil
	}
	return s.sessionRepo.Revoke(ctx, strings.TrimSpace(sessionID), userID, time.Now())
}

func (s *AuthService) RevokeAllSessions(ctx context.Context, userID uint) error {
	if s.sessionRepo == nil || userID == 0 {
		return nil
	}
	return s.sessionRepo.RevokeAll(ctx, userID, time.Now())
}

func (s *AuthService) issueAuthSession(ctx context.Context, user *models.User, metadata SessionMetadata) (*AuthResponse, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now()
	if s.sessionRepo == nil {
		token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, user.TokenVersion, s.jwtExpiry)
		if err != nil {
			return nil, err
		}
		user.Password = ""
		return &AuthResponse{Token: token, ExpiresAt: now.Add(s.jwtExpiry).Unix(), User: user}, nil
	}

	rawRefreshToken, refreshHash, err := newRefreshCredential()
	if err != nil {
		return nil, err
	}
	session := &models.RefreshSession{
		ID:               uuid.NewString(),
		UserID:           user.ID,
		RefreshTokenHash: refreshHash,
		TokenVersion:     user.TokenVersion,
		DeviceInfo:       normalizeDeviceInfo(metadata.DeviceInfo),
		ExpiresAt:        now.Add(s.refreshExpiry),
	}
	accessToken, err := s.jwtMgr.GenerateAccessToken(
		user.ID,
		user.Username,
		user.Role,
		user.TokenVersion,
		session.ID,
		s.jwtExpiry,
	)
	if err != nil {
		return nil, err
	}
	if err := s.sessionRepo.Create(ctx, session); err != nil {
		return nil, fmt.Errorf("create refresh session: %w", err)
	}

	user.Password = ""
	return &AuthResponse{
		Token:            accessToken,
		ExpiresAt:        now.Add(s.jwtExpiry).Unix(),
		RefreshToken:     rawRefreshToken,
		RefreshExpiresAt: session.ExpiresAt.Unix(),
		User:             user,
	}, nil
}

func newRefreshCredential() (raw string, hash string, err error) {
	var tokenBytes [32]byte
	if _, err = rand.Read(tokenBytes[:]); err != nil {
		return "", "", err
	}
	raw = base64.RawURLEncoding.EncodeToString(tokenBytes[:])
	return raw, hashRefreshCredential(raw), nil
}

func normalizeRefreshCredential(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	decoded, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil || len(decoded) != 32 || base64.RawURLEncoding.EncodeToString(decoded) != raw {
		return "", ErrInvalidRefreshToken
	}
	return raw, nil
}

func hashRefreshCredential(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func normalizeDeviceInfo(value string) string {
	runes := []rune(strings.TrimSpace(value))
	if len(runes) > 255 {
		runes = runes[:255]
	}
	return string(runes)
}

// GetUserByID 根据ID获取用户
func (s *AuthService) GetUserByID(id uint) (*models.User, error) {
	user, err := s.userRepo.FindByID(id)
	if err != nil {
		return nil, ErrUserNotFound
	}
	user.Password = ""
	return user, nil
}

// UpdateProfile 更新用户资料
func (s *AuthService) UpdateProfile(userID uint, email string) (*models.User, error) {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 检查邮箱是否已被其他用户使用
	if email != "" && email != user.Email {
		exists, err := s.userRepo.ExistsByEmail(email)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrEmailExists
		}
		user.Email = email
	}

	if err := s.userRepo.Update(user); err != nil {
		switch {
		case errors.Is(err, repository.ErrDuplicateEmail), errors.Is(err, repository.ErrDuplicateIdentity):
			return nil, ErrEmailExists
		case errors.Is(err, repository.ErrDuplicateUsername):
			return nil, ErrUserExists
		default:
			return nil, err
		}
	}

	user.Password = ""
	return user, nil
}

// ChangePassword 修改密码
func (s *AuthService) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	if err := validatePasswordStrength(user.Username, newPassword); err != nil {
		return err
	}

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword)); err != nil {
		return err
	}
	// 改密后立即失效缓存中的用户快照，旧 JWT 立刻不可用。
	authcache.Delete(user.ID)
	return nil
}

// SendPasswordResetCode 向用户注册邮箱发送密码重置验证码。
func (s *AuthService) SendPasswordResetCode(username, email string) error {
	if !s.isEmailServiceEnabled() {
		return ErrEmailServiceDisabled
	}

	user, err := s.findPasswordResetUser(username, email)
	if err != nil {
		// 不暴露用户名/邮箱是否存在，避免找回入口被用于枚举账号。
		return nil
	}

	key := passwordResetKey(user.Username, user.Email)
	now := time.Now()
	if s.resetCodeCache == nil {
		return ErrEmailServiceDisabled
	}

	var existing passwordResetCodeRecord
	if err := s.resetCodeCache.Get(key, &existing); err == nil && now.Sub(existing.SentAt) < s.resetConfig.SendCooldown {
		return ErrResetCodeTooFrequent
	}

	code, err := generateResetCode()
	if err != nil {
		return err
	}
	codeHash, err := bcrypt.GenerateFromPassword([]byte(code), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	if err := s.sendPasswordResetEmail(user.Email, user.Username, code); err != nil {
		return err
	}

	record := &passwordResetCodeRecord{
		CodeHash:  string(codeHash),
		ExpiresAt: now.Add(s.resetConfig.CodeTTL),
		SentAt:    now,
		Attempts:  0,
	}
	return s.resetCodeCache.Set(key, record, s.resetConfig.CodeTTL)
}

// ResetPasswordWithCode 通过邮箱验证码重置密码。
func (s *AuthService) ResetPasswordWithCode(username, email, code, newPassword string) error {
	user, err := s.findPasswordResetUser(username, email)
	if err != nil {
		return ErrInvalidRecoveryInfo
	}
	if err := s.verifyPasswordResetCode(user.Username, user.Email, code); err != nil {
		return err
	}
	if err := validatePasswordStrength(user.Username, newPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword)); err != nil {
		return err
	}
	// 重置密码后立即失效缓存中的用户快照。
	authcache.Delete(user.ID)

	if s.resetCodeCache != nil {
		_ = s.resetCodeCache.Del(passwordResetKey(user.Username, user.Email))
	}
	return nil
}

func (s *AuthService) findPasswordResetUser(username, email string) (*models.User, error) {
	user, err := s.userRepo.FindByUsername(strings.TrimSpace(username))
	if err != nil {
		return nil, ErrInvalidRecoveryInfo
	}
	if user.Email == "" || !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(email)) {
		return nil, ErrInvalidRecoveryInfo
	}
	return user, nil
}

func (s *AuthService) verifyPasswordResetCode(username, email, code string) error {
	key := passwordResetKey(username, email)
	now := time.Now()
	normalizedCode := strings.TrimSpace(code)
	if normalizedCode == "" {
		return ErrInvalidResetCode
	}
	if s.resetCodeCache == nil {
		return ErrInvalidResetCode
	}

	var record passwordResetCodeRecord
	if err := s.resetCodeCache.Get(key, &record); err != nil || now.After(record.ExpiresAt) || record.Attempts >= s.resetConfig.MaxAttempts {
		_ = s.resetCodeCache.Del(key)
		return ErrInvalidResetCode
	}
	record.Attempts++
	codeHash := record.CodeHash

	if err := bcrypt.CompareHashAndPassword([]byte(codeHash), []byte(normalizedCode)); err != nil {
		remainingTTL := time.Until(record.ExpiresAt)
		if remainingTTL <= 0 || record.Attempts >= s.resetConfig.MaxAttempts {
			_ = s.resetCodeCache.Del(key)
		} else {
			_ = s.resetCodeCache.Set(key, &record, remainingTTL)
		}
		return ErrInvalidResetCode
	}
	return nil
}

func generateResetCode() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", n.Int64()), nil
}

func (s *AuthService) sendPasswordResetEmail(to, username, code string) error {
	smtpConfig := s.resetConfig.SMTP
	from := strings.TrimSpace(smtpConfig.From)
	fromName := strings.TrimSpace(smtpConfig.FromName)
	if fromName == "" {
		fromName = "移动云盘"
	}

	// 过滤用户名中的控制字符（CR/LF 等），避免邮件头/正文注入。
	safeUsername := sanitizeEmailText(username)

	subject := "移动云盘密码重置验证码"
	body := fmt.Sprintf("你好，%s：\n\n你的移动云盘密码重置验证码为：%s\n验证码 %d 分钟内有效，请勿转发给他人。\n\n如果不是你本人操作，请忽略本邮件。",
		safeUsername, code, int(s.resetConfig.CodeTTL.Minutes()))
	message := buildEmailMessage(from, fromName, to, subject, body)
	return sendSMTPMail(smtpConfig, from, []string{to}, []byte(message))
}

// sanitizeEmailText 过滤字符串中的控制字符（特别是 CR/LF），防止 SMTP 头/正文注入。
func sanitizeEmailText(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\r' || r == '\n' || r < 0x20 {
			return -1
		}
		return r
	}, s)
}

func buildEmailMessage(from, fromName, to, subject, body string) string {
	encodedSubject := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(subject)) + "?="
	encodedFromName := "=?UTF-8?B?" + base64.StdEncoding.EncodeToString([]byte(fromName)) + "?="
	return fmt.Sprintf("From: %s <%s>\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\nContent-Transfer-Encoding: 8bit\r\n\r\n%s",
		encodedFromName, from, to, encodedSubject, body)
}

func sendSMTPMail(config SMTPConfig, from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(config.Host, config.Port)
	var auth smtp.Auth
	if config.Username != "" {
		auth = smtp.PlainAuth("", config.Username, config.Password, config.Host)
	}

	const smtpTimeout = 10 * time.Second
	deadline := time.Now().Add(smtpTimeout)

	if config.UseTLS {
		dialer := &net.Dialer{Timeout: smtpTimeout}
		conn, err := tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{
			ServerName: config.Host,
			MinVersion: tls.VersionTLS12,
		})
		if err != nil {
			return err
		}
		_ = conn.SetDeadline(deadline)
		defer conn.Close()

		client, err := smtp.NewClient(conn, config.Host)
		if err != nil {
			return err
		}
		defer client.Quit()
		if auth != nil {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
		if err := client.Mail(from); err != nil {
			return err
		}
		for _, recipient := range to {
			if err := client.Rcpt(recipient); err != nil {
				return err
			}
		}
		writer, err := client.Data()
		if err != nil {
			return err
		}
		if _, err := writer.Write(msg); err != nil {
			_ = writer.Close()
			return err
		}
		return writer.Close()
	}

	dialer := &net.Dialer{Timeout: smtpTimeout}
	conn, err := dialer.Dial("tcp", addr)
	if err != nil {
		return err
	}
	_ = conn.SetDeadline(deadline)
	defer conn.Close()

	client, err := smtp.NewClient(conn, config.Host)
	if err != nil {
		return err
	}
	defer client.Quit()

	if ok, _ := client.Extension("STARTTLS"); ok {
		if err := client.StartTLS(&tls.Config{
			ServerName: config.Host,
			MinVersion: tls.VersionTLS12,
		}); err != nil {
			return err
		}
	}
	if auth != nil {
		if ok, _ := client.Extension("AUTH"); ok {
			if err := client.Auth(auth); err != nil {
				return err
			}
		}
	}
	if err := client.Mail(from); err != nil {
		return err
	}
	for _, recipient := range to {
		if err := client.Rcpt(recipient); err != nil {
			return err
		}
	}
	writer, err := client.Data()
	if err != nil {
		return err
	}
	if _, err := writer.Write(msg); err != nil {
		_ = writer.Close()
		return err
	}
	return writer.Close()
}
