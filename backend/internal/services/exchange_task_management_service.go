package services

import (
	"context"
	"fmt"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"
)

// GetExchangeTasks 获取用户的抢兑任务列表
func (s *ExchangeService) GetExchangeTasks(userID uint, isAdmin bool) ([]*models.ExchangeTask, error) {
	return s.GetExchangeTasksWithFilter(userID, isAdmin, repository.ExchangeTaskFilter{})
}

// GetExchangeTasksWithFilter 获取用户的抢兑任务列表并填充只读调度预览字段。
func (s *ExchangeService) GetExchangeTasksWithFilter(userID uint, isAdmin bool, filter repository.ExchangeTaskFilter) ([]*models.ExchangeTask, error) {
	var (
		tasks []*models.ExchangeTask
		err   error
	)
	if isAdmin {
		tasks, err = s.exchangeTaskRepo.GetAllWithFilter(filter)
	} else {
		tasks, err = s.exchangeTaskRepo.GetByUserIDWithFilter(userID, filter)
	}
	if err != nil {
		return nil, err
	}
	s.fillExchangeTaskRuntimePreview(tasks, time.Now())
	return tasks, nil
}

func (s *ExchangeService) fillExchangeTaskRuntimePreview(tasks []*models.ExchangeTask, now time.Time) {
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if s != nil && s.exchangeTaskRepo != nil {
			task.NextRunAt = s.exchangeTaskRepo.CalculateNextRun(task, now)
		} else {
			task.NextRunAt = repository.CalculateExchangeTaskNextRun(task, now)
		}
	}
}

// UpdateExchangeTask 更新抢兑任务
func (s *ExchangeService) UpdateExchangeTask(id uint, userID uint, maxAttempts int) error {
	return s.UpdateExchangeTaskContext(context.Background(), id, userID, maxAttempts)
}

func (s *ExchangeService) UpdateExchangeTaskContext(ctx context.Context, id uint, userID uint, maxAttempts int) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		task, err := txService.exchangeTaskRepo.GetByID(id)
		if err != nil {
			return ErrExchangeTaskNotFound
		}
		if task.UserID != userID {
			return ErrExchangePermissionDenied
		}
		if err := txService.exchangeTaskRepo.UpdateMaxAttempts(task.ID, maxAttempts); err != nil {
			return fmt.Errorf("update exchange task: %w", err)
		}
		return nil
	})
}

// DeleteExchangeTask 删除抢兑任务
func (s *ExchangeService) DeleteExchangeTask(id uint, userID uint) error {
	return s.DeleteExchangeTaskContext(context.Background(), id, userID)
}

func (s *ExchangeService) DeleteExchangeTaskContext(ctx context.Context, id uint, userID uint) error {
	return s.withinTransaction(ctx, func(txService *ExchangeService) error {
		task, err := txService.exchangeTaskRepo.GetByID(id)
		if err != nil {
			return ErrExchangeTaskNotFound
		}
		if task.UserID != userID {
			return ErrExchangePermissionDenied
		}
		if err := txService.exchangeTaskRepo.Delete(id); err != nil {
			return fmt.Errorf("delete exchange task: %w", err)
		}
		return nil
	})
}
