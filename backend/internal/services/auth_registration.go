package services

import (
	"context"
	"errors"

	"caiyun/internal/models"
	"caiyun/internal/repository"

	"golang.org/x/crypto/bcrypt"
)

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
