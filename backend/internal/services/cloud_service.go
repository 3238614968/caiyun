package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"fmt"
	"sort"
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
	now := time.Now().In(cstZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
	tomorrow := today.Add(24 * time.Hour)

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

	// 获取昨日云朵数并计算差异
	yesterdayDate := today.AddDate(0, 0, -1).Format("2006-01-02")
	yesterdayStats, err := s.cloudStatsRepo.FindByUserIDAndDate(userID, yesterdayDate)
	if err == nil && len(yesterdayStats) > 0 {
		yesterdayTotal := sumCloudStats(yesterdayStats)
		data.YesterdayDiff = totalCloud - yesterdayTotal
	}

	// 今日获得优先使用当前总数与昨日快照的差值，更贴近首页“当前云朵数”的变化；
	// 若尚未有昨日快照，则回退到今日任务日志汇总。
	if data.YesterdayDiff != 0 || len(yesterdayStats) > 0 {
		data.TodayGained = data.YesterdayDiff
	} else {
		data.TodayGained = s.taskLogRepo.GetCloudGainedByUserAndRange(userID, today, tomorrow)
	}

	// 获取上周云朵数并计算差异
	lastWeekDate := today.AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekStats, err := s.cloudStatsRepo.FindByUserIDAndDate(userID, lastWeekDate)
	if err == nil && len(lastWeekStats) > 0 {
		data.WeekDiff = totalCloud - sumCloudStats(lastWeekStats)
	}

	// 获取今日任务成功率，避免历史任务把首页成功率长期稀释。
	todayTotal := s.taskLogRepo.CountByUserStatusAndRange(userID, "", today, tomorrow)
	if todayTotal > 0 {
		todaySuccess := s.taskLogRepo.CountByUserStatusAndRange(userID, "success", today, tomorrow)
		data.SuccessRate = float64(todaySuccess) / float64(todayTotal) * 100
	}

	// 获取趋势数据（最近7天）
	trendStats, err := s.cloudStatsRepo.GetTrendDataByUserID(userID, 7)
	if err == nil {
		data.TrendData = completeTrendData(trendStats, 7, totalCloud)
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
		sort.Slice(data.AccountRanking, func(i, j int) bool {
			return data.AccountRanking[i].CloudCount > data.AccountRanking[j].CloudCount
		})
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
	const batchSize = 200
	for offset := 0; ; offset += batchSize {
		accounts, err := s.accountRepo.FindActiveAccountsPaged(offset, batchSize)
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			return nil
		}

		if err := s.calculateDailyStatsForAccounts(accounts); err != nil {
			return err
		}
		if len(accounts) < batchSize {
			return nil
		}
	}
}

// CalculateDailyStatsByUserID 仅计算指定用户的每日统计数据。
func (s *CloudService) CalculateDailyStatsByUserID(userID uint) error {
	accounts, err := s.accountRepo.FindActiveAccountsByUserID(userID)
	if err != nil {
		return err
	}

	return s.calculateDailyStatsForAccounts(accounts)
}

func (s *CloudService) calculateDailyStatsForAccounts(accounts []*models.Account) error {
	// 获取今天的日期
	now := time.Now().In(cstZone)
	today := now.Format("2006-01-02")

	// 为每个账号创建/更新统计数据
	for _, account := range accounts {
		stats := &models.CloudStats{
			UserID:     account.UserID,
			AccountID:  account.ID,
			Date:       today,
			CloudCount: account.CloudCount,
		}

		// 计算对比昨日的变化
		yesterdayDate := now.AddDate(0, 0, -1).Format("2006-01-02")
		yesterdayStats, err := s.cloudStatsRepo.FindByAccountIDAndDate(account.ID, yesterdayDate)
		if err == nil && yesterdayStats != nil {
			stats.CloudDiff = account.CloudCount - yesterdayStats.CloudCount
		}

		// 计算对比上周的变化
		lastWeekDate := now.AddDate(0, 0, -7).Format("2006-01-02")
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
	days = normalizeTrendDays(days)
	trendStats, err := s.cloudStatsRepo.GetTrendDataByUserID(userID, days)
	if err != nil {
		return nil, err
	}

	totalCloud, err := s.accountRepo.GetTotalCloudCountByUserID(userID)
	if err != nil {
		return nil, err
	}

	return completeTrendData(trendStats, days, totalCloud), nil
}

// GetGlobalTrendData 获取全局趋势数据
func (s *CloudService) GetGlobalTrendData(days int) ([]TrendPoint, error) {
	days = normalizeTrendDays(days)
	trendStats, err := s.cloudStatsRepo.GetTrendDataGlobal(days)
	if err != nil {
		return nil, err
	}

	totalCloud, err := s.accountRepo.SumCloudCount()
	if err != nil {
		return nil, err
	}

	return completeTrendData(trendStats, days, totalCloud), nil
}

func normalizeTrendDays(days int) int {
	if days < 1 || days > 365 {
		return 7
	}
	return days
}

func sumCloudStats(stats []*models.CloudStats) int {
	total := 0
	for _, stat := range stats {
		total += stat.CloudCount
	}
	return total
}

func completeTrendData(stats []*models.CloudStats, days int, currentTotal int) []TrendPoint {
	days = normalizeTrendDays(days)
	now := time.Now().In(cstZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
	start := today.AddDate(0, 0, -days+1)
	todayKey := today.Format("2006-01-02")

	statMap := make(map[string]int, len(stats))
	for _, stat := range stats {
		statMap[stat.Date] = stat.CloudCount
	}

	result := make([]TrendPoint, 0, days)
	lastKnown := 0
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		if cloudCount, ok := statMap[date]; ok {
			lastKnown = cloudCount
		}
		if date == todayKey {
			lastKnown = currentTotal
		}

		result = append(result, TrendPoint{
			Date:       date,
			CloudCount: lastKnown,
		})
	}

	return result
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
