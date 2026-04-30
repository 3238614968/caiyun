package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/pkg/jwt"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"

	"golang.org/x/crypto/bcrypt"
)

var (
	// 注意：ErrUserNotFound 已在 admin_service.go 中定义
	ErrInvalidCredentials  = errors.New("用户名或密码错误")
	ErrUserExists          = errors.New("用户已存在")
	ErrEmailExists         = errors.New("邮箱已被注册")
	ErrWeakPassword        = errors.New("密码强度不足")
	ErrInvalidRecoveryInfo = errors.New("用户名或邮箱不匹配")
)

type AuthService struct {
	userRepo  *repository.UserRepository
	jwtMgr    *jwt.Manager
	jwtExpiry time.Duration
}

func NewAuthService(
	userRepo *repository.UserRepository,
	jwtMgr *jwt.Manager,
	jwtExpiry time.Duration,
) *AuthService {
	return &AuthService{
		userRepo:  userRepo,
		jwtMgr:    jwtMgr,
		jwtExpiry: jwtExpiry,
	}
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
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, s.jwtExpiry)
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
	// 查找用户
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	// 生成JWT Token
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, s.jwtExpiry)
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

// RefreshToken 刷新Token
func (s *AuthService) RefreshToken(userID uint) (*AuthResponse, error) {
	// 查找用户
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// 生成新的JWT Token
	token, err := s.jwtMgr.GenerateToken(user.ID, user.Username, user.Role, s.jwtExpiry)
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

	user.Password = string(hashedPassword)
	return s.userRepo.Update(user)
}

// ResetPasswordByEmail 通过用户名与注册邮箱重置密码。
func (s *AuthService) ResetPasswordByEmail(username, email, newPassword string) error {
	user, err := s.userRepo.FindByUsername(strings.TrimSpace(username))
	if err != nil {
		return ErrInvalidRecoveryInfo
	}
	if user.Email == "" || !strings.EqualFold(strings.TrimSpace(user.Email), strings.TrimSpace(email)) {
		return ErrInvalidRecoveryInfo
	}
	if err := validatePasswordStrength(user.Username, newPassword); err != nil {
		return err
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	user.Password = string(hashedPassword)
	return s.userRepo.Update(user)
}
