package repository

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Execution lease methods use fencing tokens for every running-state write.

// TryMarkRunning 以条件更新方式抢占任务执行权，并返回本次执行的 fencing token。
// 后续状态写入必须携带该 token，避免超时回收后的陈旧 Worker 覆盖新执行结果。
func (r *ExchangeTaskRepository) TryMarkRunning(id uint) (bool, string, error) {
	executionToken := uuid.NewString()
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ?", id, string(models.ExchangeTaskPending)).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskRunning),
			"execution_token": executionToken,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return false, "", result.Error
	}
	if result.RowsAffected == 0 {
		return false, "", nil
	}
	return true, executionToken, nil
}

// ReleaseRunning returns a task claimed by the current execution to pending.
// The fencing token prevents cancellation cleanup from overwriting a newer
// execution that reclaimed the same task.
func (r *ExchangeTaskRepository) ReleaseRunning(id uint, executionToken, reason string) (bool, error) {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return false, ErrExchangeTaskExecutionLost
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, string(models.ExchangeTaskRunning), executionToken).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskPending),
			"execution_token": "",
			"last_result":     strings.TrimSpace(reason),
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RecoverStaleRunning 将长时间停留在 running 的抢兑任务恢复为 pending。
// 这主要用于 Worker/进程在任务完成前崩溃后的自愈，避免任务永久卡住。
func (r *ExchangeTaskRepository) RecoverStaleRunning(timeout time.Duration) (int64, error) {
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	cutoff := time.Now().Add(-timeout)
	message := fmt.Sprintf("任务执行超过 %s 未完成，已自动恢复为待执行", timeout)
	result := r.db.Model(&models.ExchangeTask{}).
		Where("status = ? AND updated_at < ?", string(models.ExchangeTaskRunning), cutoff).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskPending),
			"execution_token": "",
			"last_result":     message,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return 0, result.Error
	}
	return result.RowsAffected, nil
}

// RecoverStaleRunningTask conditionally recovers one stale running task.
// The updated_at predicate prevents a fresh heartbeat from being overwritten.
func (r *ExchangeTaskRepository) RecoverStaleRunningTask(id uint, staleBefore time.Time, reason string) (bool, error) {
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND updated_at < ?", id, string(models.ExchangeTaskRunning), staleBefore).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskPending),
			"execution_token": "",
			"last_result":     strings.TrimSpace(reason),
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// RenewRunning refreshes a running task lease owned by executionToken.
func (r *ExchangeTaskRepository) RenewRunning(id uint, executionToken string, now time.Time) (bool, error) {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return false, ErrExchangeTaskExecutionLost
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, string(models.ExchangeTaskRunning), executionToken).
		Update("updated_at", now)
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
}

// TransitionRunning finishes or releases the execution only when the caller
// still owns the current fencing token.
func (r *ExchangeTaskRepository) TransitionRunning(id uint, executionToken, status, resultMessage string) error {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, string(models.ExchangeTaskRunning), executionToken).
		Updates(map[string]interface{}{
			"status":          status,
			"execution_token": "",
			"last_result":     strings.TrimSpace(resultMessage),
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return ErrExchangeTaskExecutionLost
	}
	return nil
}

// UpdateAttempt 更新任务抢兑尝试
func (r *ExchangeTaskRepository) UpdateAttempt(id uint, success bool, result string) error {
	updates := map[string]interface{}{
		"attempted_count": gorm.Expr("attempted_count + 1"),
		"last_result":     result,
		"last_attempt_at": time.Now(),
	}

	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
	} else {
		updates["fail_count"] = gorm.Expr("fail_count + 1")
	}

	return r.db.Model(&models.ExchangeTask{}).
		Where("id = ?", id).
		Updates(updates).Error
}

// UpdateAttemptOwned records an attempt only for the current execution owner.
func (r *ExchangeTaskRepository) UpdateAttemptOwned(id uint, executionToken string, success bool, result string) error {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	updates := map[string]interface{}{
		"attempted_count": gorm.Expr("attempted_count + 1"),
		"last_result":     result,
		"last_attempt_at": time.Now(),
	}
	if success {
		updates["success_count"] = gorm.Expr("success_count + 1")
	} else {
		updates["fail_count"] = gorm.Expr("fail_count + 1")
	}

	write := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ?", id, string(models.ExchangeTaskRunning), executionToken).
		Updates(updates)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return ErrExchangeTaskExecutionLost
	}
	return nil
}

// FinalizeOwned atomically records an execution result and transitions the
// task out of running.  The fencing token is checked in the same transaction
// as the record insert, so a stale Worker cannot create a result or overwrite
// counters after a newer Worker reclaimed the lease.
func (r *ExchangeTaskRepository) FinalizeOwned(task *models.ExchangeTask, executionToken string, success bool, resultMessage string, status models.ExchangeTaskStatus, executionTimeMs int) error {
	if task == nil || task.ID == 0 {
		return errors.New("exchange task is required")
	}
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"attempted_count": gorm.Expr("attempted_count + 1"),
			"last_result":     strings.TrimSpace(resultMessage),
			"last_attempt_at": time.Now(),
			"status":          string(status),
			"execution_token": "",
			"updated_at":      time.Now(),
		}
		if success {
			updates["success_count"] = gorm.Expr("success_count + 1")
		} else {
			updates["fail_count"] = gorm.Expr("fail_count + 1")
		}
		write := tx.Model(&models.ExchangeTask{}).
			Where("id = ? AND status = ? AND execution_token = ?", task.ID, string(models.ExchangeTaskRunning), executionToken).
			Updates(updates)
		if write.Error != nil {
			return write.Error
		}
		if write.RowsAffected != 1 {
			return ErrExchangeTaskExecutionLost
		}

		recordStatus := models.ExchangeRecordSuccess
		if !success {
			recordStatus = models.ExchangeRecordFailed
		}
		record := &models.ExchangeRecord{
			UserID:            task.UserID,
			ExchangeAccountID: task.ExchangeAccountID,
			ExchangeTaskID:    &task.ID,
			ProductID:         task.ProductID,
			PrizeID:           task.PrizeID,
			PrizeName:         task.PrizeName,
			Status:            string(recordStatus),
			Message:           resultMessage,
			ExecutionTimeMs:   executionTimeMs,
		}
		return tx.Create(record).Error
	})
}
