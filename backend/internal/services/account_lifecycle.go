package services

import (
	"context"
	"strings"

	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/pkg/validator"
)

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
