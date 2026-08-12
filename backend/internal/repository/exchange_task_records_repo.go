package repository

import "caiyun/internal/models"

// Record queries are isolated from task definition and execution lease writes.

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
