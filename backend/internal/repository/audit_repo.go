package repository

import (
	"caiyun/internal/models"
	"context"
	"time"

	"gorm.io/gorm"
)

// AuditLogRepository 审计日志仓库
type AuditLogRepository struct {
	db *gorm.DB
}

// NewAuditLogRepository 创建审计日志仓库
func NewAuditLogRepository(db *gorm.DB) *AuditLogRepository {
	return &AuditLogRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *AuditLogRepository) WithContext(ctx context.Context) *AuditLogRepository {
	if ctx == nil {
		return r
	}
	return &AuditLogRepository{db: r.db.WithContext(ctx)}
}

// Create 创建审计日志
func (r *AuditLogRepository) Create(log *models.AuditLog) error {
	return r.db.Create(log).Error
}

// GetByID 根据ID获取审计日志
func (r *AuditLogRepository) GetByID(id uint) (*models.AuditLog, error) {
	var log models.AuditLog
	err := r.db.First(&log, id).Error
	return &log, err
}

// List 获取审计日志列表
func (r *AuditLogRepository) List(userID uint, action string, resource string, startTime, endTime time.Time, page, limit int) ([]*models.AuditLog, int64, error) {
	var logs []*models.AuditLog
	var total int64

	query := r.db.Model(&models.AuditLog{})

	if userID > 0 {
		query = query.Where("user_id = ?", userID)
	}
	if action != "" {
		query = query.Where("action = ?", action)
	}
	if resource != "" {
		query = query.Where("resource = ?", resource)
	}
	if !startTime.IsZero() {
		query = query.Where("created_at >= ?", startTime)
	}
	if !endTime.IsZero() {
		query = query.Where("created_at <= ?", endTime)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("created_at DESC").Offset((page - 1) * limit).Limit(limit).Find(&logs).Error
	return logs, total, err
}

// GetUserActivityStats 获取用户活动统计
func (r *AuditLogRepository) GetUserActivityStats(userID uint, startTime, endTime time.Time) (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	// 总操作次数
	var totalCount int64
	err := r.db.Model(&models.AuditLog{}).
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userID, startTime, endTime).
		Count(&totalCount).Error
	if err != nil {
		return nil, err
	}
	stats["total_count"] = totalCount

	// 按操作类型统计
	var actionStats []struct {
		Action string
		Count  int64
	}
	err = r.db.Model(&models.AuditLog{}).
		Select("action, COUNT(*) as count").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userID, startTime, endTime).
		Group("action").
		Scan(&actionStats).Error
	if err != nil {
		return nil, err
	}
	stats["action_stats"] = actionStats

	// 按资源类型统计
	var resourceStats []struct {
		Resource string
		Count    int64
	}
	err = r.db.Model(&models.AuditLog{}).
		Select("resource, COUNT(*) as count").
		Where("user_id = ? AND created_at >= ? AND created_at <= ?", userID, startTime, endTime).
		Group("resource").
		Scan(&resourceStats).Error
	if err != nil {
		return nil, err
	}
	stats["resource_stats"] = resourceStats

	return stats, nil
}

// DeleteOldLogs 删除旧日志（清理策略）
func (r *AuditLogRepository) DeleteOldLogs(before time.Time) error {
	return r.db.Where("created_at < ?", before).Delete(&models.AuditLog{}).Error
}

// GetRecentLogs 获取最近的审计日志
func (r *AuditLogRepository) GetRecentLogs(limit int) ([]*models.AuditLog, error) {
	var logs []*models.AuditLog
	err := r.db.Order("created_at DESC").Limit(limit).Find(&logs).Error
	return logs, err
}
