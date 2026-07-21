package services

import (
	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/queue"
	"caiyun/internal/repository"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/pkg/validator"
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

// CreateAccountRequest 创建账号请求
type CreateAccountRequest struct {
	Phone  string `json:"phone" binding:"required"`
	Auth   string `json:"auth" binding:"required"`
	Remark string `json:"remark" binding:"omitempty"`
}

// UpdateAccountRequest 更新账号请求
type UpdateAccountRequest struct {
	Phone  string `json:"phone" binding:"required"`
	Auth   string `json:"auth" binding:"omitempty"`
	Remark string `json:"remark" binding:"omitempty"`
}

// CreateAccount 创建账号（如果当前用户已存在则更新）
func (s *AccountService) CreateAccount(userID uint, req *CreateAccountRequest) (*models.Account, error) {
	return s.CreateAccountContext(context.Background(), userID, req)
}

// CreateAccountContext 创建账号，并让数据库操作响应调用方取消与超时。
func (s *AccountService) CreateAccountContext(ctx context.Context, userID uint, req *CreateAccountRequest) (*models.Account, error) {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	accountRepo := s.accountRepositoryWithContext(ctx)
	userRepo := s.userRepositoryWithContext(ctx)

	req.Phone = strings.TrimSpace(req.Phone)
	if !validator.IsValidPhone(req.Phone) {
		return nil, ErrInvalidPhone
	}

	// 验证用户存在
	_, err := userRepo.FindByID(userID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrAccountNotFound
	}

	// 检查该用户是否已存在该手机号
	existingAccount, err := accountRepo.FindByPhoneAndUserID(req.Phone, userID)
	if err != nil {
		return nil, err
	}
	if existingAccount != nil {
		// 已存在，更新账号信息
		existingAccount.Auth = req.Auth
		existingAccount.Remark = req.Remark
		existingAccount.IsActive = true
		existingAccount.JWTErrorCount = 0 // 重置 JWT 错误计数

		// 尝试从 Auth 中解析 token、平台和过期时间
		if info, err := auth.ParseToken(req.Auth); err == nil && info != nil {
			existingAccount.Token = info.Token
			existingAccount.ExpireAt = info.Expire
			if info.Platform != "" {
				existingAccount.Platform = info.Platform
			}
		}

		if err := accountRepo.Update(existingAccount); err != nil {
			return nil, err
		}

		return existingAccount, nil
	}

	// 不存在，创建新账号
	account := &models.Account{
		UserID:   userID,
		Phone:    req.Phone,
		Auth:     req.Auth,
		Platform: "pc",
		Remark:   req.Remark,
		IsActive: true,
	}

	// 尝试从 Auth 中解析 token、平台和过期时间，避免首次使用时 token 为空导致刷新失败
	if info, err := auth.ParseToken(req.Auth); err == nil && info != nil {
		account.Token = info.Token
		account.ExpireAt = info.Expire
		if info.Platform != "" {
			account.Platform = info.Platform
		}
	}

	if err := accountRepo.Create(account); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *AccountService) GetAccount(userID, accountID uint) (*models.Account, error) {
	return s.GetAccountContext(context.Background(), userID, accountID)
}

// GetAccountContext 获取账号，并让数据库操作响应调用方取消与超时。
func (s *AccountService) GetAccountContext(ctx context.Context, userID, accountID uint) (*models.Account, error) {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	account, err := s.accountRepositoryWithContext(ctx).FindByID(accountID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return nil, ErrAccountNotFound
	}

	return account, nil
}

// GetAccountByID 获取账号详情（不带权限检查，供Worker使用）
func (s *AccountService) GetAccountByID(accountID uint) (*models.Account, error) {
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return nil, ErrAccountNotFound
	}

	return account, nil
}

// ListAccounts 列出用户的账号
func (s *AccountService) ListAccounts(userID uint, page, pageSize int, phone string) ([]*models.Account, int64, error) {
	return s.ListAccountsContext(context.Background(), userID, page, pageSize, phone)
}

// ListAccountsContext 列出用户账号，并让数据库操作响应调用方取消与超时。
func (s *AccountService) ListAccountsContext(ctx context.Context, userID uint, page, pageSize int, phone string) ([]*models.Account, int64, error) {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, 0, err
	}
	offset := (page - 1) * pageSize
	return s.accountRepositoryWithContext(ctx).ListByUserID(userID, offset, pageSize, phone)
}

// UpdateAccount 更新账号
func (s *AccountService) UpdateAccount(userID, accountID uint, req *UpdateAccountRequest) (*models.Account, error) {
	return s.UpdateAccountContext(context.Background(), userID, accountID, req)
}

// UpdateAccountContext 更新账号，并让数据库操作响应调用方取消与超时。
func (s *AccountService) UpdateAccountContext(ctx context.Context, userID, accountID uint, req *UpdateAccountRequest) (*models.Account, error) {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	accountRepo := s.accountRepositoryWithContext(ctx)

	req.Phone = strings.TrimSpace(req.Phone)
	if !validator.IsValidPhone(req.Phone) {
		return nil, ErrInvalidPhone
	}

	// 获取账号
	account, err := accountRepo.FindByID(accountID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		return nil, ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return nil, ErrAccountNotFound
	}

	// 检查该用户是否已有其他账号使用此手机号
	if req.Phone != account.Phone {
		exists, err := accountRepo.ExistsByPhoneAndUserID(req.Phone, userID)
		if err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrAccountExists
		}
	}

	// 更新账号信息
	account.Phone = req.Phone
	account.Remark = req.Remark

	// 同步解析 Auth 到 token/平台/过期时间
	if req.Auth != "" {
		account.Auth = req.Auth
		if info, err := auth.ParseToken(req.Auth); err == nil && info != nil {
			account.Token = info.Token
			account.ExpireAt = info.Expire
			if info.Platform != "" {
				account.Platform = info.Platform
			}
		}
	}

	if err := accountRepo.Update(account); err != nil {
		return nil, err
	}

	return account, nil
}

// DeleteAccount 删除账号
func (s *AccountService) DeleteAccount(userID, accountID uint) error {
	return s.DeleteAccountContext(context.Background(), userID, accountID)
}

// DeleteAccountContext 删除账号，并让数据库操作响应调用方取消与超时。
func (s *AccountService) DeleteAccountContext(ctx context.Context, userID, accountID uint) error {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.unitOfWork != nil {
		return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
			account, err := repos.Account.FindByID(accountID)
			if err != nil || account.UserID != userID {
				return ErrAccountNotFound
			}

			cleanup := []func(uint) error{
				repos.Operation.DeleteByAccountID,
				repos.ExchangeRecord.DeleteByAccountID,
				repos.ExchangeTask.DeleteByAccountID,
				repos.ExchangeAccount.DeleteByAccountID,
				repos.TaskLog.DeleteByAccountID,
				repos.CloudStats.DeleteByAccountID,
				repos.Account.AnonymizeAndDeleteByID,
			}
			for _, erase := range cleanup {
				if err := erase(accountID); err != nil {
					return err
				}
			}
			return nil
		})
	}

	// 保留面向测试替身和离线工具的兼容路径；生产仓库由构造函数自动
	// 绑定 UnitOfWork，走上面的完整清理事务。
	accountRepo := s.accountRepositoryWithContext(ctx)
	account, err := accountRepo.FindByID(accountID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return ErrAccountNotFound
	}
	if account.UserID != userID {
		return ErrAccountNotFound
	}
	return accountRepo.Delete(accountID)
}

// SetAccountStatus 设置账号状态
func (s *AccountService) SetAccountStatus(userID, accountID uint, isActive bool) error {
	return s.SetAccountStatusContext(context.Background(), userID, accountID, isActive)
}

// SetAccountStatusContext 设置账号状态，并让数据库操作响应调用方取消与超时。
func (s *AccountService) SetAccountStatusContext(ctx context.Context, userID, accountID uint, isActive bool) error {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	accountRepo := s.accountRepositoryWithContext(ctx)

	// 获取账号
	account, err := accountRepo.FindByID(accountID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return ErrAccountNotFound
	}

	return accountRepo.SetActiveStatus(accountID, isActive)
}

// GetToken 获取账号 Token（优先委托 TokenManager，未注入时回退到数据库+按需刷新路径）。
func (s *AccountService) GetToken(accountID uint) (string, error) {
	if s.tokenProvider != nil {
		tokenInfo, err := s.tokenProvider.GetToken(accountID)
		if err != nil {
			return "", err
		}
		if tokenInfo == nil {
			return "", fmt.Errorf("账号 %d Token 为空", accountID)
		}
		if tokenInfo.SSOToken != "" {
			return tokenInfo.SSOToken, nil
		}
		if tokenInfo.JWTToken != "" {
			return tokenInfo.JWTToken, nil
		}
		return "", fmt.Errorf("账号 %d Token 为空", accountID)
	}

	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return "", err
	}

	if account.Token == "" {
		if err := s.RefreshToken(account); err != nil {
			return "", err
		}
	}
	if account.Token == "" {
		return "", fmt.Errorf("账号 %d Token 为空", accountID)
	}
	return account.Token, nil
}

// RefreshToken 刷新账号Token
func (s *AccountService) RefreshToken(account *models.Account) error {
	return s.RefreshTokenContext(context.Background(), account)
}

// RefreshTokenContext 刷新账号 Token，并将取消传播到认证重试、上游 HTTP 与数据库写入。
func (s *AccountService) RefreshTokenContext(ctx context.Context, account *models.Account) error {
	ctx = accountServiceContext(ctx)
	if err := ctx.Err(); err != nil {
		return err
	}
	if account == nil {
		return fmt.Errorf("账号为空")
	}

	authClient := corehttp.NewClient()
	if authStr := sanitizeAuthValue(account.Auth); authStr != "" {
		authClient.SetAuth(authStr)
	}
	authForAccount := auth.NewAuth(authClient)

	userDomainID := ""
	jwtToken := account.JWTToken
	if err := ctx.Err(); err != nil {
		return err
	}
	if token, _, err := authForAccount.GetJWTTokenWithSSOTokenContext(ctx, account.Phone); err == nil && token != "" {
		jwtToken = token
		userDomainID = jwtUserDomainID(token)
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	refreshed, err := authForAccount.RefreshAuthorizationContext(ctx, account.Auth, account.Phone, userDomainID)
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	if refreshed.SSOToken != "" {
		if token, err := authForAccount.TyrzLoginContext(ctx, refreshed.SSOToken); err == nil && token != "" {
			jwtToken = token
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	applyAuthorizationRefreshToAccount(account, refreshed, jwtToken)

	return s.persistAuthorizationRefreshContext(ctx, account)
}

func (s *AccountService) persistAuthorizationRefreshContext(ctx context.Context, account *models.Account) error {
	if account == nil {
		return fmt.Errorf("账号为空")
	}
	if s.unitOfWork != nil {
		return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
			if err := repos.Account.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
				return err
			}
			if s.exchangeRepo != nil {
				if err := repos.ExchangeAccount.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
					return fmt.Errorf("同步抢兑账号鉴权失败: %w", err)
				}
			}
			return nil
		})
	}

	accountRepo := s.accountRepositoryWithContext(ctx)
	if err := accountRepo.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if exchangeRepo := s.exchangeRepositoryWithContext(ctx); exchangeRepo != nil {
		if err := exchangeRepo.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
			return fmt.Errorf("同步抢兑账号鉴权失败: %w", err)
		}
	}
	return ctx.Err()
}

// RefreshTokenIfNeeded 根据需要刷新Token
func (s *AccountService) RefreshTokenIfNeeded(account *models.Account) error {
	now := time.Now()
	expireAt := accountAuthorizationExpireAt(account)
	if !authorizationShouldRefresh(expireAt, now) {
		return nil
	}

	if err := s.RefreshToken(account); err != nil {
		// 提前 5 天预刷新失败时，不覆盖数据库，也不阻断仍未过期账号的正常任务。
		if expireAt > now.UnixMilli() {
			return nil
		}
		return err
	}
	return nil
}

// GetCloudCount 获取账号云朵数量
func (s *AccountService) GetCloudCount(accountID uint) (int, error) {
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return 0, err
	}
	return account.CloudCount, nil
}

// UpdateCloudCount 更新云朵数量
func (s *AccountService) UpdateCloudCount(accountID uint, cloudCount int) error {
	return s.accountRepo.UpdateCloudCount(accountID, cloudCount)
}

// GetTotalCloudCount 获取用户所有账号的总云朵数
func (s *AccountService) GetTotalCloudCount(userID uint) (int, error) {
	return s.accountRepo.GetTotalCloudCountByUserID(userID)
}

// GetActiveAccounts 获取用户的所有激活账号。
func (s *AccountService) GetActiveAccounts(userID uint) ([]*models.Account, error) {
	return s.GetActiveAccountsContext(context.Background(), userID)
}

// GetActiveAccountsContext binds the request context when the concrete
// repository supports it. Test doubles and legacy adapters remain compatible.
func (s *AccountService) GetActiveAccountsContext(ctx context.Context, userID uint) ([]*models.Account, error) {
	repo := s.accountRepo
	if binder, ok := repo.(interface {
		WithContext(context.Context) *repository.AccountRepository
	}); ok {
		repo = binder.WithContext(ctx)
	}
	return repo.FindActiveAccountsByUserID(userID)
}

// GetAllActiveAccounts 获取所有激活账号（供Worker使用）
func (s *AccountService) GetAllActiveAccounts() ([]*models.Account, error) {
	return s.accountRepo.FindActiveAccounts()
}

// ListActiveAccounts 分页获取激活账号，供 Worker 分批执行，避免一次性加载全表。
func (s *AccountService) ListActiveAccounts(offset, limit int) ([]*models.Account, error) {
	return s.accountRepo.FindActiveAccountsPaged(offset, limit)
}

// EnqueueTask 将任务加入队列
func (s *AccountService) EnqueueTask(accountID uint, taskType string) error {
	// 获取账号信息
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return err
	}

	if s.taskQueue != nil {
		return s.taskQueue.Enqueue(account.ID, account.UserID, taskType)
	}

	// 使用Redis List实现队列
	message := map[string]interface{}{
		"account_id":  accountID,
		"user_id":     account.UserID,
		"task_type":   taskType,
		"created_at":  time.Now().Unix(),
		"retry_count": 0,
	}

	// 序列化消息
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// 推入队列
	return s.cache.LPush(queue.TaskQueueKey, string(data))
}
