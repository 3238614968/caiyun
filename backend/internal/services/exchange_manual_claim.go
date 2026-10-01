package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

type exchangeManualClaimRepository interface {
	GetByID(uint) (*models.ExchangeTask, error)
	TryMarkForManualExecution(uint, uint, string, time.Time) (bool, string, error)
	RecoverStaleRunningTask(uint, time.Time, string) (bool, error)
}

type exchangeManualClaim struct {
	task    *models.ExchangeTask
	started bool
	token   string
	reason  string
}

func claimExchangeTaskManually(repo exchangeManualClaimRepository, task *models.ExchangeTask, now time.Time, timeout time.Duration) (exchangeManualClaim, error) {
	claim := exchangeManualClaim{task: task}
	if task == nil || task.ID == 0 || task.UserID == 0 {
		return claim, ErrExchangeInvalidInput
	}
	// Retry a changed snapshot a bounded number of times. No unowned failed ->
	// pending write is issued, so another Worker cannot slip into a reset gap.
	for i := 0; i < 3; i++ {
		current := claim.task
		if current.UserID != task.UserID {
			return claim, ErrExchangePermissionDenied
		}
		switch current.Status {
		case string(models.ExchangeTaskFailed):
			if strings.Contains(current.LastResult, "兑换结果待确认") || strings.Contains(current.LastResult, "任务执行异常") {
				claim.reason = "上次兑换结果待确认，请先核对领奖专区及兑换记录，不能直接重复抢兑"
				return claim, nil
			}
			fallthrough
		case string(models.ExchangeTaskPending):
			started, token, err := repo.TryMarkForManualExecution(current.ID, current.UserID, current.Status, current.UpdatedAt)
			if errors.Is(err, repository.ErrDuplicateActiveExchangeTask) {
				return claim, ErrExchangeTaskAlreadyExists
			}
			if err != nil {
				return claim, fmt.Errorf("手动抢兑抢占执行权: %w", err)
			}
			if started {
				claim.started, claim.token = true, token
				current.Status = string(models.ExchangeTaskRunning)
				current.ExecutionToken = token
				current.RetryCount = 0
				current.LastRetryAt = nil
				current.LastResult = ""
				current.SkipReason = ""
				return claim, nil
			}
		case string(models.ExchangeTaskRunning):
			if !current.UpdatedAt.Before(now.Add(-timeout)) {
				claim.reason = exchangeTaskClaimConflictReason(current.Status)
				return claim, nil
			}
			if _, err := repo.RecoverStaleRunningTask(current.ID, now.Add(-timeout), "任务执行超时，手动重试前已恢复为待执行"); err != nil {
				return claim, fmt.Errorf("恢复超时抢兑任务: %w", err)
			}
		default:
			claim.reason = exchangeTaskClaimConflictReason(current.Status)
			return claim, nil
		}
		latest, err := repo.GetByID(task.ID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return claim, ErrExchangeTaskNotFound
		}
		if err != nil {
			return claim, fmt.Errorf("读取任务当前状态: %w", err)
		}
		if latest == nil {
			return claim, ErrExchangeTaskNotFound
		}
		if latest.UserID != task.UserID {
			return claim, ErrExchangePermissionDenied
		}
		claim.task = latest
	}
	claim.reason = exchangeTaskClaimConflictReason(claim.task.Status)
	return claim, nil
}

func exchangeTaskClaimConflictReason(status string) string {
	switch status {
	case string(models.ExchangeTaskRunning):
		return "任务正在执行，请稍后查看结果"
	case string(models.ExchangeTaskCompleted):
		return "任务已完成，不能重复执行；如需再次抢兑请创建新任务"
	case "cancelled", "canceled":
		return "任务已取消，不能执行"
	case string(models.ExchangeTaskFailed):
		return "任务状态已变更，本次未取得重试执行权，请刷新后重试"
	default:
		return "任务状态已变更或已被其他实例取得执行权，请刷新后重试"
	}
}
