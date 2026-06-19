package repository

import (
	"caiyun/internal/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
)

// ExchangeTaskRepository 抢兑任务数据访问层
type ExchangeTaskRepository struct {
	db *gorm.DB
}

func NewExchangeTaskRepository(db *gorm.DB) *ExchangeTaskRepository {
	return &ExchangeTaskRepository{db: db}
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
	return r.db.Create(task).Error
}

// Update 更新抢兑任务
func (r *ExchangeTaskRepository) Update(task *models.ExchangeTask) error {
	return r.db.Save(task).Error
}

// Delete 删除抢兑任务
func (r *ExchangeTaskRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeTask{}, id).Error
}

// GetByID 根据 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetByID(id uint) (*models.ExchangeTask, error) {
	var task models.ExchangeTask
	err := r.db.Preload("ExchangeAccount").
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
	var tasks []*models.ExchangeTask
	err := r.db.Where("user_id = ?", userID).
		Preload("ExchangeAccount").
		Preload("Product").
		Order("status ASC, created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// GetAll 获取所有抢兑任务（管理员用）
func (r *ExchangeTaskRepository) GetAll() ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.
		Preload("ExchangeAccount").
		Preload("Product").
		Order("status ASC, created_at DESC").
		Find(&tasks).Error
	return tasks, err
}

// GetByExchangeAccountID 根据兑换账号 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetByExchangeAccountID(accountID uint) ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("exchange_account_id = ?", accountID).
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

// TryMarkRunning 以条件更新方式抢占任务执行权。
// 多 Worker/多副本同时拿到同一任务时，只有一个实例能从 pending 更新为 running。
func (r *ExchangeTaskRepository) TryMarkRunning(id uint) (bool, error) {
	result := r.db.Model(&models.ExchangeTask{}).
		Where("id = ? AND status = ?", id, string(models.ExchangeTaskPending)).
		Update("status", string(models.ExchangeTaskRunning))
	if result.Error != nil {
		return false, result.Error
	}
	return result.RowsAffected > 0, nil
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

// CheckTaskExists 检查任务是否存在 (同一账号同一商品)
func (r *ExchangeTaskRepository) CheckTaskExists(userID uint, accountID uint, prizeID string) bool {
	var count int64
	r.db.Model(&models.ExchangeTask{}).
		Where("user_id = ? AND exchange_account_id = ? AND prize_id = ? AND status IN ?",
			userID, accountID, prizeID, []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
		Count(&count)
	return count > 0
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
		query = query.Where("exchange_account_id = ?", accountID)
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

// GetTasksByTime 根据时间获取待执行的抢兑任务
// 参数：
//   - hour: 小时（0-23）
//   - minute: 分钟（0-59）
//
// 返回：该时间段需要执行的抢兑任务列表
func (r *ExchangeTaskRepository) GetTasksByTime(hour, minute int) ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask

	// 格式化时间为字符串（例如：10:00）
	timeStr := fmt.Sprintf("%02d:%02d:00", hour, minute)

	// 查询条件：
	// 1. 状态为待执行或运行中
	// 2. 关联的兑换账号在指定时间有抢兑任务
	// 3. 账号处于启用状态
	// 4. 任务未删除
	err := r.db.Joins("JOIN exchange_accounts ON exchange_accounts.id = exchange_tasks.exchange_account_id").
		Joins("JOIN accounts ON accounts.id = exchange_accounts.account_id").
		Where("exchange_tasks.status IN ?", []string{string(models.ExchangeTaskPending), string(models.ExchangeTaskRunning)}).
		Where("(exchange_accounts.exchange_time_1 = ? OR exchange_accounts.exchange_time_2 = ?)", timeStr, timeStr).
		Where("exchange_accounts.is_active = ?", true).
		Where("accounts.is_active = ?", true).
		Where("accounts.auth <> ''").
		Where("exchange_accounts.deleted_at IS NULL AND accounts.deleted_at IS NULL").
		Where("exchange_tasks.deleted_at IS NULL").
		Preload("ExchangeAccount").
		Preload("ExchangeAccount.Account").
		Preload("Product").
		Order("exchange_tasks.priority DESC, exchange_tasks.created_at ASC").
		Find(&tasks).Error

	return tasks, err
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
