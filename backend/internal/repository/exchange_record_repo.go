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

	if err := r.db.Model(&models.ExchangeRecord{}).Where("exchange_account_id = ?", accountID).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Where("exchange_account_id = ?", accountID).
		Order("created_at DESC").
		Offset(offset).Limit(limit).
		Find(&records).Error
	return records, total, err
}

// FindByAccountInPeriod 获取同一兑换账号在指定时间段内的抢兑记录。
func (r *ExchangeRecordRepository) FindByAccountInPeriod(userID uint, exchangeAccountID uint, startTime, endTime time.Time) ([]*models.ExchangeRecord, error) {
	var records []*models.ExchangeRecord
	err := r.db.Where("user_id = ? AND exchange_account_id = ? AND created_at >= ? AND created_at < ?",
		userID, exchangeAccountID, startTime, endTime).
		Preload("Product").
		Order("created_at DESC").
		Find(&records).Error
	return records, err
}

// GetStats 获取统计数据
func (r *ExchangeRecordRepository) GetStats(userID uint, startTime, endTime time.Time) (successCount, failCount int64, err error) {
	query := r.db.Model(&models.ExchangeRecord{}).Where("user_id = ?", userID)

	if !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	err = query.Where("status = ?", "success").Count(&successCount).Error
	if err != nil {
		return 0, 0, err
	}

	err = query.Where("status = ?", "failed").Count(&failCount).Error
	if err != nil {
		return 0, 0, err
	}

	return successCount, failCount, nil
}

// Delete 删除记录
func (r *ExchangeRecordRepository) Delete(id uint) error {
	return r.db.Delete(&models.ExchangeRecord{}, id).Error
}

// BatchCreate 批量创建记录
func (r *ExchangeRecordRepository) BatchCreate(records []*models.ExchangeRecord) error {
	return r.db.CreateInBatches(records, 100).Error
}
