package repository

import (
	"caiyun/internal/models"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ErrExchangeTaskExecutionLost = errors.New("exchange task execution ownership lost")

// ExchangeTaskRepository 抢兑任务数据访问层
type ExchangeTaskRepository struct {
	db *gorm.DB
}

// ExchangeTaskFilter 描述抢兑任务列表筛选条件。
type ExchangeTaskFilter struct {
	AccountKeyword string
	Remark         string
	Status         string
	RestockCycle   string
	MinCloud       *int
	MaxCloud       *int
	OnlyActive     *bool
}

func NewExchangeTaskRepository(db *gorm.DB) *ExchangeTaskRepository {
	return &ExchangeTaskRepository{db: db}
}

// CalculateNextRun 使用真实节假日表计算任务下一次预计触发时间。
func (r *ExchangeTaskRepository) CalculateNextRun(task *models.ExchangeTask, from time.Time) *time.Time {
	return CalculateExchangeTaskNextRunWithCalendar(task, from, r.lookupCalendarHoliday)
}

func (r *ExchangeTaskRepository) lookupCalendarHoliday(now time.Time) (bool, bool) {
	if r == nil || r.db == nil {
		return false, false
	}
	var row struct {
		DayType string `gorm:"column:day_type"`
	}
	result := r.db.Table("calendar_dates").
		Select("day_type").
		Where("date = ?", now.Format("2006-01-02")).
		Limit(1).
		Scan(&row)
	if result.Error != nil || result.RowsAffected == 0 {
		return false, false
	}
	switch strings.ToLower(strings.TrimSpace(row.DayType)) {
	case "holiday", "off", "rest", "节假日", "休息日":
		return true, true
	case "workday", "working_day", "work", "调休工作日", "工作日":
		return false, true
	default:
		return false, false
	}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *ExchangeTaskRepository) WithContext(ctx context.Context) *ExchangeTaskRepository {
	if ctx == nil {
		return r
	}
	return &ExchangeTaskRepository{db: r.db.WithContext(ctx)}
}

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

// GetByID 根据 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetByID(id uint) (*models.ExchangeTask, error) {
	var task models.ExchangeTask
	err := r.db.Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Preload("Records").
		First(&task, id).Error
	if err != nil {
		return nil, err
	}
	return &task, nil
}

// GetByUserID 根据用户 ID 获取所有抢兑任务
func (r *ExchangeTaskRepository) GetByUserID(userID uint) ([]*models.ExchangeTask, error) {
	return r.GetByUserIDWithFilter(userID, ExchangeTaskFilter{})
}

// GetByUserIDWithFilter 根据用户 ID 和筛选条件获取抢兑任务。
func (r *ExchangeTaskRepository) GetByUserIDWithFilter(userID uint, filter ExchangeTaskFilter) ([]*models.ExchangeTask, error) {
	query := r.db.Model(&models.ExchangeTask{}).Where("exchange_tasks.user_id = ?", userID)
	query = applyExchangeTaskFilter(query, filter)
	var tasks []*models.ExchangeTask
	err := query.
		Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Order("exchange_tasks.status ASC, exchange_tasks.created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// GetAll 获取所有抢兑任务（管理员用）
func (r *ExchangeTaskRepository) GetAll() ([]*models.ExchangeTask, error) {
	return r.GetAllWithFilter(ExchangeTaskFilter{})
}

// GetAllWithFilter 获取所有抢兑任务（管理员用，支持筛选）。
func (r *ExchangeTaskRepository) GetAllWithFilter(filter ExchangeTaskFilter) ([]*models.ExchangeTask, error) {
	query := applyExchangeTaskFilter(r.db.Model(&models.ExchangeTask{}), filter)
	var tasks []*models.ExchangeTask
	err := query.
		Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Order("exchange_tasks.status ASC, exchange_tasks.created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

func applyExchangeTaskFilter(query *gorm.DB, filter ExchangeTaskFilter) *gorm.DB {
	joinedRules := false
	joinedAccounts := false
	joinRules := func() {
		if !joinedRules {
			query = query.Joins("LEFT JOIN exchange_rules filter_exchange_rules ON filter_exchange_rules.id = exchange_tasks.exchange_rule_id")
			joinedRules = true
		}
	}
	joinAccounts := func() {
		joinRules()
		if !joinedAccounts {
			query = query.Joins("LEFT JOIN accounts filter_accounts ON filter_accounts.id = filter_exchange_rules.account_id")
			joinedAccounts = true
		}
	}

	if filter.Status != "" {
		query = query.Where("exchange_tasks.status = ?", filter.Status)
	}
	if filter.RestockCycle != "" {
		query = query.Where("exchange_tasks.restock_cycle = ?", filter.RestockCycle)
	}
	if filter.Remark != "" {
		joinRules()
		like := "%" + filter.Remark + "%"
		query = query.Where("filter_exchange_rules.remark LIKE ?", like)
	}
	if filter.AccountKeyword != "" {
		joinAccounts()
		like := "%" + filter.AccountKeyword + "%"
		query = query.Where("filter_exchange_rules.phone LIKE ? OR filter_exchange_rules.remark LIKE ? OR filter_accounts.phone LIKE ? OR filter_accounts.remark LIKE ?", like, like, like, like)
	}
	if filter.MinCloud != nil {
		joinAccounts()
		query = query.Where("COALESCE(filter_accounts.cloud_count, 0) >= ?", *filter.MinCloud)
	}
	if filter.MaxCloud != nil {
		joinAccounts()
		query = query.Where("COALESCE(filter_accounts.cloud_count, 0) <= ?", *filter.MaxCloud)
	}
	if filter.OnlyActive != nil {
		joinAccounts()
		query = query.Where("filter_exchange_rules.is_active = ? AND filter_accounts.is_active = ?", *filter.OnlyActive, *filter.OnlyActive)
	}
	return query
}

// GetByExchangeAccountID 根据兑换账号 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetByExchangeAccountID(accountID uint) ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("exchange_rule_id = ?", accountID).
		Preload("Product").
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// GetPendingTasks 获取待执行的抢兑任务
func (r *ExchangeTaskRepository) GetPendingTasks() ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("status = ?", models.ExchangeTaskPending).
		Preload("ExchangeAccount").
		Preload("Product").
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
}

// GetPendingTasksWithPriority 获取待执行的抢兑任务（按优先级排序）
func (r *ExchangeTaskRepository) GetPendingTasksWithPriority() ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("status = ?", models.ExchangeTaskPending).
		Preload("ExchangeAccount").
		Preload("Product").
		Order("priority DESC, task_group ASC, created_at ASC"). // 优先级降序，分组升序，时间升序
		Find(&tasks).Error
	return tasks, err
}

// GetRunningTasks 获取运行中的抢兑任务
func (r *ExchangeTaskRepository) GetRunningTasks() ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("status = ?", models.ExchangeTaskRunning).
		Preload("ExchangeAccount").
		Preload("Product").
		Order("created_at ASC").
		Find(&tasks).Error
	return tasks, err
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

// GetTasksByPrizeID 根据商品 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetTasksByPrizeID(prizeID string) ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("prize_id = ?", prizeID).
		Preload("ExchangeAccount").
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
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

// GetRecordsWithFilter 带筛选条件获取抢兑记录
func (r *ExchangeTaskRepository) GetRecordsWithFilter(userID uint, accountID uint, productName string, status string, startDate string, endDate string, page int, limit int) ([]*models.ExchangeRecord, int64, error) {
	query := r.db.Model(&models.ExchangeRecord{}).Where("user_id = ?", userID)

	if accountID > 0 {
		query = query.Where("exchange_rule_id = ?", accountID)
	}

	if productName != "" {
		query = query.Where("prize_name LIKE ?", "%"+productName+"%")
	}

	if status != "" {
		query = query.Where("status = ?", status)
	}

	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}

	if endDate != "" {
		query = query.Where("created_at <= ?", endDate+" 23:59:59")
	}

	var total int64
	query.Count(&total)

	var records []*models.ExchangeRecord
	err := query.Preload("ExchangeAccount").Preload("Product").
		Order("created_at DESC").
		Offset((page - 1) * limit).
		Limit(limit).
		Find(&records).Error

	return records, total, err
}

// GetTasksByTimeAtWithSkips 返回当前时间槽可执行任务以及因周期/日历/时间策略跳过的任务摘要。
func (r *ExchangeTaskRepository) GetTasksByTimeAtWithSkips(hour, minute int, now time.Time) ([]*models.ExchangeTask, []ExchangeTaskScheduleSkip, error) {
	var tasks []*models.ExchangeTask
	timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)

	err := r.db.Joins("JOIN exchange_rules ON exchange_rules.id = exchange_tasks.exchange_rule_id").
		Joins("JOIN accounts ON accounts.id = exchange_rules.account_id").
		Where("exchange_tasks.status IN ?", []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
		Where(`(
			(exchange_tasks.custom_cron IS NOT NULL AND exchange_tasks.custom_cron <> '')
			OR (exchange_tasks.restock_times IS NOT NULL AND exchange_tasks.restock_times <> '')
			OR (exchange_tasks.scheduled_exchange_time IS NOT NULL AND exchange_tasks.scheduled_exchange_time <> '' AND exchange_tasks.scheduled_exchange_time = ?)
			OR ((exchange_tasks.scheduled_exchange_time IS NULL OR exchange_tasks.scheduled_exchange_time = '') AND (exchange_rules.exchange_time_1 = ? OR exchange_rules.exchange_time_2 = ?))
		)`, timeStr, timeStr, timeStr).
		Where("exchange_rules.is_active = ?", true).
		Where("accounts.is_active = ?", true).
		Where("accounts.auth <> ''").
		Where("exchange_rules.deleted_at IS NULL AND accounts.deleted_at IS NULL").
		Where("exchange_tasks.deleted_at IS NULL").
		Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Order("exchange_tasks.priority DESC, exchange_tasks.created_at ASC").
		Find(&tasks).Error
	if err != nil {
		return nil, nil, err
	}

	runnable := make([]*models.ExchangeTask, 0, len(tasks))
	skipped := make([]ExchangeTaskScheduleSkip, 0)
	for _, task := range tasks {
		if ok, reason := ShouldRunExchangeTaskAtWithCalendar(task, now, r.lookupCalendarHoliday); ok {
			if task.SkipReason != "" {
				_ = r.UpdateSkipReason(task.ID, "")
				task.SkipReason = ""
			}
			runnable = append(runnable, task)
		} else {
			_ = r.UpdateSkipReason(task.ID, reason)
			task.SkipReason = reason
			skipped = append(skipped, ExchangeTaskScheduleSkip{
				TaskID:         task.ID,
				RestockCycle:   task.RestockCycle,
				CalendarPolicy: task.CalendarPolicy,
				Reason:         reason,
			})
		}
	}
	return runnable, skipped, nil
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
