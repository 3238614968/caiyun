package repository

import (
	"caiyun/internal/models"
	"time"
)

func (r *TaskLogRepository) SuccessfulTaskTypesInRange(accountID uint, codes []string, start, end time.Time) ([]string, error) {
	if len(codes) == 0 {
		return []string{}, nil
	}
	var done []string
	err := r.db.Model(&models.TaskLog{}).Distinct("task_type").
		Where("account_id = ? AND task_type IN ? AND status = ? AND created_at >= ? AND created_at < ?", accountID, codes, "success", start, end).
		Pluck("task_type", &done).Error
	return done, err
}
