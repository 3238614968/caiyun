package repository

import (
	"caiyun/internal/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type ExchangeRecordRepository struct {
	db *gorm.DB
}

// FailureReasonStat 表示失败消息分组统计。
type FailureReasonStat struct {
	Message string
	Count   int64
}

func NewExchangeRecordRepository(db *gorm.DB) *ExchangeRecordRepository {
	return &ExchangeRecordRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *ExchangeRecordRepository) WithContext(ctx context.Context) *ExchangeRecordRepository {
	if ctx == nil {
		return r
	}
	return &ExchangeRecordRepository{db: r.db.WithContext(ctx)}
}

// Create 创建抢兑记录
func (r *ExchangeRecordRepository) Create(record *models.ExchangeRecord) error {
	return r.db.Create(record).Error
}

// FindByID 根据 ID 获取记录
func (r *ExchangeRecordRepository) FindByID(id uint) (*models.ExchangeRecord, error) {
	var record models.ExchangeRecord
	err := r.db.First(&record, id).Error
	if err != nil {
		return nil, err
	}
	return &record, nil
}

// FindByUserID 根据用户 ID 获取记录
func (r *ExchangeRecordRepository) FindByUserID(userID uint, offset, limit int) ([]*models.ExchangeRecord, int64, error) {
	var records []*models.ExchangeRecord
	var total int64

	if err := r.db.Model(&models.ExchangeRecord{}).Where("user_id = ?", userID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Where("user_id = ?", userID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&records).Error
	return records, total, err
}

// FindByTaskID 根据任务 ID 获取记录
func (r *ExchangeRecordRepository) FindByTaskID(taskID uint) ([]*models.ExchangeRecord, error) {
	var records []*models.ExchangeRecord
	err := r.db.Where("exchange_task_id = ?", taskID).
		Order("created_at DESC").
		Find(&records).Error
	return records, err
}

// FindByAccountID 根据兑换账号 ID 获取记录
func (r *ExchangeRecordRepository) FindByAccountID(accountID uint, offset, limit int) ([]*models.ExchangeRecord, int64, error) {
	var records []*models.ExchangeRecord
	var total int64

	if err := r.db.Model(&models.ExchangeRecord{}).Where("exchange_rule_id = ?", accountID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Where("exchange_rule_id = ?", accountID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&records).Error
	return records, total, err
}

// FindByAccountInPeriod 获取同一兑换账号在指定时间段内的抢兑记录。
func (r *ExchangeRecordRepository) FindByAccountInPeriod(userID uint, exchangeAccountID uint, startTime, endTime time.Time) ([]*models.ExchangeRecord, error) {
	var records []*models.ExchangeRecord
	err := r.db.Where("user_id = ? AND exchange_rule_id = ? AND created_at >= ? AND created_at < ?",
		userID, exchangeAccountID, startTime, endTime).
		Preload("Product").
		Order("created_at DESC").
		Find(&records).Error
	return records, err
}

// GetStats 获取统计数据
func (r *ExchangeRecordRepository) GetStats(userID uint, startTime, endTime time.Time) (successCount, failCount int64, err error) {
	base := func() *gorm.DB {
		query := r.db.Model(&models.ExchangeRecord{})
		if userID > 0 {
			query = query.Where("user_id = ?", userID)
		}
		if !startTime.IsZero() {
			query = query.Where("created_at >= ?", startTime)
		}
		if !endTime.IsZero() {
			query = query.Where("created_at <= ?", endTime)
		}
		return query
	}

	if err = base().Where("status = ?", "success").Count(&successCount).Error; err != nil {
		return 0, 0, err
	}
	if err = base().Where("status = ?", "failed").Count(&failCount).Error; err != nil {
		return 0, 0, err
	}

	return successCount, failCount, nil
}

// GetFailureReasonStats 按失败消息聚合统计，userID 为 0 时统计全局。
func (r *ExchangeRecordRepository) GetFailureReasonStats(userID uint, startTime, endTime time.Time, limit int) ([]FailureReasonStat, error) {
	if limit <= 0 || limit > 20 {
		limit = 10
	}
	query := r.db.Model(&models.ExchangeRecord{}).
		Select("message, COUNT(*) as count").
		Where("status = ?", "failed")
	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	var stats []FailureReasonStat
	err := query.Group("message").Order("count DESC").Limit(limit).Scan(&stats).Error
	return stats, err
}

// Delete 删除记录
func (r *ExchangeRecordRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeRecord{}, id).Error
}

// BatchCreate 批量创建记录
func (r *ExchangeRecordRepository) BatchCreate(records []*models.ExchangeRecord) error {
	return r.db.CreateInBatches(records, 100).Error
}
