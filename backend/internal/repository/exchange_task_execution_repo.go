package repository

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"caiyun/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ExchangeSettlement is the only terminal result accepted by FinalizeOwned.
// A submitted result must pass through the durable unknown state first.
type ExchangeSettlement struct {
	OperationID     string
	Outcome         models.ExchangeOutcome
	Message         string
	Status          models.ExchangeTaskStatus
	ExecutionTimeMs int
}

func (s ExchangeSettlement) success() bool {
	return s.Outcome == models.ExchangeOutcomeAccepted
}

// ExchangeRecoveryResult distinguishes resumable work from a submitted result
// that needs reconciliation. Only the latter creates an exchange record.
type ExchangeRecoveryResult struct {
	Resumed int64
	Unknown []*models.ExchangeTask
}

// Execution lease methods use fencing tokens for every running-state write.

// TryMarkRunning 以条件更新方式抢占任务执行权，并返回本次执行的 fencing token。
// 后续状态写入必须携带该 token，避免超时回收后的陈旧 Worker 覆盖新执行结果。
func (r *ExchangeTaskRepository) TryMarkRunning(id uint) (bool, string, error) {
	executionToken := uuid.NewString()
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND outcome <> ?", id, string(models.ExchangeTaskPending), string(models.ExchangeOutcomeUnknown)).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskRunning),
			"execution_token": executionToken,
			"outcome":         string(models.ExchangeOutcomeNotSubmitted),
			"retry_count":     0,
			"last_retry_at":   nil,
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

// TryMarkForManualExecution atomically claims a pending or failed task for an
// explicit user action. The owner and snapshot predicates fence stale requests.
func (r *ExchangeTaskRepository) TryMarkForManualExecution(id, userID uint, expectedStatus string, expectedUpdatedAt time.Time, operationID string) (bool, string, error) {
	if expectedStatus != string(models.ExchangeTaskPending) && expectedStatus != string(models.ExchangeTaskFailed) {
		return false, "", nil
	}
	executionToken := uuid.NewString()
	claim := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND user_id = ? AND status = ? AND updated_at = ? AND outcome <> ? AND last_result NOT LIKE ? AND last_result NOT LIKE ?",
			id, userID, expectedStatus, expectedUpdatedAt, string(models.ExchangeOutcomeUnknown), "%兑换结果待确认%", "%任务执行异常%")
	if operationID != "" {
		completed := r.db.Model(&ExchangeExecutionReceipt{}).Select("1").Where(
			"operation_id = ? AND resource_kind = ? AND resource_id = ? AND completed_at IS NOT NULL", operationID, ExchangeTaskReceiptKind, id)
		claim = claim.Where("NOT EXISTS (?)", completed)
	}
	result := claim.Updates(map[string]interface{}{
		"status":          string(models.ExchangeTaskRunning),
		"execution_token": executionToken,
		"outcome":         string(models.ExchangeOutcomeNotSubmitted),
		"retry_count":     0,
		"last_retry_at":   nil,
		"last_result":     "",
		"skip_reason":     "",
		"updated_at":      time.Now(),
	})
	if result.Error != nil {
		return false, "", mapExchangeTaskWriteError(result.Error)
	}
	if result.RowsAffected == 0 {
		return false, "", nil
	}
	return true, executionToken, nil
}

// MarkSubmissionOwned persists the point beyond which a request may be sent.
// A successful update means the caller must conservatively recover as unknown
// even if cancellation prevents the actual HTTP write.
func (r *ExchangeTaskRepository) MarkSubmissionOwned(id uint, executionToken string) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		enabled, err := NewSystemConfigRepository(tx.Clauses(clause.Locking{Strength: "SHARE"})).ExchangeEnabled()
		if err != nil {
			return fmt.Errorf("读取抢兑开关失败: %w", err)
		}
		if !enabled {
			return errors.New("抢兑功能已关闭")
		}
		result := tx.Model(&models.ExchangeTask{}).
			Where("id = ? AND status = ? AND execution_token = ? AND outcome = ?",
				id, string(models.ExchangeTaskRunning), executionToken, string(models.ExchangeOutcomeNotSubmitted)).
			Updates(map[string]interface{}{
				"outcome":    string(models.ExchangeOutcomeUnknown),
				"updated_at": time.Now(),
			})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrExchangeTaskExecutionLost
		}
		return nil
	})
}

// ReleaseRunning releases only work that provably did not pass the submission
// boundary. Unknown work must be settled, never returned to pending.
func (r *ExchangeTaskRepository) ReleaseRunning(id uint, executionToken, reason string) (bool, error) {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return false, ErrExchangeTaskExecutionLost
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ? AND outcome IN ?",
			id, string(models.ExchangeTaskRunning), executionToken,
			[]string{"", string(models.ExchangeOutcomeNotSubmitted)}).
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

// RecoverStaleRunning resumes only unsubmitted work. Submitted unknown rows
// are settled individually in transactions; failures remain for the next pass.
func (r *ExchangeTaskRepository) RecoverStaleRunning(timeout time.Duration) (int64, error) {
	result, err := r.RecoverStaleRunningWithResults(timeout)
	if err != nil {
		return 0, err
	}
	return result.Resumed, nil
}

func (r *ExchangeTaskRepository) RecoverStaleRunningWithResults(timeout time.Duration) (ExchangeRecoveryResult, error) {
	var summary ExchangeRecoveryResult
	if timeout <= 0 {
		timeout = 15 * time.Minute
	}
	cutoff := time.Now().Add(-timeout)
	message := fmt.Sprintf("任务执行超过 %s 未完成，已自动恢复为待执行", timeout)
	result := r.db.Model(&models.ExchangeTask{}).
		Where("status = ? AND updated_at < ? AND outcome IN ?", string(models.ExchangeTaskRunning), cutoff,
			[]string{"", string(models.ExchangeOutcomeNotSubmitted)}).
		Updates(map[string]interface{}{
			"status":          string(models.ExchangeTaskPending),
			"execution_token": "",
			"last_result":     message,
			"updated_at":      time.Now(),
		})
	if result.Error != nil {
		return summary, result.Error
	}
	summary.Resumed = result.RowsAffected

	var tasks []*models.ExchangeTask
	if err := r.db.Where("status = ? AND outcome = ? AND updated_at < ?",
		string(models.ExchangeTaskRunning), string(models.ExchangeOutcomeUnknown), cutoff).
		Order("updated_at ASC").Limit(100).Find(&tasks).Error; err != nil {
		return summary, err
	}
	for _, task := range tasks {
		if err := r.FinalizeStaleSubmission(task, cutoff, ""); errors.Is(err, ErrExchangeTaskExecutionLost) {
			continue
		} else if err != nil {
			return summary, err
		}
		now := time.Now()
		attemptedAt := now
		summary.Unknown = append(summary.Unknown, &models.ExchangeTask{
			ID: task.ID, UserID: task.UserID, ExchangeAccountID: task.ExchangeAccountID,
			ProductID: task.ProductID, PrizeID: task.PrizeID, PrizeName: task.PrizeName,
			Status: string(models.ExchangeTaskFailed), Outcome: string(models.ExchangeOutcomeUnknown),
			AttemptedCount: task.AttemptedCount + 1, FailCount: task.FailCount + 1,
			LastAttemptAt: &attemptedAt, UpdatedAt: now,
			LastResult: "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换",
		})
	}
	return summary, nil
}

// RecoverStaleRunningTask is kept for the manual-claim compatibility path.
// Unknown work is deliberately not reclaimed as a fresh attempt here; the
// execution entry point settles it before returning a reconciliation error.
func (r *ExchangeTaskRepository) RecoverStaleRunningTask(id uint, staleBefore time.Time, reason string) (bool, error) {
	var task models.ExchangeTask
	if err := r.db.First(&task, id).Error; err != nil {
		return false, err
	}
	if task.Status == string(models.ExchangeTaskRunning) && task.Outcome != "" && task.Outcome != string(models.ExchangeOutcomeNotSubmitted) {
		settlement := ExchangeSettlement{Outcome: models.ExchangeOutcomeUnknown, Status: models.ExchangeTaskFailed, Message: "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"}
		if err := r.db.Transaction(func(tx *gorm.DB) error {
			return finalizeExchangeTx(tx, &task, settlement,
				"id = ? AND status = ? AND outcome = ? AND updated_at < ?",
				[]interface{}{id, string(models.ExchangeTaskRunning), task.Outcome, staleBefore})
		}); err != nil {
			return false, err
		}
		return false, nil
	}
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND updated_at < ? AND outcome IN ?",
			id, string(models.ExchangeTaskRunning), staleBefore,
			[]string{"", string(models.ExchangeOutcomeNotSubmitted)}).
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

// TransitionRunning releases only work that has not crossed submission. Monthly
// guards and other preflight skips therefore cannot clear an in-flight result.
func (r *ExchangeTaskRepository) TransitionRunning(id uint, executionToken, status, resultMessage string) error {
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	if status != string(models.ExchangeTaskPending) {
		return fmt.Errorf("unsupported pre-submission transition %q", status)
	}
	return r.releaseUnsubmitted(id, executionToken, status, resultMessage)
}

func (r *ExchangeTaskRepository) releaseUnsubmitted(id uint, executionToken, status, resultMessage string) error {
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ? AND execution_token = ? AND outcome IN ?",
			id, string(models.ExchangeTaskRunning), executionToken,
			[]string{"", string(models.ExchangeOutcomeNotSubmitted)}).
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
// task out of running. Expected outcome prevents an owner from finalizing a
// pre-submission result after another attempt has already crossed submission.
func (r *ExchangeTaskRepository) FinalizeOwned(task *models.ExchangeTask, executionToken string, settlement ExchangeSettlement) error {
	if task == nil || task.ID == 0 {
		return errors.New("exchange task is required")
	}
	executionToken = strings.TrimSpace(executionToken)
	if executionToken == "" {
		return ErrExchangeTaskExecutionLost
	}
	expected, err := settlementPrecondition(settlement.Outcome)
	if err != nil {
		return err
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return finalizeExchangeTx(tx, task, settlement, "id = ? AND status = ? AND execution_token = ? AND outcome = ?",
			[]interface{}{task.ID, string(models.ExchangeTaskRunning), executionToken, expected})
	})
}

func settlementPrecondition(outcome models.ExchangeOutcome) (string, error) {
	switch outcome {
	case models.ExchangeOutcomeNotSubmitted:
		return string(models.ExchangeOutcomeNotSubmitted), nil
	case models.ExchangeOutcomeAccepted, models.ExchangeOutcomeRejected, models.ExchangeOutcomeUnknown:
		return string(models.ExchangeOutcomeUnknown), nil
	default:
		return "", fmt.Errorf("invalid exchange settlement outcome %q", outcome)
	}
}

func finalizeExchangeTx(tx *gorm.DB, task *models.ExchangeTask, settlement ExchangeSettlement, where string, args []interface{}) error {
	if settlement.Outcome == models.ExchangeOutcomeUnknown {
		settlement.Status = models.ExchangeTaskFailed
	}
	updates := map[string]interface{}{
		"attempted_count": gorm.Expr("attempted_count + 1"),
		"last_result":     strings.TrimSpace(settlement.Message),
		"last_attempt_at": time.Now(),
		"status":          string(settlement.Status),
		"outcome":         string(settlement.Outcome),
		"execution_token": "",
		"updated_at":      time.Now(),
	}
	if settlement.success() {
		updates["success_count"] = gorm.Expr("success_count + 1")
	} else {
		updates["fail_count"] = gorm.Expr("fail_count + 1")
	}
	write := tx.Model(&models.ExchangeTask{}).Where(where, args...).Updates(updates)
	if write.Error != nil {
		return write.Error
	}
	if write.RowsAffected != 1 {
		return ErrExchangeTaskExecutionLost
	}
	recordStatus := models.ExchangeRecordFailed
	if settlement.success() {
		recordStatus = models.ExchangeRecordSuccess
	}
	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            string(recordStatus),
		Outcome:           string(settlement.Outcome),
		Message:           settlement.Message,
		ExecutionTimeMs:   settlement.ExecutionTimeMs,
	}
	if err := tx.Create(record).Error; err != nil {
		return err
	}
	if settlement.OperationID == "" {
		return nil
	}
	completedAt := time.Now()
	return tx.Create(&ExchangeExecutionReceipt{
		OperationID: settlement.OperationID, ResourceID: task.ID, ResourceKind: ExchangeTaskReceiptKind,
		UserID: task.UserID, ProductID: task.ProductID, Outcome: record.Outcome, Message: record.Message,
		ExecutionTimeMs: record.ExecutionTimeMs, CompletedAt: &completedAt,
	}).Error
}

// RecoverSubmittedUnknown settles an abandoned submitted attempt atomically.
// It deliberately preserves unknown even if an accepted response was received
// but not durably finalized before the crash.
func (r *ExchangeTaskRepository) RecoverSubmittedUnknown(id uint, staleBefore time.Time) error {
	var task models.ExchangeTask
	if err := r.db.First(&task, id).Error; err != nil {
		return err
	}
	return r.FinalizeStaleSubmission(&task, staleBefore, "")
}

func (r *ExchangeTaskRepository) FinalizeStaleSubmission(task *models.ExchangeTask, staleBefore time.Time, operationID string) error {
	settlement := ExchangeSettlement{
		OperationID: operationID,
		Outcome:     models.ExchangeOutcomeUnknown,
		Status:      models.ExchangeTaskFailed,
		Message:     "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换",
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return finalizeExchangeTx(tx, task, settlement,
			"id = ? AND user_id = ? AND status = ? AND outcome = ? AND execution_token = ? AND updated_at < ?",
			[]interface{}{task.ID, task.UserID, string(models.ExchangeTaskRunning), string(models.ExchangeOutcomeUnknown), task.ExecutionToken, staleBefore})
	})
}

// SettleOwnedUnknown finalizes a submitted attempt when its caller was
// canceled or an Operation was redelivered. It never invokes the upstream.
func (r *ExchangeTaskRepository) SettleOwnedUnknown(id uint, executionToken string) error {
	var task models.ExchangeTask
	if err := r.db.First(&task, id).Error; err != nil {
		return err
	}
	settlement := ExchangeSettlement{
		Outcome: models.ExchangeOutcomeUnknown,
		Status:  models.ExchangeTaskFailed,
		Message: "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换",
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		return finalizeExchangeTx(tx, &task, settlement,
			"id = ? AND status = ? AND execution_token = ? AND outcome = ?",
			[]interface{}{id, string(models.ExchangeTaskRunning), executionToken, string(models.ExchangeOutcomeUnknown)})
	})
}
