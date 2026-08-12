package services

import (
	"context"
	"errors"

	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
)

var (
	ErrAccountNotFound = errors.New("账号不存在")
	ErrAccountExists   = errors.New("账号已存在")
	ErrInvalidPhone    = errors.New("手机号格式不正确")
)

type AccountService struct {
	accountRepo   accountRepository
	exchangeRepo  accountExchangeRepository
	userRepo      accountUserRepository
	cache         *cache.RedisCache
	authMgr       *auth.Auth
	taskQueue     queue.ReliableTaskQueue
	tokenProvider accountTokenProvider
	unitOfWork    repository.UnitOfWork
}

type accountRepository interface {
	Create(account *models.Account) error
	FindByID(id uint) (*models.Account, error)
	Update(account *models.Account) error
	Delete(id uint) error
	ListByUserID(userID uint, offset, limit int, phone string) ([]*models.Account, int64, error)
	FindActiveAccounts() ([]*models.Account, error)
	FindActiveAccountsPaged(offset, limit int) ([]*models.Account, error)
	FindActiveAccountsByUserID(userID uint) ([]*models.Account, error)
	UpdateCloudCount(id uint, cloudCount int) error
	GetTotalCloudCountByUserID(userID uint) (int, error)
	ExistsByPhoneAndUserID(phone string, userID uint) (bool, error)
	FindByPhoneAndUserID(phone string, userID uint) (*models.Account, error)
	SetActiveStatus(id uint, isActive bool) error
	UpdateAuthorizationFields(id uint, authValue, token, jwtToken, platform string, expireAt int64) error
}

type accountUserRepository interface {
	FindByID(id uint) (*models.User, error)
}

type accountExchangeRepository interface {
	UpdateAuthByAccountID(accountID uint, auth, token, jwtToken string) error
}

type accountTokenProvider interface {
	GetToken(accountID uint) (*TokenInfo, error)
}

func NewAccountService(
	accountRepo accountRepository,
	userRepo accountUserRepository,
	cache *cache.RedisCache,
	authMgr *auth.Auth,
	exchangeRepos ...accountExchangeRepository,
) *AccountService {
	service := &AccountService{
		accountRepo: accountRepo,
		userRepo:    userRepo,
		cache:       cache,
		authMgr:     authMgr,
	}
	if concrete, ok := accountRepo.(*repository.AccountRepository); ok {
		service.unitOfWork = repository.NewUnitOfWorkFromAccountRepository(concrete)
	}
	if len(exchangeRepos) > 0 {
		service.exchangeRepo = exchangeRepos[0]
	}
	return service
}

func (s *AccountService) SetTaskQueue(taskQueue queue.ReliableTaskQueue) {
	s.taskQueue = taskQueue
}

func (s *AccountService) SetExchangeAccountRepository(exchangeRepo accountExchangeRepository) {
	s.exchangeRepo = exchangeRepo
}

func (s *AccountService) SetTokenProvider(tokenProvider accountTokenProvider) {
	s.tokenProvider = tokenProvider
}

// SetUnitOfWork overrides the repository-derived transaction boundary.
func (s *AccountService) SetUnitOfWork(unitOfWork repository.UnitOfWork) {
	s.unitOfWork = unitOfWork
}

func accountServiceContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func (s *AccountService) accountRepositoryWithContext(ctx context.Context) accountRepository {
	repo := s.accountRepo
	if binder, ok := repo.(interface {
		WithContext(context.Context) *repository.AccountRepository
	}); ok {
		repo = binder.WithContext(accountServiceContext(ctx))
	}
	return repo
}

func (s *AccountService) userRepositoryWithContext(ctx context.Context) accountUserRepository {
	repo := s.userRepo
	if binder, ok := repo.(interface {
		WithContext(context.Context) *repository.UserRepository
	}); ok {
		repo = binder.WithContext(accountServiceContext(ctx))
	}
	return repo
}

func (s *AccountService) exchangeRepositoryWithContext(ctx context.Context) accountExchangeRepository {
	repo := s.exchangeRepo
	if binder, ok := repo.(interface {
		WithContext(context.Context) *repository.ExchangeAccountRepository
	}); ok {
		repo = binder.WithContext(accountServiceContext(ctx))
	}
	return repo
}
