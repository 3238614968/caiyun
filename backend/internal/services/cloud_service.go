package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
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
	Date            string `json:"date"`
	CloudCount      int    `json:"cloud_count"`
	CloudDiff       int    `json:"cloud_diff"`
	HasData         bool   `json:"has_data"`
	Comparable      bool   `json:"comparable"`
	SampledAccounts int    `json:"sampled_accounts"`
	AccountCount    int    `json:"account_count"`
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
	now := nowCST()
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
		data.YesterdayDiff = comparableCloudBalanceDiff(accounts, yesterdayStats)
	}

	// Task earnings are separate from balance changes and account additions.
	data.TodayGained = s.taskLogRepo.GetCloudGainedByUserAndRange(userID, today, tomorrow)

	// 获取上周云朵数并计算差异
	lastWeekDate := today.AddDate(0, 0, -7).Format("2006-01-02")
	lastWeekStats, err := s.cloudStatsRepo.FindByUserIDAndDate(userID, lastWeekDate)
	if err == nil && len(lastWeekStats) > 0 {
		data.WeekDiff = comparableCloudBalanceDiff(accounts, lastWeekStats)
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
		data.TrendData = completeTrendData(trendStats, 7, totalCloud, len(accounts))
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
		return nil, 0, ErrAccountNotFound
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
	var lastID uint
	for {
		accounts, err := s.accountRepo.FindCloudBalancesAfterID(lastID, batchSize)
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			return nil
		}
		lastID = accounts[len(accounts)-1].ID

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
	accounts, err := s.accountRepo.FindByUserID(userID)
	if err != nil {
		return err
	}

	return s.calculateDailyStatsForAccounts(accounts)
}

func (s *CloudService) calculateDailyStatsForAccounts(accounts []*models.Account) error {
	now := nowCST()
	today := now.Format("2006-01-02")
	yesterday := now.AddDate(0, 0, -1).Format("2006-01-02")
	lastWeek := now.AddDate(0, 0, -7).Format("2006-01-02")
	ids := make([]uint, 0, len(accounts))
	for _, account := range accounts {
		if account != nil {
			ids = append(ids, account.ID)
		}
	}
	previous, err := s.cloudStatsRepo.ComparisonSnapshots(ids, []string{yesterday, lastWeek})
	if err != nil {
		return err
	}
	baseline := make(map[uint]map[string]int)
	for _, stat := range previous {
		if baseline[stat.AccountID] == nil {
			baseline[stat.AccountID] = make(map[string]int)
		}
		baseline[stat.AccountID][stat.Date] = stat.CloudCount
	}
	stats := make([]*models.CloudStats, 0, len(accounts))
	for _, account := range accounts {
		if account == nil {
			continue
		}
		snapshot := &models.CloudStats{UserID: account.UserID, AccountID: account.ID, Date: today, CloudCount: account.CloudCount}
		if balance, known := baseline[account.ID][yesterday]; known {
			snapshot.CloudDiff = account.CloudCount - balance
		}
		if balance, known := baseline[account.ID][lastWeek]; known {
			snapshot.CloudDiffWeek = account.CloudCount - balance
		}
		stats = append(stats, snapshot)
	}
	if err := s.cloudStatsRepo.UpsertBatch(stats); err != nil {
		return fmt.Errorf("write daily balance snapshots: %w", err)
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

	totalCloud, accountCount, err := s.accountRepo.CloudBalanceSummary(&userID)
	if err != nil {
		return nil, err
	}

	return completeTrendData(trendStats, days, totalCloud, accountCount), nil
}

// GetGlobalTrendData 获取全局趋势数据
func (s *CloudService) GetGlobalTrendData(days int) ([]TrendPoint, error) {
	days = normalizeTrendDays(days)
	trendStats, err := s.cloudStatsRepo.GetTrendDataGlobal(days)
	if err != nil {
		return nil, err
	}

	totalCloud, accountCount, err := s.accountRepo.CloudBalanceSummary(nil)
	if err != nil {
		return nil, err
	}

	return completeTrendData(trendStats, days, totalCloud, accountCount), nil
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

func comparableCloudBalanceDiff(accounts []*models.Account, snapshots []*models.CloudStats) int {
	baseline := make(map[uint]int, len(snapshots))
	for _, stat := range snapshots {
		baseline[stat.AccountID] = stat.CloudCount
	}
	diff := 0
	for _, account := range accounts {
		if previous, ok := baseline[account.ID]; ok {
			diff += account.CloudCount - previous
		}
	}
	return diff
}

func completeTrendData(stats []*models.CloudStats, days, currentTotal int, population ...int) []TrendPoint {
	count := 0
	if len(population) > 0 {
		count = population[0]
	}
	return completeTrendDataAt(stats, days, currentTotal, count, nowCST())
}

func completeTrendDataAt(stats []*models.CloudStats, days, currentTotal, accountCount int, now time.Time) []TrendPoint {
	days = normalizeTrendDays(days)
	local := now.In(cstZone)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, cstZone)
	start := today.AddDate(0, 0, -days+1)
	byDate := make(map[string]*models.CloudStats, len(stats))
	for _, stat := range stats {
		if stat != nil {
			byDate[stat.Date] = stat
		}
	}
	result := make([]TrendPoint, 0, days)
	for i := 0; i < days; i++ {
		date := start.AddDate(0, 0, i).Format("2006-01-02")
		point := TrendPoint{Date: date, AccountCount: accountCount}
		if stat := byDate[date]; stat != nil {
			point.CloudCount = stat.CloudCount
			point.SampledAccounts = stat.SampledAccounts
			point.HasData = accountCount == 0 || stat.SampledAccounts == accountCount
		}
		if i == days-1 {
			point.CloudCount = currentTotal
			point.SampledAccounts = accountCount
			point.HasData = true
		}
		if i > 0 && point.HasData && result[i-1].HasData {
			point.CloudDiff = point.CloudCount - result[i-1].CloudCount
			point.Comparable = true
		}
		// Missing or partial snapshots stay marked as such. They are never
		// backfilled with an invented balance or counted as daily growth.
		result = append(result, point)
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

// withContext returns a short-lived service clone whose repositories are bound
// to one request. It deliberately does not store request context globally.
func (s *CloudService) withContext(ctx context.Context) *CloudService {
	if ctx == nil {
		ctx = context.Background()
	}
	return &CloudService{
		accountRepo:    s.accountRepo.WithContext(ctx),
		cloudStatsRepo: s.cloudStatsRepo.WithContext(ctx),
		taskLogRepo:    s.taskLogRepo.WithContext(ctx),
	}
}

func (s *CloudService) GetDashboardContext(ctx context.Context, userID uint) (*DashboardData, error) {
	return s.withContext(ctx).GetDashboard(userID)
}

func (s *CloudService) GetCloudStatsByAccountContext(ctx context.Context, userID, accountID uint, page, pageSize int) ([]*models.CloudStats, int64, error) {
	return s.withContext(ctx).GetCloudStatsByAccount(userID, accountID, page, pageSize)
}

func (s *CloudService) GetCloudStatsByUserIDContext(ctx context.Context, userID uint, page, pageSize int) ([]*models.CloudStats, int64, error) {
	return s.withContext(ctx).GetCloudStatsByUserID(userID, page, pageSize)
}

func (s *CloudService) CalculateDailyStatsContext(ctx context.Context) error {
	return s.withContext(ctx).CalculateDailyStats()
}

func (s *CloudService) CalculateDailyStatsByUserIDContext(ctx context.Context, userID uint) error {
	return s.withContext(ctx).CalculateDailyStatsByUserID(userID)
}

func (s *CloudService) UpdateCloudDiffsContext(ctx context.Context, userID uint) error {
	return s.withContext(ctx).UpdateCloudDiffs(userID)
}

func (s *CloudService) GetTotalCloudCountContext(ctx context.Context, userID uint) (int, error) {
	return s.withContext(ctx).GetTotalCloudCount(userID)
}

func (s *CloudService) GetTrendDataContext(ctx context.Context, userID uint, days int) ([]TrendPoint, error) {
	return s.withContext(ctx).GetTrendData(userID, days)
}

func (s *CloudService) GetGlobalTrendDataContext(ctx context.Context, days int) ([]TrendPoint, error) {
	return s.withContext(ctx).GetGlobalTrendData(days)
}
