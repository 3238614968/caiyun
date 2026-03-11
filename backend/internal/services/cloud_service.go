package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"time"
)

type CloudService struct {
	accountRepo    *repository.AccountRepository
	cloudStatsRepo *repository.CloudStatsRepository
	taskLogRepo    *repository.TaskLogRepository
}

func NewCloudService(
	accountRepo *repository.AccountRepository,
	cloudStatsRepo *repository.CloudStatsRepository,
	taskLogRepo *repository.TaskLogRepository,
) *CloudService {
	return &CloudService{
		accountRepo:    accountRepo,
		cloudStatsRepo: cloudStatsRepo,
		taskLogRepo:    taskLogRepo,
	}
}

// DashboardData 仪表盘数据
type DashboardData struct {
	TotalCloud     int           `json:"total_cloud"`     // 总云朵数
	AccountCount   int           `json:"account_count"`   // 账号数
	TodayGained    int           `json:"today_gained"`    // 今日获得
	YesterdayDiff  int           `json:"yesterday_diff"`  // 对比昨日
	WeekDiff       int           `json:"week_diff"`       // 对比上周
	SuccessRate    float64       `json:"success_rate"`    // 任务成功率
	TrendData      []TrendPoint  `json:"trend_data"`      // 趋势数据
	AccountRanking []AccountRank `json:"account_ranking"` // 账号排名
}

// TrendPoint 趋势点
type TrendPoint struct {
	Date       string `json:"date"`
	CloudCount int    `json:"cloud_count"`
}

// AccountRank 账号排名
type AccountRank struct {
	AccountID  uint   `json:"account_id"`
	Phone      string `json:"phone"`
	Remark     string `json:"remark"`
	CloudCount int    `json:"cloud_count"`
}

// GetDashboard 获取仪表盘数据
func (s *CloudService) GetDashboard(userID uint) (*DashboardData, error) {
	data := &DashboardData{}

	// 获取总云朵数
	totalCloud, err := s.accountRepo.GetTotalCloudCountByUserID(userID)
	if err != nil {
		return nil, err
	}
	data.TotalCloud = totalCloud

	// 获取账号数
	accounts, err := s.accountRepo.FindByUserID(userID)
	if err != nil {
		return nil, err
	}
	data.AccountCount = len(accounts)

	// 获取今日获得云朵数
	todayGained, err := s.taskLogRepo.GetTodayCloudGainedByUserID(userID)
	if err != nil {
		return nil, err
	}
	data.TodayGained = todayGained

	// 获取昨日云朵数并计算差异
	yesterdayDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
	yesterdayStats, err := s.cloudStatsRepo.FindByUserIDAndDate(userID, yesterdayDate)
	if err == nil && len(yesterdayStats) > 0 {
		yesterdayTotal := 0
		for _, stat := range yesterdayStats {
			yesterdayTotal += stat.CloudCount
		}
		data.YesterdayDiff = totalCloud - yesterdayTotal
	}

	// 获取上周云朵数并计算差异
	lastWeekDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekStats, err := s.cloudStatsRepo.FindByUserIDAndDate(userID, lastWeekDate)
	if err == nil && len(lastWeekStats) > 0 {
		lastWeekTotal := 0
		for _, stat := range lastWeekStats {
			lastWeekTotal += stat.CloudCount
		}
		data.WeekDiff = totalCloud - lastWeekTotal
	}

	// 获取任务成功率
	successRate, err := s.taskLogRepo.GetSuccessRate(userID)
	if err != nil {
		data.SuccessRate = 0
	} else {
		data.SuccessRate = successRate
	}

	// 获取趋势数据（最近7天）
	trendStats, err := s.cloudStatsRepo.GetTrendDataByUserID(userID, 7)
	if err == nil {
		data.TrendData = make([]TrendPoint, len(trendStats))
		for i, stat := range trendStats {
			data.TrendData[i] = TrendPoint{
				Date:       stat.Date,
				CloudCount: stat.CloudCount,
			}
		}
	}

	// 获取账号排名
	if len(accounts) > 0 {
		data.AccountRanking = make([]AccountRank, len(accounts))
		for i, account := range accounts {
			data.AccountRanking[i] = AccountRank{
				AccountID:  account.ID,
				Phone:      account.Phone,
				Remark:     account.Remark,
				CloudCount: account.CloudCount,
			}
		}
		// 按云朵数排序（降序）
		for i := 0; i < len(data.AccountRanking)-1; i++ {
			for j := i + 1; j < len(data.AccountRanking); j++ {
				if data.AccountRanking[i].CloudCount < data.AccountRanking[j].CloudCount {
					data.AccountRanking[i], data.AccountRanking[j] = data.AccountRanking[j], data.AccountRanking[i]
				}
			}
		}
	}

	return data, nil
}

// GetCloudStatsByAccount 获取指定账号的云朵统计
func (s *CloudService) GetCloudStatsByAccount(userID, accountID uint, page, pageSize int) ([]*models.CloudStats, int64, error) {
	// 验证账号所有权
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return nil, 0, err
	}
	if account.UserID != userID {
		return nil, 0, fmt.Errorf("账号不存在")
	}

	offset := (page - 1) * pageSize
	return s.cloudStatsRepo.FindByAccountID(accountID, offset, pageSize)
}

// GetCloudStatsByUserID 获取用户的所有云朵统计
func (s *CloudService) GetCloudStatsByUserID(userID uint, page, pageSize int) ([]*models.CloudStats, int64, error) {
	offset := (page - 1) * pageSize
	return s.cloudStatsRepo.FindByUserID(userID, offset, pageSize)
}

// CalculateDailyStats 计算每日统计数据
func (s *CloudService) CalculateDailyStats() error {
	// 获取所有账号
	accounts, err := s.accountRepo.FindActiveAccounts()
	if err != nil {
		return err
	}

	// 获取今天的日期
	today := time.Now().Format("2006-01-02")

	// 为每个账号创建/更新统计数据
	for _, account := range accounts {
		stats := &models.CloudStats{
			UserID:     account.UserID,
			AccountID:  account.ID,
			Date:       today,
			CloudCount: account.CloudCount,
		}

		// 计算对比昨日的变化
		yesterdayDate := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		yesterdayStats, err := s.cloudStatsRepo.FindByAccountIDAndDate(account.ID, yesterdayDate)
		if err == nil && yesterdayStats != nil {
			stats.CloudDiff = account.CloudCount - yesterdayStats.CloudCount
		}

		// 计算对比上周的变化
		lastWeekDate := time.Now().AddDate(0, 0, -7).Format("2006-01-02")
		lastWeekStats, err := s.cloudStatsRepo.FindByAccountIDAndDate(account.ID, lastWeekDate)
		if err == nil && lastWeekStats != nil {
			stats.CloudDiffWeek = account.CloudCount - lastWeekStats.CloudCount
		}

		// 插入或更新
		if err := s.cloudStatsRepo.UpsertByAccountIDAndDate(stats); err != nil {
			// 继续处理其他账号
			continue
		}
	}

	return nil
}

// UpdateCloudDiffs 更新所有统计数据的差异值
func (s *CloudService) UpdateCloudDiffs(userID uint) error {
	// 计算对比昨日的差异
	if err := s.cloudStatsRepo.CalculateDailyDiff(userID); err != nil {
		return err
	}

	// 计算对比上周的差异
	if err := s.cloudStatsRepo.CalculateWeeklyDiff(userID); err != nil {
		return err
	}

	return nil
}

// GetTotalCloudCount 获取用户总云朵数
func (s *CloudService) GetTotalCloudCount(userID uint) (int, error) {
	return s.accountRepo.GetTotalCloudCountByUserID(userID)
}

// GetTrendData 获取趋势数据
func (s *CloudService) GetTrendData(userID uint, days int) ([]TrendPoint, error) {
	trendStats, err := s.cloudStatsRepo.GetTrendDataByUserID(userID, days)
	if err != nil {
		return nil, err
	}

	result := make([]TrendPoint, len(trendStats))
	for i, stat := range trendStats {
		result[i] = TrendPoint{
			Date:       stat.Date,
			CloudCount: stat.CloudCount,
		}
	}

	return result, nil
}

// GetAccountCloudCount 获取账号云朵数
func (s *CloudService) GetAccountCloudCount(accountID uint) (int, error) {
	account, err := s.accountRepo.FindByID(accountID)
	if err != nil {
		return 0, err
	}
	return account.CloudCount, nil
}

// UpdateAccountCloudCount 更新账号云朵数
func (s *CloudService) UpdateAccountCloudCount(accountID uint, cloudCount int) error {
	return s.accountRepo.UpdateCloudCount(accountID, cloudCount)
}
