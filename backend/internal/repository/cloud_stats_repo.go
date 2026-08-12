package repository

import (
	"caiyun/internal/models"
	"context"
	"fmt"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CloudStatsRepository struct {
	db *gorm.DB
}

const (
	defaultCloudStatsPageLimit = 200
	maxCloudStatsPageLimit     = 1000
)

func normalizeCloudStatsPage(offset, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > maxCloudStatsPageLimit {
		limit = defaultCloudStatsPageLimit
	}
	return offset, limit
}

// normalizeCloudStatsDate keeps the API contract for DATE columns stable when
// the SQL driver returns an RFC3339 value (as SQLite does for GORM's date type).
func normalizeCloudStatsDate(stats *models.CloudStats) {
	if stats == nil || len(stats.Date) == len("2006-01-02") {
		return
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02 15:04:05"} {
		if date, err := time.Parse(layout, stats.Date); err == nil {
			stats.Date = date.Format("2006-01-02")
			return
		}
	}
}

func normalizeCloudStatsDates(stats []*models.CloudStats) {
	for _, stat := range stats {
		normalizeCloudStatsDate(stat)
	}
}

func NewCloudStatsRepository(db *gorm.DB) *CloudStatsRepository {
	return &CloudStatsRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *CloudStatsRepository) WithContext(ctx context.Context) *CloudStatsRepository {
	if ctx == nil {
		return r
	}
	return &CloudStatsRepository{db: r.db.WithContext(ctx)}
}

// Create 创建云朵统计记录
func (r *CloudStatsRepository) Create(stats *models.CloudStats) error {
	return r.db.Create(stats).Error
}

// FindByID 根据ID查找统计记录
func (r *CloudStatsRepository) FindByID(id uint) (*models.CloudStats, error) {
	var stats models.CloudStats
	err := r.db.Preload("Account").First(&stats, id).Error
	if err != nil {
		return nil, err
	}
	normalizeCloudStatsDate(&stats)
	return &stats, nil
}

// FindByUserID 根据用户ID查找统计记录
func (r *CloudStatsRepository) FindByUserID(userID uint, offset, limit int) ([]*models.CloudStats, int64, error) {
	var stats []*models.CloudStats
	var total int64

	query := r.db.Model(&models.CloudStats{}).Where("user_id = ?", userID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Account").Order("date DESC").Offset(offset).Limit(limit).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, total, err
}

// FindByAccountID 根据账号ID查找统计记录
func (r *CloudStatsRepository) FindByAccountID(accountID uint, offset, limit int) ([]*models.CloudStats, int64, error) {
	var stats []*models.CloudStats
	var total int64

	query := r.db.Model(&models.CloudStats{}).Where("account_id = ?", accountID)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Order("date DESC").Offset(offset).Limit(limit).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, total, err
}

// FindByDate 根据日期查找统计记录
func (r *CloudStatsRepository) FindByDate(date string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("date = ?", date).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// FindByUserIDAndDate 根据用户ID和日期查找统计记录
func (r *CloudStatsRepository) FindByUserIDAndDate(userID uint, date string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, date).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// FindByAccountIDAndDate 根据账号ID和日期查找统计记录
func (r *CloudStatsRepository) FindByAccountIDAndDate(accountID uint, date string) (*models.CloudStats, error) {
	var stats models.CloudStats
	err := r.db.Where("account_id = ? AND date = ?", accountID, date).First(&stats).Error
	if err != nil {
		return nil, err
	}
	normalizeCloudStatsDate(&stats)
	return &stats, nil
}

// FindByDateRange 根据日期范围分页查找统计记录。
func (r *CloudStatsRepository) FindByDateRange(startDate, endDate string, offset, limit int) ([]*models.CloudStats, int64, error) {
	var stats []*models.CloudStats
	var total int64
	offset, limit = normalizeCloudStatsPage(offset, limit)

	query := r.db.Model(&models.CloudStats{}).Where("date BETWEEN ? AND ?", startDate, endDate)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	err := query.Preload("Account").Order("date DESC").Offset(offset).Limit(limit).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, total, err
}

// FindByDateRangeAll 根据日期范围加载全部统计记录；调用方应自行控制日期跨度。
func (r *CloudStatsRepository) FindByDateRangeAll(startDate, endDate string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("date BETWEEN ? AND ?", startDate, endDate).Order("date DESC").Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// FindByUserIDAndDateRange 根据用户ID和日期范围查找统计记录
func (r *CloudStatsRepository) FindByUserIDAndDateRange(userID uint, startDate, endDate string, offset, limit int) ([]*models.CloudStats, int64, error) {
	var stats []*models.CloudStats
	var total int64

	query := r.db.Model(&models.CloudStats{}).
		Where("user_id = ? AND date BETWEEN ? AND ?", userID, startDate, endDate)
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := query.Preload("Account").Order("date DESC").Offset(offset).Limit(limit).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, total, err
}

// Update 更新统计记录
func (r *CloudStatsRepository) Update(stats *models.CloudStats) error {
	return r.db.Save(stats).Error
}

// Delete 删除统计记录
func (r *CloudStatsRepository) Delete(id uint) error {
	return r.db.Delete(&models.CloudStats{}, id).Error
}

// UpsertByAccountIDAndDate 插入或更新账号某日的统计数据
func (r *CloudStatsRepository) UpsertByAccountIDAndDate(stats *models.CloudStats) error {
	if stats == nil {
		return fmt.Errorf("cloud stats is nil")
	}
	normalizeCloudStatsDate(stats)
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "account_id"}, {Name: "date"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"user_id", "cloud_count", "cloud_diff", "cloud_diff_week", "updated_at", "deleted_at",
		}),
	}).Create(stats).Error
}

// GetTodayStatsByUserID 获取用户今日的所有统计记录
func (r *CloudStatsRepository) GetTodayStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	today := nowCST().Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, today).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// GetYesterdayStatsByUserID 获取用户昨日的统计记录
func (r *CloudStatsRepository) GetYesterdayStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	yesterday := nowCST().AddDate(0, 0, -1).Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, yesterday).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// GetLastWeekStatsByUserID 获取用户上周同期的统计记录
func (r *CloudStatsRepository) GetLastWeekStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	lastWeek := nowCST().AddDate(0, 0, -7).Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, lastWeek).Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// GetTotalCloudCountByUserID 获取用户所有账号的总云朵数（最近一天）
func (r *CloudStatsRepository) GetTotalCloudCountByUserID(userID uint) (int, error) {
	var total int
	latestDate := r.db.Model(&models.CloudStats{}).
		Select("MAX(date)").
		Where("user_id = ?", userID)
	err := r.db.Model(&models.CloudStats{}).
		Where("user_id = ? AND date = (?)", userID, latestDate).
		Select("COALESCE(SUM(cloud_count), 0)").
		Scan(&total).Error
	return total, err
}

// GetTrendDataByUserID 获取用户最近N天的趋势数据
func (r *CloudStatsRepository) GetTrendDataByUserID(userID uint, days int) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	now := nowCST()
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days+1).Format("2006-01-02")

	err := r.db.Model(&models.CloudStats{}).
		Where("user_id = ? AND date BETWEEN ? AND ?", userID, startDate, endDate).
		Group("date").
		// Trend points intentionally contain only grouped/aggregated fields.
		// Timestamps from individual account snapshots are not meaningful here
		// and selecting them violates MySQL ONLY_FULL_GROUP_BY.
		Select("date, SUM(cloud_count) as cloud_count, 0 as cloud_diff, 0 as cloud_diff_week").
		Order("date ASC").
		Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// GetTrendDataGlobal 获取全局最近N天的趋势数据
func (r *CloudStatsRepository) GetTrendDataGlobal(days int) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	now := nowCST()
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days+1).Format("2006-01-02")

	err := r.db.Model(&models.CloudStats{}).
		Where("date BETWEEN ? AND ?", startDate, endDate).
		Group("date").
		Select("date, SUM(cloud_count) as cloud_count, 0 as cloud_diff, 0 as cloud_diff_week").
		Order("date ASC").
		Find(&stats).Error
	normalizeCloudStatsDates(stats)
	return stats, err
}

// GetDashboardStats 获取仪表盘统计数据
func (r *CloudStatsRepository) GetDashboardStats(userID uint) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// 获取今日统计数据
	today := nowCST().Format("2006-01-02")
	var todayStats []*models.CloudStats
	err := r.db.Where("user_id = ? AND date = ?", userID, today).Find(&todayStats).Error
	if err != nil {
		return nil, err
	}

	totalClouds := 0
	activeAccounts := 0
	todayChange := 0

	for _, stat := range todayStats {
		totalClouds += stat.CloudCount
		if stat.CloudCount > 0 {
			activeAccounts++
		}
		todayChange += stat.CloudDiff
	}

	result["total_clouds"] = totalClouds
	result["active_accounts"] = activeAccounts
	result["today_change"] = todayChange

	// 获取本周趋势数据
	weekStats, err := r.GetTrendDataByUserID(userID, 7)
	if err != nil {
		return nil, err
	}

	// 计算周增长率
	weekGrowth := 0.0
	if len(weekStats) >= 2 {
		firstDay := weekStats[0].CloudCount
		lastDay := weekStats[len(weekStats)-1].CloudCount
		if firstDay > 0 {
			weekGrowth = (float64(lastDay) - float64(firstDay)) / float64(firstDay) * 100
		}
	}

	result["week_growth"] = weekGrowth
	result["week_trend"] = weekStats

	// 获取账号排名
	var accountRankings []struct {
		AccountID  uint `json:"account_id"`
		CloudCount int  `json:"cloud_count"`
	}
	err = r.db.Model(&models.CloudStats{}).
		Where("user_id = ? AND date = ?", userID, today).
		Select("account_id, cloud_count").
		Order("cloud_count DESC").
		Limit(5).
		Scan(&accountRankings).Error
	if err != nil {
		return nil, err
	}

	result["account_rankings"] = accountRankings

	return result, nil
}

// CalculateDailyDiff 计算对比昨日的变化
func (r *CloudStatsRepository) CalculateDailyDiff(userID uint) error {
	// 获取昨日的数据
	now := nowCST()
	yesterdayDate := now.AddDate(0, 0, -1).Format("2006-01-02")
	yesterdayStats, err := r.FindByUserIDAndDate(userID, yesterdayDate)
	if err != nil {
		return fmt.Errorf("load yesterday cloud stats: %w", err)
	}

	// 创建昨日的云朵数量映射
	yesterdayMap := make(map[uint]int)
	for _, stat := range yesterdayStats {
		yesterdayMap[stat.AccountID] = stat.CloudCount
	}

	// 获取今日的数据
	todayDate := now.Format("2006-01-02")
	todayStats, err := r.FindByUserIDAndDate(userID, todayDate)
	if err != nil {
		return fmt.Errorf("load today cloud stats: %w", err)
	}

	// 更新今日数据的差异
	for _, stat := range todayStats {
		if yesterdayCount, ok := yesterdayMap[stat.AccountID]; ok {
			stat.CloudDiff = stat.CloudCount - yesterdayCount
		}
		if err := r.db.Save(stat).Error; err != nil {
			return fmt.Errorf("save daily cloud diff for account %d: %w", stat.AccountID, err)
		}
	}

	return nil
}

// CalculateWeeklyDiff 计算对比上周的变化
func (r *CloudStatsRepository) CalculateWeeklyDiff(userID uint) error {
	// 获取上周同期的数据
	now := nowCST()
	lastWeekDate := now.AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekStats, err := r.FindByUserIDAndDate(userID, lastWeekDate)
	if err != nil {
		return fmt.Errorf("load last-week cloud stats: %w", err)
	}

	// 创建上周的云朵数量映射
	lastWeekMap := make(map[uint]int)
	for _, stat := range lastWeekStats {
		lastWeekMap[stat.AccountID] = stat.CloudCount
	}

	// 获取今日的数据
	todayDate := now.Format("2006-01-02")
	todayStats, err := r.FindByUserIDAndDate(userID, todayDate)
	if err != nil {
		return fmt.Errorf("load today cloud stats: %w", err)
	}

	// 更新今日数据的周差异
	for _, stat := range todayStats {
		if lastWeekCount, ok := lastWeekMap[stat.AccountID]; ok {
			stat.CloudDiffWeek = stat.CloudCount - lastWeekCount
		}
		if err := r.db.Save(stat).Error; err != nil {
			return fmt.Errorf("save weekly cloud diff for account %d: %w", stat.AccountID, err)
		}
	}

	return nil
}

// List 列出所有统计记录
func (r *CloudStatsRepository) List(offset, limit int) ([]*models.CloudStats, int64, error) {
	var stats []*models.CloudStats
	var total int64

	if err := r.db.Model(&models.CloudStats{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	err := r.db.Preload("Account").Order("date DESC").Offset(offset).Limit(limit).Find(&stats).Error
	return stats, total, err
}
