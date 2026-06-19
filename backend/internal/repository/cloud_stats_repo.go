package repository

import (
	"caiyun/internal/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type CloudStatsRepository struct {
	db *gorm.DB
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
	return stats, total, err
}

// FindByDate 根据日期查找统计记录
func (r *CloudStatsRepository) FindByDate(date string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("date = ?", date).Find(&stats).Error
	return stats, err
}

// FindByUserIDAndDate 根据用户ID和日期查找统计记录
func (r *CloudStatsRepository) FindByUserIDAndDate(userID uint, date string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, date).Find(&stats).Error
	return stats, err
}

// FindByAccountIDAndDate 根据账号ID和日期查找统计记录
func (r *CloudStatsRepository) FindByAccountIDAndDate(accountID uint, date string) (*models.CloudStats, error) {
	var stats models.CloudStats
	err := r.db.Where("account_id = ? AND date = ?", accountID, date).First(&stats).Error
	if err != nil {
		return nil, err
	}
	return &stats, nil
}

// FindByDateRange 根据日期范围查找统计记录
func (r *CloudStatsRepository) FindByDateRange(startDate, endDate string) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("date BETWEEN ? AND ?", startDate, endDate).Order("date DESC").Find(&stats).Error
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
	var existing models.CloudStats
	err := r.db.Where("account_id = ? AND date = ?", stats.AccountID, stats.Date).First(&existing).Error

	if err == nil {
		// 记录存在，更新
		stats.ID = existing.ID
		return r.db.Save(stats).Error
	}

	if err == gorm.ErrRecordNotFound {
		// 记录不存在，创建
		return r.db.Create(stats).Error
	}

	return err
}

// GetTodayStatsByUserID 获取用户今日的所有统计记录
func (r *CloudStatsRepository) GetTodayStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	today := time.Now().In(cstZone).Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, today).Find(&stats).Error
	return stats, err
}

// GetYesterdayStatsByUserID 获取用户昨日的统计记录
func (r *CloudStatsRepository) GetYesterdayStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	yesterday := time.Now().In(cstZone).AddDate(0, 0, -1).Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, yesterday).Find(&stats).Error
	return stats, err
}

// GetLastWeekStatsByUserID 获取用户上周同期的统计记录
func (r *CloudStatsRepository) GetLastWeekStatsByUserID(userID uint) ([]*models.CloudStats, error) {
	lastWeek := time.Now().In(cstZone).AddDate(0, 0, -7).Format("2006-01-02")
	var stats []*models.CloudStats
	err := r.db.Preload("Account").Where("user_id = ? AND date = ?", userID, lastWeek).Find(&stats).Error
	return stats, err
}

// GetTotalCloudCountByUserID 获取用户所有账号的总云朵数（最近一天）
func (r *CloudStatsRepository) GetTotalCloudCountByUserID(userID uint) (int, error) {
	var total int
	err := r.db.Model(&models.CloudStats{}).
		Where("user_id = ?", userID).
		Order("date DESC").
		Limit(1).
		Select("COALESCE(SUM(cloud_count), 0)").
		Scan(&total).Error
	return total, err
}

// GetTrendDataByUserID 获取用户最近N天的趋势数据
func (r *CloudStatsRepository) GetTrendDataByUserID(userID uint, days int) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	now := time.Now().In(cstZone)
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days+1).Format("2006-01-02")

	err := r.db.Model(&models.CloudStats{}).
		Where("user_id = ? AND date BETWEEN ? AND ?", userID, startDate, endDate).
		Group("date").
		Select("date, SUM(cloud_count) as cloud_count, 0 as cloud_diff, 0 as cloud_diff_week, created_at, updated_at").
		Order("date ASC").
		Find(&stats).Error
	return stats, err
}

// GetTrendDataGlobal 获取全局最近N天的趋势数据
func (r *CloudStatsRepository) GetTrendDataGlobal(days int) ([]*models.CloudStats, error) {
	var stats []*models.CloudStats
	now := time.Now().In(cstZone)
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -days+1).Format("2006-01-02")

	err := r.db.Model(&models.CloudStats{}).
		Where("date BETWEEN ? AND ?", startDate, endDate).
		Group("date").
		Select("date, SUM(cloud_count) as cloud_count, 0 as cloud_diff, 0 as cloud_diff_week, created_at, updated_at").
		Order("date ASC").
		Find(&stats).Error
	return stats, err
}

// GetDashboardStats 获取仪表盘统计数据
func (r *CloudStatsRepository) GetDashboardStats(userID uint) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	// 获取今日统计数据
	today := time.Now().Format("2006-01-02")
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
	yesterdayDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	yesterdayStats, err := r.FindByUserIDAndDate(userID, yesterdayDate)
	if err != nil {
		return nil
	}

	// 创建昨日的云朵数量映射
	yesterdayMap := make(map[uint]int)
	for _, stat := range yesterdayStats {
		yesterdayMap[stat.AccountID] = stat.CloudCount
	}

	// 获取今日的数据
	todayDate := time.Now().Format("2006-01-02")
	todayStats, err := r.FindByUserIDAndDate(userID, todayDate)
	if err != nil {
		return nil
	}

	// 更新今日数据的差异
	for _, stat := range todayStats {
		if yesterdayCount, ok := yesterdayMap[stat.AccountID]; ok {
			stat.CloudDiff = stat.CloudCount - yesterdayCount
		}
		r.db.Save(stat)
	}

	return nil
}

// CalculateWeeklyDiff 计算对比上周的变化
func (r *CloudStatsRepository) CalculateWeeklyDiff(userID uint) error {
	// 获取上周同期的数据
	lastWeekDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekStats, err := r.FindByUserIDAndDate(userID, lastWeekDate)
	if err != nil {
		return nil
	}

	// 创建上周的云朵数量映射
	lastWeekMap := make(map[uint]int)
	for _, stat := range lastWeekStats {
		lastWeekMap[stat.AccountID] = stat.CloudCount
	}

	// 获取今日的数据
	todayDate := time.Now().Format("2006-01-02")
	todayStats, err := r.FindByUserIDAndDate(userID, todayDate)
	if err != nil {
		return nil
	}

	// 更新今日数据的周差异
	for _, stat := range todayStats {
		if lastWeekCount, ok := lastWeekMap[stat.AccountID]; ok {
			stat.CloudDiffWeek = stat.CloudCount - lastWeekCount
		}
		r.db.Save(stat)
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
