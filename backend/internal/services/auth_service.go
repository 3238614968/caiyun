package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/pkg/jwt"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

var (
	// 注意：ErrUserNotFound 已在 admin_service.go 中定义
	ErrInvalidCredentials = errors.New("用户名或密码错误")
	ErrUserExists         = errors.New("用户已存在")
	ErrEmailExists        = errors.New("邮箱已被注册")
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
	Token     string       `json:"token"`
	ExpiresAt int64        `json:"expires_at"`
	User      *models.User `json:"user"`
}

// Register 用户注册
func (s *AuthService) Register(req *RegisterRequest) (*AuthResponse, error) {
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

	// 加密新密码
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcrypt.DefaultCost)
	if err != nil {
		return err
	}

	user.Password = string(hashedPassword)
	return s.userRepo.Update(user)
}
