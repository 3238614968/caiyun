package services

import (
	"context"
	"log"

	"golang.org/x/crypto/bcrypt"
)

// Login 用户登录（保留兼容；HTTP 调用应使用 LoginContext）。
func (s *AuthService) Login(req *LoginRequest) (*AuthResponse, error) {
	return s.LoginContext(context.Background(), req, SessionMetadata{})
}

// LoginContext authenticates credentials and creates a rotating refresh session.
func (s *AuthService) LoginContext(ctx context.Context, req *LoginRequest, metadata SessionMetadata) (*AuthResponse, error) {
	// 登录失败锁定：优先检查 Redis 计数；Redis 不可用时降级到数据库表 login_fail_locks。
	lockKeys := loginLockKeysFor(req.Username, metadata.ClientIP)
	locked, err := s.isLoginLockedForKeys(lockKeys)
	if err != nil {
		return nil, ErrLoginProtectionUnavailable
	}
	if locked {
		return nil, ErrAccountLocked
	}

	// 查找用户
	user, err := s.userRepo.FindByUsername(req.Username)
	if err != nil {
		// 不暴露用户是否存在：仍递增失败计数，防止通过响应差异枚举账号。
		if recordErr := s.recordLoginFailureForKeys(lockKeys); recordErr != nil {
			return nil, ErrLoginProtectionUnavailable
		}
		return nil, ErrInvalidCredentials
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(req.Password)); err != nil {
		if recordErr := s.recordLoginFailureForKeys(lockKeys); recordErr != nil {
			return nil, ErrLoginProtectionUnavailable
		}
		return nil, ErrInvalidCredentials
	}

	// 登录成功：清除失败计数。
	if clearErr := s.clearLoginFailureForKeys(lockKeys); clearErr != nil {
		// A valid password has already been verified. Keep the successful-login
		// path available during a transient cache/store cleanup outage; a later
		// failed attempt can safely re-establish the counters.
		log.Printf("登录保护计数清理失败，继续签发会话: %v", clearErr)
	}

	return s.issueAuthSession(ctx, user, metadata)
}
