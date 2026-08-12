package services

import (
	"caiyun/internal/models"
	"time"
)

// cstZone is the business reporting time zone.
var cstZone = time.FixedZone("CST", 8*3600)

// nowCST is the business clock for all service-level reporting dates.
func nowCST() time.Time {
	return time.Now().In(cstZone)
}

// todayStartCST returns the start of the current day in the reporting time zone.
func todayStartCST() time.Time {
	now := nowCST()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
}

// StatsOverview 统计概览
type StatsOverview struct {
	UserCount    int64 `json:"user_count"`
	AccountCount int64 `json:"account_count"`
	TotalCloud   int   `json:"total_cloud"`
	ActiveTasks  int   `json:"active_tasks"`
}

// GetStatsOverview 获取统计概览
func (s *AdminService) GetStatsOverview() (*StatsOverview, error) {
	_, userTotal, err := s.userRepo.List(0, 1)
	if err != nil {
		return nil, err
	}

	_, accountTotal, err := s.accountRepo.List(0, 1)
	if err != nil {
		return nil, err
	}

	totalCloud, err := s.accountRepo.SumCloudCount()
	if err != nil {
		return nil, err
	}

	activeAccountCount, err := s.accountRepo.CountActive()
	if err != nil {
		return nil, err
	}

	return &StatsOverview{
		UserCount:    userTotal,
		AccountCount: accountTotal,
		TotalCloud:   totalCloud,
		ActiveTasks:  int(activeAccountCount),
	}, nil
}

// AccountSummary 账号概况（管理员查看所有账号）
type AccountSummary struct {
	ID              uint   `json:"id"`
	Phone           string `json:"phone"`
	Remark          string `json:"remark"`
	OwnerUsername   string `json:"owner_username"`
	CloudCount      int    `json:"cloud_count"`
	IsActive        bool   `json:"is_active"`
	CreatedAt       string `json:"created_at"`
	TodayGained     int    `json:"today_gained"`
	YesterdayGained int    `json:"yesterday_gained"`
	SuccessCount    int64  `json:"success_count"`
	FailedCount     int64  `json:"failed_count"`
	LastExecutedAt  string `json:"last_executed_at"`
}

// GetAccountSummaries 获取所有账号概况
func (s *AdminService) GetAccountSummaries(page, pageSize int) ([]*AccountSummary, int64, error) {
	offset := (page - 1) * pageSize
	accounts, total, err := s.accountRepo.List(offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	today := todayStartCST()
	tomorrow := today.Add(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)
	accountIDs := make([]uint, 0, len(accounts))
	for _, acc := range accounts {
		accountIDs = append(accountIDs, acc.ID)
	}
	logSummaries, err := s.taskLogRepo.GetAccountSummariesByIDs(accountIDs, today, tomorrow, yesterday)
	if err != nil {
		return nil, 0, err
	}

	summaries := make([]*AccountSummary, len(accounts))
	for i, acc := range accounts {
		summary := &AccountSummary{
			ID:         acc.ID,
			Phone:      acc.Phone,
			Remark:     acc.Remark,
			CloudCount: acc.CloudCount,
			IsActive:   acc.IsActive,
			CreatedAt:  acc.CreatedAt.Format("2006-01-02 15:04:05"),
		}

		if acc.User.ID > 0 {
			summary.OwnerUsername = acc.User.Username
		}

		if logSummary, ok := logSummaries[acc.ID]; ok {
			summary.TodayGained = logSummary.TodayGained
			summary.YesterdayGained = logSummary.YesterdayGained
			summary.SuccessCount = logSummary.SuccessCount
			summary.FailedCount = logSummary.FailedCount
			if logSummary.LastExecutedAt.Valid {
				summary.LastExecutedAt = logSummary.LastExecutedAt.Time.Format("2006-01-02 15:04:05")
			}
		}

		summaries[i] = summary
	}

	return summaries, total, nil
}

// AdminDashboardData 管理员仪表盘数据
type AdminDashboardData struct {
	TotalCloud      int                `json:"total_cloud"`
	AccountCount    int64              `json:"account_count"`
	UserCount       int64              `json:"user_count"`
	TodayGained     int                `json:"today_gained"`
	YesterdayGained int                `json:"yesterday_gained"`
	SuccessRate     float64            `json:"success_rate"`
	AccountRanking  []AdminAccountRank `json:"account_ranking"`
}

// AdminAccountRank admin dashboard account ranking item
type AdminAccountRank struct {
	AccountID     uint   `json:"account_id"`
	Phone         string `json:"phone"`
	Remark        string `json:"remark"`
	OwnerUsername string `json:"owner_username"`
	CloudCount    int    `json:"cloud_count"`
	TodayGained   int    `json:"today_gained"`
}

// GetAdminDashboard 获取管理员仪表盘数据（全局视角）
func (s *AdminService) GetAdminDashboard() (*AdminDashboardData, error) {
	data := &AdminDashboardData{}

	// User count
	_, userTotal, err := s.userRepo.List(0, 1)
	if err != nil {
		return nil, err
	}
	data.UserCount = userTotal

	// Account count
	_, accountTotal, err := s.accountRepo.List(0, 1)
	if err != nil {
		return nil, err
	}
	data.AccountCount = accountTotal

	// Total cloud
	totalCloud, err := s.accountRepo.SumCloudCount()
	if err != nil {
		return nil, err
	}
	data.TotalCloud = totalCloud

	today := todayStartCST()
	tomorrow := today.Add(24 * time.Hour)
	yesterday := today.Add(-24 * time.Hour)

	// Today gained (all accounts)
	data.TodayGained = s.taskLogRepo.GetCloudGainedGlobal(today, tomorrow)

	// Yesterday gained
	data.YesterdayGained = s.taskLogRepo.GetCloudGainedGlobal(yesterday, today)

	// Global success rate (today)
	successCount := s.taskLogRepo.CountByStatusAndRange("success", today, tomorrow)
	totalCount := s.taskLogRepo.CountByStatusAndRange("", today, tomorrow)
	if totalCount > 0 {
		data.SuccessRate = float64(successCount) / float64(totalCount) * 100
	}

	// Account ranking (top 20 by cloud_count)
	topAccounts, err := s.accountRepo.TopByCloudCount(20)
	if err != nil {
		return nil, err
	}
	rankingIDs := make([]uint, 0, len(topAccounts))
	for _, acc := range topAccounts {
		rankingIDs = append(rankingIDs, acc.ID)
	}
	logSummaries, err := s.taskLogRepo.GetAccountSummariesByIDs(rankingIDs, today, tomorrow, yesterday)
	if err != nil {
		return nil, err
	}
	ranking := make([]AdminAccountRank, 0, len(topAccounts))
	for _, acc := range topAccounts {
		ownerName := ""
		if acc.User.ID > 0 {
			ownerName = acc.User.Username
		}
		todayGained := 0
		if summary, ok := logSummaries[acc.ID]; ok {
			todayGained = summary.TodayGained
		}
		ranking = append(ranking, AdminAccountRank{
			AccountID:     acc.ID,
			Phone:         acc.Phone,
			Remark:        acc.Remark,
			OwnerUsername: ownerName,
			CloudCount:    acc.CloudCount,
			TodayGained:   todayGained,
		})
	}
	data.AccountRanking = ranking

	return data, nil
}

// GetTaskConfigs 获取所有任务配置
func (s *AdminService) GetTaskConfigs() ([]*models.TaskConfig, error) {
	return s.taskConfigRepo.List()
}

// UpdateTaskConfigRequest 更新任务配置请求
type UpdateTaskConfigRequest struct {
	IsEnabled bool `json:"is_enabled"`
}

// UpdateTaskConfig 更新任务配置（上架/下架）
func (s *AdminService) UpdateTaskConfig(taskType string, req *UpdateTaskConfigRequest) error {
	return s.taskConfigRepo.UpdateEnabled(taskType, req.IsEnabled)
}
