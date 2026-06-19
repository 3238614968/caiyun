package repository

import (
	"caiyun/internal/models"
	"context"
	"database/sql"
	"time"

	"gorm.io/gorm"
)

type TaskLogRepository struct {
	db *gorm.DB
}

// AccountLogSummary 汇总指定账号在今日/昨日的任务统计。
type AccountLogSummary struct {
	AccountID       uint
	TodayGained     int
	YesterdayGained int
	SuccessCount    int64
	FailedCount     int64
	LastExecutedAt  sql.NullTime
}

// 北京时间时区
var cstZone = time.FixedZone("CST", 8*3600)

// todayStart 获取北京时间今天0点
func todayStart() time.Time {
	now := time.Now().In(cstZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
}

func NewTaskLogRepository(db *gorm.DB) *TaskLogRepository {
	return &TaskLogRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *TaskLogRepository) WithContext(ctx context.Context) *TaskLogRepository {
	if ctx == nil {
		return r
	}
	return &TaskLogRepository{db: r.db.WithContext(ctx)}
}

// Create 创建任务日志
func (r *TaskLogRepository) Create(log *models.TaskLog) error {
	return r.db.Create(log).Error
}

// FindByID 根据ID查找任务日志
func (r *TaskLogRepository) FindByID(id uint) (*models.TaskLog, error) {
	var log models.TaskLog
	err := r.db.Preload("Account").First(&log, id).Error
	if err != nil {
		return nil, err
	}
	return &log, nil
}

// FindByAccountID 根据账号ID查找任务日志
func (r *TaskLogRepository) FindByAccountID(accountID uint, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).Where("account_id = ?", accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// FindByUserID 根据用户ID查找任务日志
func (r *TaskLogRepository) FindByUserID(userID uint, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Account").Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// FindByTaskType 根据任务类型查找日志
func (r *TaskLogRepository) FindByTaskType(taskType string, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).Where("task_type = ?", taskType)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// FindByStatus 根据状态查找日志
func (r *TaskLogRepository) FindByStatus(status string, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).Where("status = ?", status)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// FindByDateRange 根据日期范围查找日志
func (r *TaskLogRepository) FindByDateRange(startDate, endDate time.Time, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).Where("created_at BETWEEN ? AND ?", startDate, endDate)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// FindByAccountIDAndDateRange 根据账号ID和日期范围查找日志
func (r *TaskLogRepository) FindByAccountIDAndDateRange(accountID uint, startDate, endDate time.Time, offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	query := r.db.Model(&models.TaskLog{}).
		Where("account_id = ? AND created_at BETWEEN ? AND ?", accountID, startDate, endDate)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// GetTotalCloudGainedByAccountID 获取账号获得的总云朵数
func (r *TaskLogRepository) GetTotalCloudGainedByAccountID(accountID uint) (int, error) {
	var total int
	err := r.db.Model(&models.TaskLog{}).
		Where("account_id = ?", accountID).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total).Error
	return total, err
}

// GetTotalCloudGainedByUserID 获取用户获得的总云朵数
func (r *TaskLogRepository) GetTotalCloudGainedByUserID(userID uint) (int, error) {
	var total int
	err := r.db.Model(&models.TaskLog{}).
		Where("user_id = ?", userID).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total).Error
	return total, err
}

// GetTodayCloudGainedByUserID 获取用户今日获得的云朵数
func (r *TaskLogRepository) GetTodayCloudGainedByUserID(userID uint) (int, error) {
	var total int
	today := todayStart()
	tomorrow := today.Add(24 * time.Hour)

	err := r.db.Model(&models.TaskLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, today, tomorrow).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total).Error
	return total, err
}

// GetSuccessRate 获取任务成功率
func (r *TaskLogRepository) GetSuccessRate(userID uint) (float64, error) {
	var successCount, totalCount int64

	r.db.Model(&models.TaskLog{}).Where("user_id = ?", userID).Count(&totalCount)
	r.db.Model(&models.TaskLog{}).Where("user_id = ? AND status = ?", userID, "success").Count(&successCount)

	if totalCount == 0 {
		return 0, nil
	}

	return float64(successCount) / float64(totalCount) * 100, nil
}

// Delete 删除任务日志
func (r *TaskLogRepository) Delete(id uint) error {
	return r.db.Delete(&models.TaskLog{}, id).Error
}

// DeleteByAccountID 删除指定账号的所有日志
func (r *TaskLogRepository) DeleteByAccountID(accountID uint) error {
	return r.db.Where("account_id = ?", accountID).Delete(&models.TaskLog{}).Error
}

// List 列出所有任务日志
func (r *TaskLogRepository) List(offset, limit int) ([]*models.TaskLog, int64, error) {
	var logs []*models.TaskLog
	var total int64

	if err := r.db.Model(&models.TaskLog{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Preload("Account").Order("created_at DESC").Offset(offset).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// UpdateCloudGained 更新指定任务日志的云朵获得数（最近一条）
func (r *TaskLogRepository) UpdateCloudGained(userID, accountID uint, taskType string, cloudGained int) error {
	today := todayStart()
	tomorrow := today.Add(24 * time.Hour)

	return r.db.Model(&models.TaskLog{}).
		Where("user_id = ? AND account_id = ? AND task_type = ? AND created_at >= ? AND created_at < ?",
			userID, accountID, taskType, today, tomorrow).
		Order("created_at DESC").
		Limit(1).
		Update("cloud_gained", cloudGained).Error
}

// CountByAccountIDAndDateRange 统计指定账号在日期范围内的任务日志数量
func (r *TaskLogRepository) CountByAccountIDAndDateRange(accountID uint, startDate, endDate time.Time, count *int64) {
	r.db.Model(&models.TaskLog{}).
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, startDate, endDate).
		Count(count)
}

// CountByAccountIDTaskTypesAndDateRange 统计指定账号在日期范围内指定任务类型的日志数量。
func (r *TaskLogRepository) CountByAccountIDTaskTypesAndDateRange(accountID uint, taskTypes []string, startDate, endDate time.Time, count *int64) {
	if len(taskTypes) == 0 {
		*count = 0
		return
	}
	r.db.Model(&models.TaskLog{}).
		Where("account_id = ? AND task_type IN ? AND created_at >= ? AND created_at < ?", accountID, taskTypes, startDate, endDate).
		Count(count)
}

// GetCloudGainedByAccountAndRange 获取指定账号在日期范围内获得的云朵数
func (r *TaskLogRepository) GetCloudGainedByAccountAndRange(accountID uint, start, end time.Time) int {
	var total int
	r.db.Model(&models.TaskLog{}).
		Where("account_id = ? AND created_at >= ? AND created_at < ?", accountID, start, end).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total)
	return total
}

// GetAccountSummariesByIDs 批量聚合账号任务统计，避免管理员账号概况页按账号逐条查询。
func (r *TaskLogRepository) GetAccountSummariesByIDs(ids []uint, today, tomorrow, yesterday time.Time) (map[uint]AccountLogSummary, error) {
	result := make(map[uint]AccountLogSummary, len(ids))
	if len(ids) == 0 {
		return result, nil
	}

	var rows []AccountLogSummary
	err := r.db.Model(&models.TaskLog{}).
		Select(`
			account_id,
			COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? THEN cloud_gained ELSE 0 END), 0) AS today_gained,
			COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? THEN cloud_gained ELSE 0 END), 0) AS yesterday_gained,
			COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? AND status = ? THEN 1 ELSE 0 END), 0) AS success_count,
			COALESCE(SUM(CASE WHEN created_at >= ? AND created_at < ? AND status = ? THEN 1 ELSE 0 END), 0) AS failed_count,
			MAX(created_at) AS last_executed_at`,
			today, tomorrow,
			yesterday, today,
			today, tomorrow, "success",
			today, tomorrow, "failed",
		).
		Where("account_id IN ?", ids).
		Group("account_id").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	for _, row := range rows {
		result[row.AccountID] = row
	}
	return result, nil
}

// GetCloudGainedByUserAndRange 获取指定用户在日期范围内获得的云朵数
func (r *TaskLogRepository) GetCloudGainedByUserAndRange(userID uint, start, end time.Time) int {
	var total int
	r.db.Model(&models.TaskLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total)
	return total
}

// CountByAccountStatusAndRange 统计指定账号在日期范围内指定状态的日志数量
func (r *TaskLogRepository) CountByAccountStatusAndRange(accountID uint, status string, start, end time.Time) int64 {
	var count int64
	r.db.Model(&models.TaskLog{}).
		Where("account_id = ? AND status = ? AND created_at >= ? AND created_at < ?", accountID, status, start, end).
		Count(&count)
	return count
}

// CountByUserStatusAndRange 统计指定用户在日期范围内指定状态的日志数量（空status表示全部）
func (r *TaskLogRepository) CountByUserStatusAndRange(userID uint, status string, start, end time.Time) int64 {
	var count int64
	q := r.db.Model(&models.TaskLog{}).Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q.Count(&count)
	return count
}

// FindLastByAccountID 获取指定账号最近一条日志
func (r *TaskLogRepository) FindLastByAccountID(accountID uint) *models.TaskLog {
	var log models.TaskLog
	err := r.db.Where("account_id = ?", accountID).Order("created_at DESC").First(&log).Error
	if err != nil {
		return nil
	}
	return &log
}

// GetCloudGainedGlobal 获取全局在日期范围内获得的云朵数
func (r *TaskLogRepository) GetCloudGainedGlobal(start, end time.Time) int {
	var total int
	r.db.Model(&models.TaskLog{}).
		Where("created_at >= ? AND created_at < ?", start, end).
		Select("COALESCE(SUM(cloud_gained), 0)").
		Scan(&total)
	return total
}

// CountByStatusAndRange 统计日期范围内指定状态的日志数量（空status表示全部）
func (r *TaskLogRepository) CountByStatusAndRange(status string, start, end time.Time) int64 {
	var count int64
	q := r.db.Model(&models.TaskLog{}).Where("created_at >= ? AND created_at < ?", start, end)
	if status != "" {
		q = q.Where("status = ?", status)
	}
	q.Count(&count)
	return count
}
