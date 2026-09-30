package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/tasks"
	"caiyun/internal/envutil"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
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
	eventHub       *ws.Hub
	mailDedup      tasks.MailSendDedupStore
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

// SetEventHub attaches the Hub owned by API or Worker assembly.
func (s *TaskService) SetEventHub(eventHub *ws.Hub) {
	if s != nil {
		s.eventHub = eventHub
	}
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
	mailPeers         []tasks.MailPeer
	mailDedup         tasks.MailSendDedupStore
	assistTokenMgr    *TokenManager
	assistAccountRepo *repository.AccountRepository
}

func (s *TaskService) SetMailDedupStore(store tasks.MailSendDedupStore) {
	s.mailDedup = store
}

// NewTaskRunner 创建任务运行器
func NewTaskRunner(account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	return buildTaskRunner(nil, account, storage, authMgr, disabledTasks, taskRunnerOptions{maxJWTRetries: 1})
}

// NewTaskRunnerWithRetry 创建任务运行器；账号停用统一由 TokenManager 判定。
func (s *TaskService) NewTaskRunnerWithRetry(account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	return buildTaskRunner(s, account, storage, authMgr, disabledTasks, taskRunnerOptions{
		maxJWTRetries: 3,
		useManagedJWT: s.tokenMgr != nil && account != nil && account.JWTToken != "",
	})
}

type taskRunnerOptions struct {
	maxJWTRetries int
	useManagedJWT bool
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
	if opts.useManagedJWT {
		client.SetJWTToken(account.JWTToken)
		return runner
	}

	// 获取 JWT token - 总是尝试获取最新的，因为传入的 account.JWTToken 可能已过期
	jwtToken := account.JWTToken
	ssoToken := ""

	for i := 0; i < opts.maxJWTRetries; i++ {
		if token, matchedSSOToken, err := authMgrForJWT.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
			jwtToken = token
			ssoToken = matchedSSOToken
			lg.Info("成功获取 JWT token")
			account.JWTToken = token
			break
		} else {
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

	if jwtToken == "" {
		lg.Error("没有可用的 JWT token，部分任务可能无法执行")
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
	return envutil.Int(key, defaultVal)
}

// GetTaskLogs 获取任务日志。
func (s *TaskService) GetTaskLogs(userID uint, accountID *uint, taskType, status string, page, pageSize int) ([]*models.TaskLog, int64, error) {
	return s.GetTaskLogsContext(context.Background(), userID, accountID, taskType, status, page, pageSize)
}

// GetTaskLogsContext propagates cancellation through ownership validation and
// the filtered log query without storing request context on the service.
func (s *TaskService) GetTaskLogsContext(ctx context.Context, userID uint, accountID *uint, taskType, status string, page, pageSize int) ([]*models.TaskLog, int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if accountID != nil {
		account, err := s.accountRepo.WithContext(ctx).FindMetadataByID(*accountID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, 0, ErrAccountNotFound
		}
		if err != nil {
			return nil, 0, err
		}
		if account.UserID != userID {
			return nil, 0, ErrAccountNotFound
		}
	}
	return s.taskLogRepo.WithContext(ctx).FindByFilter(userID, accountID, taskType, status, (page-1)*pageSize, pageSize)
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
	executed, _ := s.HasExecutedTodayForTaskTypesContext(context.Background(), accountID, taskTypes)
	return executed
}

// HasExecutedTodayForTaskTypesContext returns database errors instead of
// silently treating a failed read as "not executed".
func (s *TaskService) HasExecutedTodayForTaskTypesContext(ctx context.Context, accountID uint, taskTypes []string) (bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cstZone := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cstZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
	tomorrow := today.Add(24 * time.Hour)

	var count int64
	if err := s.taskLogRepo.WithContext(ctx).CountByAccountIDTaskTypesAndDateRangeWithError(accountID, taskTypes, today, tomorrow, &count); err != nil {
		return false, err
	}
	return count > 0, nil
}
