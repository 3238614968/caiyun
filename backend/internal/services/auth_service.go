package services

import (
	"caiyun/internal/middleware"
	"caiyun/internal/models"
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

type AuthService struct {
	userRepo       authUserRepository
	jwtMgr         *jwt.Manager
	jwtExpiry      time.Duration
	resetConfig    PasswordResetConfig
	resetCodeCache passwordResetCache
	loginLockCache passwordResetCache
	loginLockStore loginLockStore
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

	return &AuthService{
		userRepo:       userRepo,
		jwtMgr:         jwtMgr,
		jwtExpiry:      jwtExpiry,
		resetConfig:    resetConfig,
		resetCodeCache: resetCodeCache,
		loginLockCache: resetCodeCache, // 登录锁定复用同一 Redis 缓存
		loginLockStore: dbLoginLockStore,
	}
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
	Password string `json:"password" binding:"required,min=12"`
	Email    string `json:"email" binding:"omitempty,email"`
}

// LoginRequest 登录请求
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// AuthResponse 认证响应
type AuthResponse struct {
	Token     string       `json:"-"`
	ExpiresAt int64        `json:"expires_at"`
	User      *models.User `json:"user"`
}

// Register 用户注册
func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
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
		return nil, err
	}

	// 生成JWT Token
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, user.TokenVersion, s.jwtExpiry)
	if err != nil {
		return nil, err
	}

	// 清除密码字段
	user.Password = ""

	return &AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(s.jwtExpiry).Unix(),
		User:      user,
	}, nil
}

func validatePasswordStrength(username, password string) error {
	if len([]rune(password)) < 12 {
		return fmt.Errorf("%w：长度至少 12 个字符", ErrWeakPassword)
	}
	lowerUsername := strings.ToLower(strings.TrimSpace(username))
	lowerPassword := strings.ToLower(password)
	if lowerUsername != "" && strings.Contains(lowerPassword, lowerUsername) {
		return fmt.Errorf("%w：不能包含用户名", ErrWeakPassword)
	}

	commonPasswords := map[string]struct{}{
		"password":    {},
		"password123": {},
		"123456":      {},
		"123456789":   {},
		"1234567890":  {},
		"qwerty123":   {},
		"admin123":    {},
		"admin123456": {},
		"letmein":     {},
		"welcome123":  {},
		"changeme":    {},
		"iloveyou":    {},
		"abc123456":   {},
		"111111":      {},
	}
	if _, ok := commonPasswords[lowerPassword]; ok {
		return fmt.Errorf("%w：不能使用常见弱口令", ErrWeakPassword)
	}

	var hasLower, hasUpper, hasDigit, hasSymbol bool
	for _, r := range password {
		switch {
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsDigit(r):
			hasDigit = true
		case unicode.IsPunct(r) || unicode.IsSymbol(r):
			hasSymbol = true
		}
	}

	classes := 0
	for _, ok := range []bool{hasLower, hasUpper, hasDigit, hasSymbol} {
		if ok {
			classes++
		}
	}
	if classes < 3 {
		return fmt.Errorf("%w：需包含大小写字母、数字、符号中的至少三类", ErrWeakPassword)
	}

	return nil
}

// Login 用户登录
func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
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

	// 生成JWT Token
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, user.TokenVersion, s.jwtExpiry)
	if err != nil {
		return nil, err
	}

	// 清除密码字段
	user.Password = ""

	return &AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(s.jwtExpiry).Unix(),
		User:      user,
	}, nil
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

// RefreshToken 刷新Token
func (s *AuthService) RefreshToken(userID uint) (*AuthResponse, error) {
	// 查找用户
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 生成新的JWT Token
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, user.TokenVersion, s.jwtExpiry)
	if err != nil {
		return nil, err
	}

	// 清除密码字段
	user.Password = ""

	return &AuthResponse{
		Token:     token,
		ExpiresAt: time.Now().Add(s.jwtExpiry).Unix(),
		User:      user,
	}, nil
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
		return nil, err
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
	middleware.InvalidateAuthUserCache(user.ID)
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
	middleware.InvalidateAuthUserCache(user.ID)

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
