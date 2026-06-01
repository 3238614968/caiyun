package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// 北京时间时区
var cstZone = time.FixedZone("CST", 8*3600)

// todayStartCST 获取北京时间今天0点
func todayStartCST() time.Time {
	now := time.Now().In(cstZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
}

var (
	ErrUserNotFound     = errors.New("用户不存在")
	ErrCannotDeleteSelf = errors.New("不能删除自己")
)

// AdminService 管理员服务
type AdminService struct {
	userRepo       *repository.UserRepository
	accountRepo    *repository.AccountRepository
	taskLogRepo    *repository.TaskLogRepository
	taskConfigRepo *repository.TaskConfigRepository
}

// NewAdminService 创建管理员服务
func NewAdminService(
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
	taskLogRepo *repository.TaskLogRepository,
	taskConfigRepo *repository.TaskConfigRepository,
) *AdminService {
	return &AdminService{
		userRepo:       userRepo,
		accountRepo:    accountRepo,
		taskLogRepo:    taskLogRepo,
		taskConfigRepo: taskConfigRepo,
	}
}

// UserListItem 用户列表项
type UserListItem struct {
	ID        uint   `json:"id"`
	Username  string `json:"username"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
}

// GetAllUsers 获取所有用户
func (s *AdminService) GetAllUsers(page, size int) ([]*UserListItem, int64, error) {
	offset := (page - 1) * size
	users, total, err := s.userRepo.List(offset, size)
	if err != nil {
		return nil, 0, err
	}

	result := make([]*UserListItem, len(users))
	for i, user := range users {
		result[i] = &UserListItem{
			ID:        user.ID,
			Username:  user.Username,
			Email:     user.Email,
			Role:      user.Role,
			CreatedAt: user.CreatedAt.Format("2006-01-02 15:04:05"),
		}
	}

	return result, total, nil
}

// GetAllAccounts 获取所有账号
func (s *AdminService) GetAllAccounts(page, pageSize int) ([]*models.Account, int64, error) {
	offset := (page - 1) * pageSize
	return s.accountRepo.List(offset, pageSize)
}

// SearchAllAccountsRequest 搜索所有账号请求
type SearchAllAccountsRequest struct {
	Keyword string `json:"keyword" form:"keyword"`
	Limit   int    `json:"limit" form:"limit"`
}

// SearchAllAccountsResponse 搜索所有账号响应
type SearchAllAccountsResponse struct {
	Accounts []*AccountSearchItem `json:"accounts"`
}

// AccountSearchItem 账号搜索项
type AccountSearchItem struct {
	ID       uint   `json:"id"`
	Phone    string `json:"phone"`
	Remark   string `json:"remark"`
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	IsActive bool   `json:"is_active"`
}

// SearchAllAccounts 搜索所有账号（管理员用）
func (s *AdminService) SearchAllAccounts(req *SearchAllAccountsRequest) (*SearchAllAccountsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}

	accounts, err := s.accountRepo.SearchAll(req.Keyword, limit)
	if err != nil {
		return nil, err
	}

	result := make([]*AccountSearchItem, 0, len(accounts))
	for _, acc := range accounts {
		// 获取用户信息
		user, err := s.userRepo.FindByID(acc.UserID)
		username := ""
		if err == nil && user != nil {
			username = user.Username
		}

		result = append(result, &AccountSearchItem{
			ID:       acc.ID,
			Phone:    acc.Phone,
			Remark:   acc.Remark,
			UserID:   acc.UserID,
			Username: username,
			IsActive: acc.IsActive,
		})
	}

	return &SearchAllAccountsResponse{
		Accounts: result,
	}, nil
}

// UpdateUserRoleRequest 更新用户角色请求
type UpdateUserRoleRequest struct {
	Role string `json:"role" binding:"required,oneof=user admin"`
}

// ResetUserPasswordRequest 管理员重置用户密码请求
type ResetUserPasswordRequest struct {
	Password string `json:"password" binding:"required,min=12"`
}

// UpdateUserRole 更新用户角色
func (s *AdminService) UpdateUserRole(userID uint, req *UpdateUserRoleRequest) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	user.Role = req.Role
	return s.userRepo.Update(user)
}

// ResetUserPassword 管理员重置用户密码
func (s *AdminService) ResetUserPassword(userID uint, req *ResetUserPasswordRequest) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if err := validatePasswordStrength(user.Username, req.Password); err != nil {
		return err
	}
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	return s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword))
}

// UpdateAccountStatusRequest 更新账号状态请求
type UpdateAccountStatusRequest struct {
	IsActive bool `json:"is_active"`
}

// UpdateAccountStatus 更新账号状态
func (s *AdminService) UpdateAccountStatus(accountID uint, req *UpdateAccountStatusRequest) error {
	return s.accountRepo.SetActiveStatus(accountID, req.IsActive)
}

// DeleteUser 删除用户
func (s *AdminService) DeleteUser(userID, currentUserID uint) error {
	if userID == currentUserID {
		return ErrCannotDeleteSelf
	}
	return s.userRepo.Delete(userID)
}

// DeleteAccount 删除账号
func (s *AdminService) DeleteAccount(accountID uint) error {
	return s.accountRepo.Delete(accountID)
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

	accounts, err := s.accountRepo.FindActiveAccounts()
	if err != nil {
		return nil, err
	}

	totalCloud := 0
	for _, account := range accounts {
		totalCloud += account.CloudCount
	}

	return &StatsOverview{
		UserCount:    userTotal,
		AccountCount: accountTotal,
		TotalCloud:   totalCloud,
		ActiveTasks:  len(accounts),
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

		// Today's gained cloud
		summary.TodayGained = s.taskLogRepo.GetCloudGainedByAccountAndRange(acc.ID, today, tomorrow)

		// Yesterday's gained cloud
		summary.YesterdayGained = s.taskLogRepo.GetCloudGainedByAccountAndRange(acc.ID, yesterday, today)

		// Today's success/fail count
		summary.SuccessCount = s.taskLogRepo.CountByAccountStatusAndRange(acc.ID, "success", today, tomorrow)
		summary.FailedCount = s.taskLogRepo.CountByAccountStatusAndRange(acc.ID, "failed", today, tomorrow)

		// Last executed time
		lastLog := s.taskLogRepo.FindLastByAccountID(acc.ID)
		if lastLog != nil {
			summary.LastExecutedAt = lastLog.CreatedAt.Format("2006-01-02 15:04:05")
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

	// All accounts
	allAccounts, accountTotal, err := s.accountRepo.List(0, 99999)
	if err != nil {
		return nil, err
	}
	data.AccountCount = accountTotal

	// Total cloud
	totalCloud := 0
	for _, acc := range allAccounts {
		totalCloud += acc.CloudCount
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
	ranking := make([]AdminAccountRank, 0, len(allAccounts))
	for _, acc := range allAccounts {
		ownerName := ""
		if acc.User.ID > 0 {
			ownerName = acc.User.Username
		}
		todayGained := s.taskLogRepo.GetCloudGainedByAccountAndRange(acc.ID, today, tomorrow)
		ranking = append(ranking, AdminAccountRank{
			AccountID:     acc.ID,
			Phone:         acc.Phone,
			Remark:        acc.Remark,
			OwnerUsername: ownerName,
			CloudCount:    acc.CloudCount,
			TodayGained:   todayGained,
		})
	}
	// Sort by cloud_count desc
	for i := 0; i < len(ranking)-1; i++ {
		for j := i + 1; j < len(ranking); j++ {
			if ranking[i].CloudCount < ranking[j].CloudCount {
				ranking[i], ranking[j] = ranking[j], ranking[i]
			}
		}
	}
	if len(ranking) > 20 {
		ranking = ranking[:20]
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
