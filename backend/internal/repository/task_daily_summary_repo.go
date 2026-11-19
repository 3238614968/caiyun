package repository

import (
	"context"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/presentation"

	"gorm.io/gorm"
)

// Day summaries sum receipts across all executions, but count only the latest
// outcome of each task. Retrying one task must not multiply the failure count.
func (r *TaskLogRepository) DailySummaries(ctx context.Context, userID uint, start, end time.Time, offset, limit int) ([]*models.TaskDailySummary, int64, error) {
	base := func() *gorm.DB {
		return r.db.WithContext(ctx).Model(&models.TaskLog{}).
			Where("user_id = ? AND created_at >= ? AND created_at < ?", userID, start, end).
			Where("task_type NOT IN ?", []string{"exchange"})
	}
	var total int64
	if err := base().Distinct("account_id").Count(&total).Error; err != nil {
		return nil, 0, err
	}
	rows := make([]*models.TaskDailySummary, 0)
	offset, limit = normalizeTaskLogPage(offset, limit)
	if err := base().Select("account_id, " + effectiveCloudGainSum + " AS cloud_gained, COALESCE(SUM(CASE WHEN migu_notes_gained > 0 THEN migu_notes_gained ELSE 0 END), 0) AS migu_notes_gained").
		Group("account_id").Order("MAX(created_at) DESC, account_id ASC").Offset(offset).Limit(limit).Scan(&rows).Error; err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return rows, total, nil
	}
	byAccount := make(map[uint]*models.TaskDailySummary, len(rows))
	ids := make([]uint, 0, len(rows))
	for _, row := range rows {
		byAccount[row.AccountID] = row
		ids = append(ids, row.AccountID)
	}
	latest := base().Select("MAX(id)").Where("account_id IN ?", ids).Group("account_id, task_type")
	var outcomes []struct {
		AccountID uint
		Status    string
		CreatedAt time.Time
	}
	if err := r.db.WithContext(ctx).Model(&models.TaskLog{}).Select("account_id, status, created_at").Where("id IN (?)", latest).Scan(&outcomes).Error; err != nil {
		return nil, 0, err
	}
	for _, outcome := range outcomes {
		row := byAccount[outcome.AccountID]
		if row == nil {
			continue
		}
		if outcome.CreatedAt.After(row.LastExecutedAt) {
			row.LastExecutedAt = outcome.CreatedAt
		}
		switch presentation.TaskLogStatus(outcome.Status) {
		case "success":
			row.SuccessCount++
		case "failed":
			row.FailedCount++
		case "skipped":
			row.SkippedCount++
		default:
			row.PendingCount++
		}
	}
	var accounts []struct {
		ID     uint
		Phone  string
		Remark string
	}
	if err := r.db.WithContext(ctx).Model(&models.Account{}).Select("id, phone, remark").
		Where("user_id = ? AND id IN ?", userID, ids).Scan(&accounts).Error; err != nil {
		return nil, 0, err
	}
	for _, account := range accounts {
		if row := byAccount[account.ID]; row != nil {
			row.Phone, row.Remark = account.Phone, account.Remark
		}
	}
	return rows, total, nil
}
