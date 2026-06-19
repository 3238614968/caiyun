package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/tasks"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// TaskResult 任务执行结果
type TaskResult struct {
	TaskType      string // 任务类型: signin, wechat, shake等
	Status        string // success, failed, pending
	Message       string // 执行结果/错误信息
	CloudGained   int    // 获得云朵数
	ExecutionTime int    // 执行时长(毫秒)
}

type TaskService struct {
	accountRepo    *repository.AccountRepository
	taskLogRepo    *repository.TaskLogRepository
	cloudStatsRepo *repository.CloudStatsRepository
	storage        tasks.Storage
	authMgr        *auth.Auth
	taskConfigRepo *repository.TaskConfigRepository
	tokenMgr       *TokenManager
}

func NewTaskService(
	accountRepo *repository.AccountRepository,
	taskLogRepo *repository.TaskLogRepository,
	storage tasks.Storage,
	authMgr *auth.Auth,
	taskConfigRepo *repository.TaskConfigRepository,
	cloudStatsRepo ...*repository.CloudStatsRepository,
) *TaskService {
	svc := &TaskService{
		accountRepo:    accountRepo,
		taskLogRepo:    taskLogRepo,
		storage:        storage,
		authMgr:        authMgr,
		taskConfigRepo: taskConfigRepo,
	}
	if len(cloudStatsRepo) > 0 {
		svc.cloudStatsRepo = cloudStatsRepo[0]
	}
	return svc
}

// SetTokenManager 设置 TokenManager
func (s *TaskService) SetTokenManager(tokenMgr *TokenManager) {
	s.tokenMgr = tokenMgr
}

// TaskRunner 任务运行器
type TaskRunner struct {
	account           *models.Account
	httpClient        *http.Client
	logger            *logger.Logger
	api               *api.CaiyunAPI
	startTime         time.Time
	storage           tasks.Storage
	authMgr           *auth.Auth
	initialCloudCount int             // 任务执行前的云朵数
	finalCloudCount   int             // 任务执行后的云朵数
	disabledTasks     map[string]bool // 被下架的任务类型
}

// NewTaskRunner 创建任务运行器
func NewTaskRunner(account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	return buildTaskRunner(nil, account, storage, authMgr, disabledTasks, taskRunnerOptions{maxJWTRetries: 1})
}

// NewTaskRunnerWithRetry 创建任务运行器（带JWT获取重试和自动禁用功能）
func (s *TaskService) NewTaskRunnerWithRetry(account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	return buildTaskRunner(s, account, storage, authMgr, disabledTasks, taskRunnerOptions{
		maxJWTRetries:      3,
		updateJWTErrorStat: true,
	})
}

type taskRunnerOptions struct {
	maxJWTRetries      int
	updateJWTErrorStat bool
}

func buildTaskRunner(svc *TaskService, account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool, opts taskRunnerOptions) *TaskRunner {
	client := http.NewClient()
	lg := logger.NewLogger(logger.LevelInfo)
	runner := newTaskRunner(account, client, lg, storage, authMgr, disabledTasks)
	if opts.maxJWTRetries <= 0 {
		opts.maxJWTRetries = 1
	}
	if account == nil {
		lg.Error("账号为空，跳过认证设置")
		return runner
	}

	// 检查 account.Auth 是否为空，空 auth 无法执行任何任务
	if account.Auth == "" {
		lg.Error("账号 Auth 为空，跳过认证设置")
		return runner
	}

	// 设置认证信息
	// account.Auth 存储的是 "Basic <base64>" 或纯 "<base64>" 格式。
	// SetAuth 方法会自动移除 "Basic " 前缀，只保存 base64 部分。
	// 清理 auth 中的非法字符（换行、回车、非ASCII等），防止 net/http: invalid header field value。
	client.SetMarketAccount(account.Phone)
	authStr := sanitizeHeaderValue(account.Auth)
	if authStr != "" {
		client.SetAuth(authStr)
	}

	// 创建 auth 管理器的 HTTP 客户端（用于获取 JWT token）
	authClient := http.NewClient()
	if authStr != "" {
		authClient.SetAuth(authStr)
	}
	authMgrForJWT := auth.NewAuth(authClient)

	// 获取 JWT token - 总是尝试获取最新的，因为传入的 account.JWTToken 可能已过期
	jwtToken := account.JWTToken
	ssoToken := ""
	var lastErr error

	for i := 0; i < opts.maxJWTRetries; i++ {
		if token, matchedSSOToken, err := authMgrForJWT.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
			jwtToken = token
			ssoToken = matchedSSOToken
			lastErr = nil
			lg.Info("成功获取 JWT token")
			account.JWTToken = token
			// 成功获取后重置错误计数
			if opts.updateJWTErrorStat && svc != nil && account.JWTErrorCount > 0 {
				account.JWTErrorCount = 0
				if err := svc.accountRepo.ResetJWTErrorCount(account.ID); err != nil {
					lg.Error("重置账号JWT错误计数失败:", err)
				}
			}
			break
		} else {
			lastErr = err
			if opts.maxJWTRetries > 1 {
				lg.Error(fmt.Sprintf("获取 JWT token 失败 (尝试 %d/%d):", i+1, opts.maxJWTRetries), err)
			} else {
				lg.Error("获取 JWT token 失败:", err)
			}
			if i < opts.maxJWTRetries-1 {
				time.Sleep(time.Second * time.Duration(i+1))
			}
		}
	}

	if !opts.updateJWTErrorStat && jwtToken == "" {
		lg.Error("没有可用的 JWT token，部分任务可能无法执行")
	}

	// 如果重试后仍然失败
	if opts.updateJWTErrorStat && svc != nil && (jwtToken == "" || lastErr != nil) {
		// 增加错误计数
		newCount, err := svc.accountRepo.IncrementJWTErrorCount(account.ID)
		if err != nil {
			lg.Error("更新账号JWT错误计数失败:", err)
			newCount = account.JWTErrorCount + 1
		}
		account.JWTErrorCount = newCount
		lg.Error(fmt.Sprintf("JWT获取失败次数: %d/%d", account.JWTErrorCount, opts.maxJWTRetries))

		// 如果达到最大重试次数，禁用账号
		if account.JWTErrorCount >= opts.maxJWTRetries {
			account.IsActive = false
			lg.Error(fmt.Sprintf("账号 %s JWT获取失败超过3次，已自动禁用", account.Phone))
			// 发送WebSocket通知
			if wsHub := ws.GetHub(); wsHub != nil {
				wsHub.SendToUser(account.UserID, ws.Message{
					Type: "account_disabled",
					Data: map[string]interface{}{
						"account_id": account.ID,
						"phone":      account.Phone,
						"reason":     "JWT获取失败超过3次",
					},
				})
			}
			if err := svc.accountRepo.SetActiveStatus(account.ID, false); err != nil {
				lg.Error("禁用账号失败:", err)
			}
		}
	}

	if jwtToken != "" {
		client.SetJWTToken(jwtToken)
	}
	if ssoToken != "" {
		client.SetSSOToken(ssoToken)
	}

	return runner
}

func newTaskRunner(account *models.Account, client *http.Client, lg *logger.Logger, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	return &TaskRunner{
		account:       account,
		httpClient:    client,
		logger:        lg,
		api:           api.NewCaiyunAPI(client),
		startTime:     time.Now(),
		storage:       storage,
		authMgr:       authMgr,
		disabledTasks: disabledTasks,
	}
}

// Run 执行所有任务
// sanitizeHeaderValue 清理 HTTP Header 值中的非法字符
// Go 的 net/http 不允许 header value 包含 \r \n 以及非 ASCII 可打印字符
func sanitizeHeaderValue(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, c := range s {
		// 只保留 ASCII 可打印字符和空格（0x20-0x7E）
		if c >= 0x20 && c <= 0x7E {
			b.WriteRune(c)
		}
		// 换行、回车、tab、非ASCII 全部丢弃
	}
	return strings.TrimSpace(b.String())
}

// readTaskEnvInt 读取任务相关整数环境变量，解析失败时回退默认值
func readTaskEnvInt(key string, defaultVal int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultVal
	}
	val, err := strconv.Atoi(raw)
	if err != nil {
		return defaultVal
	}
	return val
}

// GetTaskLogs 获取任务日志
func (s *TaskService) GetTaskLogs(userID uint, accountID *uint, page, pageSize int) ([]*models.TaskLog, int64, error) {
	if accountID != nil {
		account, err := s.accountRepo.FindByID(*accountID)
		if err != nil || account.UserID != userID {
			return nil, 0, fmt.Errorf("账号不存在")
		}
		return s.taskLogRepo.FindByAccountID(*accountID, (page-1)*pageSize, pageSize)
	}
	return s.taskLogRepo.FindByUserID(userID, (page-1)*pageSize, pageSize)
}

// DailyTaskTypes 返回当前配置下参与“每日已执行”判断的日常任务类型。
func (s *TaskService) DailyTaskTypes() []string {
	return resolveConfiguredTaskCodes(s.taskConfigRepo)
}

// HasExecutedToday 检查账号今日是否已执行过日常任务。
func (s *TaskService) HasExecutedToday(accountID uint) bool {
	return s.HasExecutedTodayForTaskTypes(accountID, s.DailyTaskTypes())
}

// HasExecutedTodayForTaskTypes 检查账号今日是否已执行过指定日常任务类型。
// 只统计当前日常任务注册表中的任务类型，避免兑换、健康检查等系统日志误判为“今日已执行”。
func (s *TaskService) HasExecutedTodayForTaskTypes(accountID uint, taskTypes []string) bool {
	// 使用北京时间
	cstZone := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cstZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
	tomorrow := today.Add(24 * time.Hour)

	var count int64
	s.taskLogRepo.CountByAccountIDTaskTypesAndDateRange(accountID, taskTypes, today, tomorrow, &count)
	return count > 0
}
