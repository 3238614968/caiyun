package repository

import (
	"errors"
	"strings"
	"time"

	"caiyun/internal/models"

	"gorm.io/gorm"
)

// Write methods keep mutable task-definition and retry metadata separate from
// execution-lease transitions, so callers can make ownership boundaries clear.

// Create 创建抢兑任务
func (r *ExchangeTaskRepository) Create(task *models.ExchangeTask) error {
	return mapExchangeTaskWriteError(r.db.Create(task).Error)
}

// FindBySourceOperationID returns the task created by a durable operation.
// A nil task with nil error means no task has been linked yet.
func (r *ExchangeTaskRepository) FindBySourceOperationID(operationID string) (*models.ExchangeTask, error) {
	operationID = strings.TrimSpace(operationID)
	if operationID == "" {
		return nil, nil
	}
	var task models.ExchangeTask
	err := r.db.Where("source_operation_id = ?", operationID).First(&task).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// UpdateTaskDefinition 精确更新任务定义快照，避免 Save 全量覆盖并发修改的其他列。
func (r *ExchangeTaskRepository) UpdateTaskDefinition(id, productID uint, prizeID, prizeName, taskType string, maxAttempts int, status string) error {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	updates := map[string]interface{}{
		"product_id":   productID,
		"prize_id":     prizeID,
		"prize_name":   prizeName,
		"task_type":    taskType,
		"max_attempts": maxAttempts,
		"updated_at":   time.Now(),
	}
	if status != "" {
		updates["status"] = status
	}
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateMaxAttempts 更新任务最大执行次数，避免全量覆盖任务快照。
func (r *ExchangeTaskRepository) UpdateMaxAttempts(id uint, maxAttempts int) error {
	if maxAttempts <= 0 {
		maxAttempts = 1
	}
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"max_attempts": maxAttempts,
			"updated_at":   time.Now(),
		}).Error
}

// Delete 删除抢兑任务
func (r *ExchangeTaskRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeTask{}, id).Error
}

// UpdateStatus 更新任务状态
func (r *ExchangeTaskRepository) UpdateStatus(id uint, status string) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Update("status", status).Error
}

// UpdateLastResult 更新任务最近结果说明，但不增加尝试次数。
// 用于调度层跳过本月同系列任务时给前端展示原因，同时避免生成新的抢兑记录。
func (r *ExchangeTaskRepository) UpdateLastResult(id uint, result string) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Update("last_result", result).Error
}

func (r *ExchangeTaskRepository) UpdatePrizeSnapshot(id, productID uint, prizeID, prizeName string) error {
	updates := map[string]interface{}{
		"product_id": productID,
		"prize_id":   prizeID,
		"prize_name": prizeName,
		"updated_at": time.Now(),
	}
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateSkipReason 更新任务最近一次调度跳过原因。
func (r *ExchangeTaskRepository) UpdateSkipReason(id uint, reason string) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"skip_reason": reason,
			"updated_at":  time.Now(),
		}).Error
}

// UpdatePendingLastResult annotates a task only while it is still pending.
func (r *ExchangeTaskRepository) UpdatePendingLastResult(id uint, result string) (bool, error) {
	write := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ?", id, string(models.ExchangeTaskPending)).
		Updates(map[string]interface{}{
			"last_result": strings.TrimSpace(result),
			"updated_at":  time.Now(),
		})
	if write.Error != nil {
		return false, write.Error
	}
	return write.RowsAffected == 1, nil
}

// ActiveTaskExists checks for an active task and preserves database errors.
func (r *ExchangeTaskRepository) ActiveTaskExists(userID uint, accountID uint, prizeID string) (bool, error) {
	var count int64
	err := r.db.Model(&models.ExchangeTask{}).
		Where("user_id = ? AND exchange_rule_id = ? AND prize_id = ? AND status IN ?",
			userID, accountID, prizeID, []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
		Count(&count).Error
	return count > 0, err
}

// CheckTaskExists is retained for compatibility. New write paths must call
// ActiveTaskExists so query failures cannot be mistaken for "not found".
func (r *ExchangeTaskRepository) CheckTaskExists(userID uint, accountID uint, prizeID string) bool {
	exists, _ := r.ActiveTaskExists(userID, accountID, prizeID)
	return exists
}

// BatchUpdateStatus 批量更新任务状态
func (r *ExchangeTaskRepository) BatchUpdateStatus(ids []uint, status string) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id IN ?", ids).
		Update("status", status).Error
}

// UpdateRetry 更新任务重试次数
func (r *ExchangeTaskRepository) UpdateRetry(id uint, retryCount int) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"retry_count":   retryCount,
			"last_retry_at": time.Now(),
			"status":        models.ExchangeTaskPending, // 重置为待执行状态
		}).Error
}

// UpdateRetryCount 更新任务重试次数和时间
func (r *ExchangeTaskRepository) UpdateRetryCount(id uint, retryCount int, lastRetryAt *time.Time) error {
	updates := map[string]interface{}{
		"retry_count": retryCount,
	}
	if lastRetryAt != nil {
		updates["last_retry_at"] = *lastRetryAt
	}
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateRetryCountOwned updates retry metadata only while the supplied Worker
// owns the running task.  Retrying code must never mutate a task reclaimed by
// a newer execution.
func (r *ExchangeTaskRepository) UpdateRetryCountOwned(id uint, executionToken string, retryCount int, lastRetryAt *time.Time) error {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	updates := map[string]interface{}{
		"retry_count": retryCount,
		"updated_at":  time.Now(),
	}
	if lastRetryAt != nil {
		updates["last_retry_at"] = *lastRetryAt
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, string(models.ExchangeTaskRunning), executionToken).
		Updates(updates)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrExchangeTaskExecutionLost
	}
	return nil
}

// IncrementSuccessCount 增加任务成功次数
func (r *ExchangeTaskRepository) IncrementSuccessCount(id uint) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		UpdateColumn("success_count", gorm.Expr("success_count + 1")).
		Error
}

// IncrementFailCount 增加任务失败次数
func (r *ExchangeTaskRepository) IncrementFailCount(id uint) error {
	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		UpdateColumn("fail_count", gorm.Expr("fail_count + 1")).
		Error
}
