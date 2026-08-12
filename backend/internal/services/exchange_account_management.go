package services

import (
	"context"
	"errors"
	"fmt"

	"caiyun/internal/models"
	"caiyun/internal/repository"
)

// AddExchangeAccount 添加兑换账号
func (s *ExchangeService) AddExchangeAccount(userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string, productID *uint) (*models.ExchangeAccount, error) {
	return s.AddExchangeAccountContext(context.Background(), userID, accountID, remark, exchangeTime1, exchangeTime2, productID)
}

func (s *ExchangeService) AddExchangeAccountContext(ctx context.Context, userID uint, accountID uint, remark string, exchangeTime1, exchangeTime2 string, productID *uint) (*models.ExchangeAccount, error) {
	var created *models.ExchangeAccount
	err := s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.accountRepo.GetByID(accountID)
		if err != nil || account.UserID != userID {
			return ErrExchangeCloudAccountMissing
		}
		existing, err := txService.exchangeAccountRepo.FindByAccountID(accountID)
		if err != nil {
			return fmt.Errorf("query exchange rule: %w", err)
		}
		if existing != nil {
			return ErrExchangeTaskConflict
		}

		created = &models.ExchangeAccount{
			UserID:        userID,
			AccountID:     accountID,
			Phone:         account.Phone,
			Auth:          account.Auth,
			Token:         account.Token,
			JWTToken:      account.JWTToken,
			Remark:        remark,
			ExchangeTime1: exchangeTime1,
			ExchangeTime2: exchangeTime2,
			IsActive:      true,
		}
		if err := txService.exchangeAccountRepo.Create(created); err != nil {
			if errors.Is(err, repository.ErrDuplicateExchangeRule) {
				return ErrExchangeTaskConflict
			}
			return fmt.Errorf("create exchange rule: %w", err)
		}
		if productID != nil && *productID > 0 {
			if err := txService.syncScheduledTaskForAccount(created, *productID); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return created, nil
}

// GetExchangeAccounts 获取用户的兑换账号列表
func (s *ExchangeService) GetExchangeAccounts(userID uint, isAdmin bool) ([]*models.ExchangeAccount, error) {
	return s.GetExchangeAccountsContext(context.Background(), userID, isAdmin)
}

func (s *ExchangeService) GetExchangeAccountsContext(ctx context.Context, userID uint, isAdmin bool) ([]*models.ExchangeAccount, error) {
	repo := s.exchangeAccountRepo.WithContext(ctx)
	if isAdmin {
		return repo.GetAll()
	}
	return repo.GetByUserID(userID)
}
func (s *ExchangeService) syncScheduledTaskForAccount(exchangeAccount *models.ExchangeAccount, productID uint) error {
	product, err := s.productRepo.GetByID(productID)
	if err != nil {
		return ErrExchangeProductNotFound
	}

	tasks, err := s.exchangeTaskRepo.GetByExchangeAccountID(exchangeAccount.ID)
	if err != nil {
		return fmt.Errorf("load scheduled tasks: %w", err)
	}

	if existingTask := findActiveScheduledTask(tasks); existingTask != nil {
		existingTask.ProductID = product.ID
		existingTask.PrizeID = product.PrizeID
		existingTask.PrizeName = product.PrizeName
		existingTask.TaskType = string(models.ExchangeTaskLongTerm)
		if existingTask.MaxAttempts <= 0 {
			existingTask.MaxAttempts = 10
		}
		if existingTask.Status == "" {
			existingTask.Status = string(models.ExchangeTaskPending)
		}
		if err := s.exchangeTaskRepo.UpdateTaskDefinition(
			existingTask.ID,
			product.ID,
			product.PrizeID,
			product.PrizeName,
			string(models.ExchangeTaskLongTerm),
			existingTask.MaxAttempts,
			existingTask.Status,
		); err != nil {
			return fmt.Errorf("update scheduled task: %w", err)
		}
		return nil
	}

	task := &models.ExchangeTask{
		UserID:            exchangeAccount.UserID,
		ExchangeAccountID: exchangeAccount.ID,
		ProductID:         product.ID,
		PrizeID:           product.PrizeID,
		PrizeName:         product.PrizeName,
		TaskType:          string(models.ExchangeTaskLongTerm),
		MaxAttempts:       10,
		AttemptedCount:    0,
		Status:            string(models.ExchangeTaskPending),
		SuccessCount:      0,
		FailCount:         0,
	}

	if err := s.exchangeTaskRepo.Create(task); err != nil {
		if errors.Is(err, repository.ErrDuplicateActiveExchangeTask) {
			return ErrExchangeTaskConflict
		}
		return fmt.Errorf("create scheduled task: %w", err)
	}

	return nil
}

func findActiveScheduledTask(tasks []*models.ExchangeTask) *models.ExchangeTask {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.TaskType != string(models.ExchangeTaskLongTerm) {
			continue
		}
		if task.Status == string(models.ExchangeTaskPending) || task.Status == string(models.ExchangeTaskRunning) {
			return task
		}
	}
	return nil
}

// UpdateExchangeAccount 更新兑换账号配置
func (s *ExchangeService) UpdateExchangeAccount(id uint, userID uint, isAdmin bool, remark string, exchangeTime1, exchangeTime2 string, isActive bool, productID *uint) error {
	return s.UpdateExchangeAccountContext(context.Background(), id, userID, isAdmin, remark, exchangeTime1, exchangeTime2, isActive, productID)
}

func (s *ExchangeService) UpdateExchangeAccountContext(ctx context.Context, id uint, userID uint, isAdmin bool, remark string, exchangeTime1, exchangeTime2 string, isActive bool, productID *uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.exchangeAccountRepo.GetByID(id)
		if err != nil {
			return ErrExchangeRuleNotFound
		}
		if !isAdmin && account.UserID != userID {
			return ErrExchangePermissionDenied
		}
		account.Remark = remark
		account.ExchangeTime1 = exchangeTime1
		account.ExchangeTime2 = exchangeTime2
		account.IsActive = isActive
		if productID != nil && *productID > 0 {
			if err := txService.syncScheduledTaskForAccount(account, *productID); err != nil {
				return err
			}
		}
		if err := txService.exchangeAccountRepo.Update(account); err != nil {
			return fmt.Errorf("update exchange rule: %w", err)
		}
		return nil
	})
}

// DeleteExchangeAccount 删除兑换账号
func (s *ExchangeService) DeleteExchangeAccount(id uint, userID uint) error {
	return s.DeleteExchangeAccountContext(context.Background(), id, userID)
}

func (s *ExchangeService) DeleteExchangeAccountContext(ctx context.Context, id uint, userID uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		account, err := txService.exchangeAccountRepo.GetByID(id)
		if err != nil {
			return ErrExchangeRuleNotFound
		}
		if account.UserID != userID {
			return ErrExchangePermissionDenied
		}
		tasks, err := txService.exchangeTaskRepo.GetByExchangeAccountID(account.ID)
		if err != nil {
			return fmt.Errorf("load exchange-rule tasks: %w", err)
		}
		for _, task := range tasks {
			if err := txService.exchangeTaskRepo.Delete(task.ID); err != nil {
				return fmt.Errorf("delete exchange-rule task: %w", err)
			}
		}
		if err := txService.exchangeAccountRepo.Delete(id); err != nil {
			return fmt.Errorf("delete exchange rule: %w", err)
		}
		return nil
	})
}
