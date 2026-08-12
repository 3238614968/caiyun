package repository

import (
	"caiyun/internal/models"

	"gorm.io/gorm"
)

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

// GetTasksByPrizeID 根据商品 ID 获取抢兑任务
func (r *ExchangeTaskRepository) GetTasksByPrizeID(prizeID string) ([]*models.ExchangeTask, error) {
	var tasks []*models.ExchangeTask
	err := r.db.Where("prize_id = ?", prizeID).
		Preload("ExchangeAccount").
		Order("created_at DESC").
		Find(&tasks).Error
	return tasks, err
}
