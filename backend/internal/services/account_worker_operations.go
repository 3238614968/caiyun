package services

import (
	"context"
	"errors"

	"caiyun/internal/models"
	"caiyun/internal/repository"
)

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

	if s.taskQueue == nil {
		return errors.New("reliable task queue is not configured")
	}
	return s.taskQueue.Enqueue(account.ID, account.UserID, taskType)
}
