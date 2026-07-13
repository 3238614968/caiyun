package services

import (
	"context"
	"errors"
	"testing"

	"caiyun/internal/models"
)

type stubAccountRepository struct {
	account *models.Account
	err     error
}

func (s *stubAccountRepository) Create(account *models.Account) error { return nil }
func (s *stubAccountRepository) FindByID(id uint) (*models.Account, error) {
	if s.err != nil {
		return nil, s.err
	}
	if s.account != nil {
		clone := *s.account
		if clone.ID == 0 {
			clone.ID = id
		}
		return &clone, nil
	}
	return nil, errors.New("should not be called")
}
func (s *stubAccountRepository) Update(account *models.Account) error { return nil }
func (s *stubAccountRepository) Delete(id uint) error                 { return nil }
func (s *stubAccountRepository) ListByUserID(userID uint, offset, limit int, phone string) ([]*models.Account, int64, error) {
	return nil, 0, nil
}
func (s *stubAccountRepository) FindActiveAccounts() ([]*models.Account, error) { return nil, nil }
func (s *stubAccountRepository) FindActiveAccountsPaged(offset, limit int) ([]*models.Account, error) {
	return nil, nil
}
func (s *stubAccountRepository) FindActiveAccountsByUserID(userID uint) ([]*models.Account, error) {
	return nil, nil
}
func (s *stubAccountRepository) UpdateCloudCount(id uint, cloudCount int) error      { return nil }
func (s *stubAccountRepository) GetTotalCloudCountByUserID(userID uint) (int, error) { return 0, nil }
func (s *stubAccountRepository) ExistsByPhoneAndUserID(phone string, userID uint) (bool, error) {
	return false, nil
}
func (s *stubAccountRepository) FindByPhoneAndUserID(phone string, userID uint) (*models.Account, error) {
	return nil, nil
}
func (s *stubAccountRepository) SetActiveStatus(id uint, isActive bool) error { return nil }
func (s *stubAccountRepository) UpdateAuthorizationFields(id uint, authValue, token, jwtToken, platform string, expireAt int64) error {
	return nil
}

type stubAccountUserRepository struct{}

func (s *stubAccountUserRepository) FindByID(id uint) (*models.User, error) {
	return &models.User{ID: id}, nil
}

type stubTokenProvider struct {
	info *TokenInfo
	err  error
}

func (s *stubTokenProvider) GetToken(accountID uint) (*TokenInfo, error) {
	return s.info, s.err
}

func TestAccountServiceGetTokenDelegatesToTokenProvider(t *testing.T) {
	service := NewAccountService(&stubAccountRepository{}, &stubAccountUserRepository{}, nil, nil)
	service.SetTokenProvider(&stubTokenProvider{info: &TokenInfo{SSOToken: "sso-token", JWTToken: "jwt-token"}})

	token, err := service.GetToken(101)
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if token != "sso-token" {
		t.Fatalf("GetToken() = %q, want %q", token, "sso-token")
	}
}

func TestAccountServiceGetTokenReturnsProviderError(t *testing.T) {
	service := NewAccountService(&stubAccountRepository{}, &stubAccountUserRepository{}, nil, nil)
	service.SetTokenProvider(&stubTokenProvider{err: errors.New("boom")})

	if _, err := service.GetToken(101); err == nil || err.Error() != "boom" {
		t.Fatalf("GetToken() error = %v, want boom", err)
	}
}

func TestAccountServiceGetTokenFallsBackToRepositoryWithoutCache(t *testing.T) {
	repo := &stubAccountRepository{account: &models.Account{ID: 101, Token: "repo-token"}}
	service := NewAccountService(repo, &stubAccountUserRepository{}, nil, nil)

	token, err := service.GetToken(101)
	if err != nil {
		t.Fatalf("GetToken() error = %v", err)
	}
	if token != "repo-token" {
		t.Fatalf("GetToken() = %q, want %q", token, "repo-token")
	}
}

func TestAccountServiceContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	service := NewAccountService(&stubAccountRepository{}, &stubAccountUserRepository{}, nil, nil)
	if _, err := service.GetAccountContext(ctx, 1, 1); !errors.Is(err, context.Canceled) {
		t.Fatalf("GetAccountContext() error = %v, want context.Canceled", err)
	}
	if _, _, err := service.ListAccountsContext(ctx, 1, 1, 10, ""); !errors.Is(err, context.Canceled) {
		t.Fatalf("ListAccountsContext() error = %v, want context.Canceled", err)
	}
	if err := service.RefreshTokenContext(ctx, &models.Account{ID: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("RefreshTokenContext() error = %v, want context.Canceled", err)
	}
}
