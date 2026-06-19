package services

import (
	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/queue"
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
	accountRepo  accountRepository
	exchangeRepo accountExchangeRepository
	userRepo     accountUserRepository
	cache        *cache.RedisCache
	authMgr      *auth.Auth
	taskQueue    queue.ReliableTaskQueue
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
	req.Phone = strings.TrimSpace(req.Phone)
	if !validator.IsValidPhone(req.Phone) {
		return nil, ErrInvalidPhone
	}

	// 验证用户存在
	_, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrAccountNotFound
	}

	// 检查该用户是否已存在该手机号
	existingAccount, err := s.accountRepo.FindByPhoneAndUserID(req.Phone, userID)
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

		if err := s.accountRepo.Update(existingAccount); err != nil {
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

	if err := s.accountRepo.Create(account); err != nil {
		return nil, err
	}

	return account, nil
}

func (s *AccountService) GetAccount(userID, accountID uint) (*models.Account, error) {
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
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
	offset := (page - 1) * pageSize
	return s.accountRepo.ListByUserID(userID, offset, pageSize, phone)
}

// UpdateAccount 更新账号
func (s *AccountService) UpdateAccount(userID, accountID uint, req *UpdateAccountRequest) (*models.Account, error) {
	req.Phone = strings.TrimSpace(req.Phone)
	if !validator.IsValidPhone(req.Phone) {
		return nil, ErrInvalidPhone
	}

	// 获取账号
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return nil, ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return nil, ErrAccountNotFound
	}

	// 检查该用户是否已有其他账号使用此手机号
	if req.Phone != account.Phone {
		exists, err := s.accountRepo.ExistsByPhoneAndUserID(req.Phone, userID)
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

	if err := s.accountRepo.Update(account); err != nil {
		return nil, err
	}

	return account, nil
}

// DeleteAccount 删除账号
func (s *AccountService) DeleteAccount(userID, accountID uint) error {
	// 获取账号
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return ErrAccountNotFound
	}

	return s.accountRepo.Delete(accountID)
}

// SetAccountStatus 设置账号状态
func (s *AccountService) SetAccountStatus(userID, accountID uint, isActive bool) error {
	// 获取账号
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return ErrAccountNotFound
	}

	// 检查权限
	if account.UserID != userID {
		return ErrAccountNotFound
	}

	return s.accountRepo.SetActiveStatus(accountID, isActive)
}

// GetToken 获取账号Token（优先从缓存获取）
func (s *AccountService) GetToken(accountID uint) (string, error) {
	// 从缓存获取
	cacheKey := fmt.Sprintf("account:token:%d", accountID)
	var token string
	err := s.cache.Get(cacheKey, &token)
	if err == nil && token != "" {
		return token, nil
	}

	// 从数据库获取
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return "", err
	}

	// 如果数据库中也没有Token，则刷新
	if account.Token == "" {
		if err := s.RefreshToken(account); err != nil {
			return "", err
		}
		token = account.Token
	} else {
		token = account.Token
	}

	// 缓存Token（24小时）
	tokenCacheKey := fmt.Sprintf("account:token:%d", accountID)
	s.cache.Set(tokenCacheKey, token, 24*time.Hour)

	return token, nil
}

// RefreshToken 刷新账号Token
func (s *AccountService) RefreshToken(account *models.Account) error {
	if account == nil {
		return fmt.Errorf("账号为空")
	}

	// 使用账号自己的 authorization 创建临时认证客户端，避免复用全局 client 造成串号。
	authClient := corehttp.NewClient()
	if authStr := sanitizeAuthValue(account.Auth); authStr != "" {
		authClient.SetAuth(authStr)
	}
	authForAccount := auth.NewAuth(authClient)

	// refreshToken 新接口推荐携带 userDomainId；先尽力使用当前 authorization 换取 JWT 并解析。
	userDomainID := ""
	jwtToken := account.JWTToken
	if token, _, err := authForAccount.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
		jwtToken = token
		userDomainID = jwtUserDomainID(token)
	}

	refreshed, err := authForAccount.RefreshAuthorization(account.Auth, account.Phone, userDomainID)
	if err != nil {
		return err
	}

	// 刷新成功后再落库；如果后续 JWT 换取失败，也保留成功刷新的 authorization。
	if refreshed.SSOToken != "" {
		if token, err := authForAccount.TyrzLogin(refreshed.SSOToken); err == nil && token != "" {
			jwtToken = token
		}
	}
	applyAuthorizationRefreshToAccount(account, refreshed, jwtToken)

	if err := s.accountRepo.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
		return err
	}
	if s.exchangeRepo != nil {
		if err := s.exchangeRepo.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
			return fmt.Errorf("同步抢兑账号鉴权失败: %w", err)
		}
	}

	// 更新缓存
	cacheKey := fmt.Sprintf("account:token:%d", account.ID)
	s.cache.Set(cacheKey, account.Token, 24*time.Hour)

	return nil
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

// GetActiveAccounts 获取用户的所有激活账号
func (s *AccountService) GetActiveAccounts(userID uint) ([]*models.Account, error) {
	return s.accountRepo.FindActiveAccountsByUserID(userID)
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
