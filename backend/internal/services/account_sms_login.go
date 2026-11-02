package services

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
	"github.com/google/uuid"
)

var (
	ErrActiveAccountLogin        = errors.New("该手机号已有有效凭据；重新登录会使当前凭据失效，如需替换请明确选择强制登录")
	ErrAccountLoginBusy          = errors.New("该手机号正在登录，请稍后重试")
	errAccountCredentialsChanged = errors.New("账号凭据已更新，请重试")
	smsLoginLocks                sync.Map
)

// CheckSMSLoginAllowedContext checks metadata only, including credentials held
// by another tenant, since upstream RCS credentials are single-active per phone.
func (s *AccountService) CheckSMSLoginAllowedContext(ctx context.Context, userID uint, phone string, force bool) error {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if force {
		return nil
	}
	repo := s.accountRepositoryWithContext(ctx)
	if checker, ok := repo.(interface {
		HasActiveLoginForPhone(string, int64) (bool, error)
	}); ok {
		active, err := checker.HasActiveLoginForPhone(phone, time.Now().UnixMilli())
		if err != nil {
			return err
		}
		if active {
			return ErrActiveAccountLogin
		}
		return nil
	}
	account, err := repo.FindByPhoneAndUserID(phone, userID)
	if err != nil {
		return err
	}
	if account != nil && account.IsActive && strings.TrimSpace(account.Auth) != "" && accountAuthorizationExpireAt(account) > time.Now().UnixMilli() {
		return ErrActiveAccountLogin
	}
	return nil
}

// WithSMSLoginContext holds the phone lease through verification AND saving the
// replacement credential. A second request must recheck the newly saved login.
func (s *AccountService) WithSMSLoginContext(ctx context.Context, userID uint, phone string, force bool, login func(context.Context) (*models.Account, error)) (*models.Account, error) {
	ctx, cancel := context.WithTimeout(accountServiceContext(ctx), 2*time.Minute)
	defer cancel()
	release, err := acquireCredentialRotationLock(ctx, s.cache, phone)
	if err != nil {
		return nil, err
	}
	defer release()
	if err := s.CheckSMSLoginAllowedContext(ctx, userID, phone, force); err != nil {
		return nil, err
	}
	return login(ctx)
}

// All credential-changing paths share this phone-scoped lease, including SMS
// login, manual refresh and worker recovery after a rejected credential.
func acquireCredentialRotationLock(ctx context.Context, lockCache *cache.RedisCache, phone string) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	key := fmt.Sprintf("caiyun:sms-login:%x", sha256.Sum256([]byte(strings.TrimSpace(phone))))
	value, _ := smsLoginLocks.LoadOrStore(key, &sync.Mutex{})
	mu := value.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, ErrAccountLoginBusy
	}
	if lockCache != nil {
		owner := uuid.NewString()
		locked, err := lockCache.SetNX(key, owner, 3*time.Minute)
		if err != nil {
			mu.Unlock()
			return nil, fmt.Errorf("获取登录互斥锁失败: %w", err)
		}
		if !locked {
			mu.Unlock()
			return nil, ErrAccountLoginBusy
		}
		return func() { defer mu.Unlock(); _, _ = lockCache.DelIfValue(key, owner) }, nil
	}
	return mu.Unlock, nil
}
