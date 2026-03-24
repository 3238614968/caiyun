package services

import (
	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var (
	ErrAccountNotFound = errors.New("账号不存在")
	ErrAccountExists   = errors.New("账号已存在")
)

type AccountService struct {
	accountRepo *repository.AccountRepository
	userRepo    *repository.UserRepository
	cache       *cache.RedisCache
	authMgr     *auth.Auth
}

func NewAccountService(
	accountRepo *repository.AccountRepository,
	userRepo *repository.UserRepository,
	cache *cache.RedisCache,
	authMgr *auth.Auth,
) *AccountService {
	return &AccountService{
		accountRepo: accountRepo,
		userRepo:    userRepo,
		cache:       cache,
		authMgr:     authMgr,
	}
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
	Auth   string `json:"auth" binding:"required"`
	Remark string `json:"remark" binding:"omitempty"`
}

// CreateAccount 创建账号（如果当前用户已存在则更新）
func (s *AccountService) CreateAccount(userID uint, req *CreateAccountRequest) (*models.Account, error) {
	// 验证用户存在
	_, err := s.userRepo.FindByID(userID)
	if err != nil {
		return nil, ErrAccountNotFound
	}

	// 检查该用户是否已存在该手机号
	existingAccount, err := s.accountRepo.FindByPhoneAndUserID(req.Phone, userID)
	if err == nil && existingAccount != nil {
		// 已存在，更新账号信息
		existingAccount.Auth = req.Auth
		existingAccount.Remark = req.Remark
		existingAccount.IsActive = true
		existingAccount.JWTErrorCount = 0 // 重置JWT错误计数

		// 尝试从 Auth 中解析 token/平台/过期时间
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

	// 尝试从 Auth 中解析 token/平台/过期时间，避免首次使用时 token 为空导致刷新失败
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

// GetAccount 获取账号详情（带权限检查）
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
	account.Auth = req.Auth
	account.Remark = req.Remark

	// 同步解析 Auth 到 token/平台/过期时间
	if info, err := auth.ParseToken(req.Auth); err == nil && info != nil {
		account.Token = info.Token
		account.ExpireAt = info.Expire
		if info.Platform != "" {
			account.Platform = info.Platform
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
	// 先确保 account.Token 非空：如果为空则从 account.Auth 解析
	if account.Token == "" {
		info, err := auth.ParseToken(account.Auth)
		if err != nil {
			return fmt.Errorf("Token为空且解析Auth失败: %w", err)
		}
		account.Token = info.Token
		// ParseToken 里会尽力解析过期时间
		account.ExpireAt = info.Expire
		if info.Platform != "" {
			account.Platform = info.Platform
		}
		// 将解析出的 token 写回数据库，后续请求可直接使用
		_ = s.accountRepo.Update(account)
	}

	// 使用 authMgr 刷新 Token（authTokenRefresh.do 需要“纯 token”，不是 Basic Auth）
	newToken, err := s.authMgr.RefreshToken(account.Token, account.Phone)
	if err != nil {
		return err
	}

	// 更新数据库
	account.Token = newToken
	// 设置过期时间（默认30天）
	account.ExpireAt = time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	// 同步更新 Auth（否则后续依赖 Auth 的请求仍用旧 token）
	if account.Platform == "" {
		account.Platform = "pc"
	}
	account.Auth = auth.GenerateAuth(newToken, account.Phone, account.Platform)

	if err := s.accountRepo.Update(account); err != nil {
		return err
	}

	// 更新缓存
	cacheKey := fmt.Sprintf("account:token:%d", account.ID)
	s.cache.Set(cacheKey, newToken, 24*time.Hour)

	return nil
}

// RefreshTokenIfNeeded 根据需要刷新Token
func (s *AccountService) RefreshTokenIfNeeded(account *models.Account) error {
	// 检查是否需要刷新
	now := time.Now().Unix() * 1000 // 转换为毫秒
	if account.ExpireAt > 0 && account.ExpireAt > now {
		// Token未过期，不需要刷新
		return nil
	}

	// 刷新Token
	return s.RefreshToken(account)
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

// EnqueueTask 将任务加入队列
func (s *AccountService) EnqueueTask(accountID uint, taskType string) error {
	// 获取账号信息
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return err
	}

	// 使用Redis List实现队列
	cacheKey := fmt.Sprintf("task:queue:pending")
	message := map[string]interface{}{
		"account_id": accountID,
		"user_id":    account.UserID,
		"task_type":  taskType,
		"created_at": time.Now().Unix(),
	}

	// 序列化消息
	data, err := json.Marshal(message)
	if err != nil {
		return err
	}

	// 推入队列
	return s.cache.LPush(cacheKey, string(data))
}
