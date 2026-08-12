package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"errors"
)

// GetAllAccounts 获取所有账号
func (s *AdminService) GetAllAccounts(page, pageSize int, phones ...string) ([]*models.Account, int64, error) {
	offset := (page - 1) * pageSize
	return s.accountRepo.List(offset, pageSize, phones...)
}

// SearchAllAccountsRequest 搜索所有账号请求
type SearchAllAccountsRequest struct {
	Keyword string `json:"keyword" form:"keyword"`
	Limit   int    `json:"limit" form:"limit"`
}

// SearchAllAccountsResponse 搜索所有账号响应
type SearchAllAccountsResponse struct {
	Accounts []*AccountSearchItem `json:"accounts"`
}

// AccountSearchItem 账号搜索项
type AccountSearchItem struct {
	ID       uint   `json:"id"`
	Phone    string `json:"phone"`
	Remark   string `json:"remark"`
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

// SearchAllAccounts 搜索所有账号（管理员用）
func (s *AdminService) SearchAllAccounts(req *SearchAllAccountsRequest) (*SearchAllAccountsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	accounts, err := s.accountRepo.SearchAll(req.Keyword, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*AccountSearchItem, 0, len(accounts))
	for _, acc := range accounts {
		username := ""
		if acc.User.ID > 0 {
			username = acc.User.Username
		}

		result = append(result, &AccountSearchItem{
			ID:       acc.ID,
			Phone:    acc.Phone,
			Remark:   acc.Remark,
			UserID:   acc.UserID,
			Username: username,
			IsActive: acc.IsActive,
		})
	}

	return &SearchAllAccountsResponse{
		Accounts: result,
	}, nil
}

// UpdateAccountStatusRequest 更新账号状态请求
type UpdateAccountStatusRequest struct {
	IsActive bool `json:"is_active"`
}

// UpdateAccountStatus 更新账号状态
func (s *AdminService) UpdateAccountStatus(accountID uint, req *UpdateAccountStatusRequest) error {
	return s.accountRepo.SetActiveStatus(accountID, req.IsActive)
}

// DeleteAccount 删除账号。
func (s *AdminService) DeleteAccount(accountID uint) error {
	return s.DeleteAccountContext(context.Background(), accountID)
}

// DeleteAccountContext atomically removes all data derived from a cloud
// account and overwrites credentials before physically deleting the account.
func (s *AdminService) DeleteAccountContext(ctx context.Context, accountID uint) error {
	if s.unitOfWork == nil {
		return errors.New("unit of work is not configured")
	}
	return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
		if _, err := repos.Account.FindByID(accountID); err != nil {
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
