package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/security/authcache"
	"context"
	"errors"
	"strings"
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
	ErrUserNotFound          = errors.New("用户不存在")
	ErrCannotDeleteSelf      = errors.New("不能删除自己")
	ErrCannotDemoteSelf      = errors.New("不能将自己的管理员角色降级")
	ErrCannotRemoveLastAdmin = errors.New("不能移除最后一个管理员")
)

// AdminService 管理员服务
type AdminService struct {
	userRepo       *repository.UserRepository
	accountRepo    *repository.AccountRepository
	taskLogRepo    *repository.TaskLogRepository
	taskConfigRepo *repository.TaskConfigRepository
	unitOfWork     repository.UnitOfWork
}

// NewAdminService 创建管理员服务
func NewAdminService(
	userRepo *repository.UserRepository,
	accountRepo *repository.AccountRepository,
	taskLogRepo *repository.TaskLogRepository,
	taskConfigRepo *repository.TaskConfigRepository,
	unitOfWorks ...repository.UnitOfWork,
) *AdminService {
	unitOfWork := repository.NewUnitOfWorkFromUserRepository(userRepo)
	if len(unitOfWorks) > 0 && unitOfWorks[0] != nil {
		unitOfWork = unitOfWorks[0]
	}
	return &AdminService{
		userRepo:       userRepo,
		accountRepo:    accountRepo,
		taskLogRepo:    taskLogRepo,
		taskConfigRepo: taskConfigRepo,
		unitOfWork:     unitOfWork,
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
func (s *AdminService) GetAllUsers(page, size int, keywords ...string) ([]*UserListItem, int64, error) {
	offset := (page - 1) * size
	keyword := ""
	if len(keywords) > 0 {
		keyword = strings.TrimSpace(keywords[0])
	}
	users, total, err := s.userRepo.Search(keyword, offset, size)
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
		username := ""
		if acc.User.ID > 0 {
			username = acc.User.Username
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
	Password string `json:"password" binding:"required,min=6"`
}

// UpdateUserRole 更新用户角色，并吊销目标用户既有会话。
func (s *AdminService) UpdateUserRole(userID, currentUserID uint, req *UpdateUserRoleRequest) error {
	user, err := s.userRepo.FindByID(userID)
	if err != nil {
		return ErrUserNotFound
	}
	if user.Role == req.Role {
		return nil
	}
	if userID == currentUserID && user.Role == "admin" && req.Role != "admin" {
		return ErrCannotDemoteSelf
	}
	if user.Role == "admin" && req.Role != "admin" {
		adminCount, err := s.userRepo.CountByRole("admin")
		if err != nil {
			return err
		}
		if adminCount <= 1 {
			return ErrCannotRemoveLastAdmin
		}
	}
	if err := s.userRepo.UpdateRoleAndRevokeSessions(user.ID, req.Role); err != nil {
		return err
	}
	// 角色变更后失效认证缓存，并通过 token_version 使旧 JWT 立即失效。
	authcache.Delete(user.ID)
	return nil
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
	if err := s.userRepo.UpdatePasswordAndRevokeSessions(user.ID, string(hashedPassword)); err != nil {
		return err
	}
	// 管理员重置密码后立即失效该用户的认证缓存。
	authcache.Delete(user.ID)
	return nil
}

// UpdateAccountStatusRequest 更新账号状态请求
type UpdateAccountStatusRequest struct {
	IsActive bool `json:"is_active"`
}

// UpdateAccountStatus 更新账号状态
func (s *AdminService) UpdateAccountStatus(accountID uint, req *UpdateAccountStatusRequest) error {
	return s.accountRepo.SetActiveStatus(accountID, req.IsActive)
}

// DeleteUser 删除用户。禁止删除自己和最后一个管理员，避免管理面锁死。
func (s *AdminService) DeleteUser(userID, currentUserID uint) error {
	return s.DeleteUserContext(context.Background(), userID, currentUserID)
}

// DeleteUserContext atomically erases dependent business data, anonymizes audit
// evidence and soft-deletes the anonymized user identity.
func (s *AdminService) DeleteUserContext(ctx context.Context, userID, currentUserID uint) error {
	if userID == currentUserID {
		return ErrCannotDeleteSelf
	}
	if s.unitOfWork == nil {
		return errors.New("unit of work is not configured")
	}
	err := s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
		user, err := repos.User.FindByID(userID)
		if err != nil {
			return ErrUserNotFound
		}
		if user.Role == "admin" {
			adminCount, err := repos.User.CountByRole("admin")
			if err != nil {
				return err
			}
			if adminCount <= 1 {
				return ErrCannotRemoveLastAdmin
			}
		}

		cleanup := []func(uint) error{
			repos.Operation.DeleteByUserID,
			repos.RefreshSession.DeleteByUserID,
			repos.ExchangeRecord.DeleteByUserID,
			repos.ExchangeTask.DeleteByUserID,
			repos.ExchangeAccount.DeleteByUserID,
			repos.TaskLog.DeleteByUserID,
			repos.CloudStats.DeleteByUserID,
			repos.Account.DeleteByUserID,
			repos.WSMessage.DeleteByUserID,
		}
		for _, erase := range cleanup {
			if err := erase(userID); err != nil {
				return err
			}
		}
		if err := repos.AuditLog.AnonymizeByUserID(userID); err != nil {
			return err
		}
		return repos.User.AnonymizeAndDelete(userID)
	})
	if err != nil {
		return err
	}
	// Cache invalidation is deliberately after commit; a rolled-back deletion
	// must not evict a still-valid user snapshot.
	authcache.Delete(userID)
	return nil
}

// DeleteAccount 删除账号。
func (s *AdminService) DeleteAccount(accountID uint) error {
	return s.DeleteAccountContext(context.Background(), accountID)
}

// DeleteAccountContext atomically removes all data derived from a cloud
// account and overwrites credentials before physically deleting the account.
func (s *AdminService) DeleteAccountContext(ctx context.Context, accountID uint) error {
	if s.unitOfWork == nil {
		return errors.New("unit of work is not configured")
	}
	return s.unitOfWork.WithinTransaction(ctx, func(repos repository.TransactionRepositories) error {
		if _, err := repos.Account.FindByID(accountID); err != nil {
			return ErrAccountNotFound
		}

		cleanup := []func(uint) error{
			repos.Operation.DeleteByAccountID,
			repos.ExchangeRecord.DeleteByAccountID,
			repos.ExchangeTask.DeleteByAccountID,
			repos.ExchangeAccount.DeleteByAccountID,
			repos.TaskLog.DeleteByAccountID,
			repos.CloudStats.DeleteByAccountID,
			repos.Account.AnonymizeAndDeleteByID,
		}
		for _, erase := range cleanup {
			if err := erase(accountID); err != nil {
				return err
			}
		}
		return nil
	})
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
