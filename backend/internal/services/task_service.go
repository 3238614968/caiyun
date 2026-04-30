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
	client := http.NewClient()
	lg := logger.NewLogger(logger.LevelInfo)

	// 检查 account.Auth 是否为空，空 auth 无法执行任何任务
	if account.Auth == "" {
		lg.Error("账号 Auth 为空，跳过认证设置")
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

	// 设置认证信息
	// account.Auth 存储的是 "Basic <base64>" 或纯 "<base64>" 格式
	// SetAuth 方法会自动移除 "Basic " 前缀，只保存 base64 部分
	// 清理 auth 中的非法字符（换行、回车、非ASCII等），防止 net/http: invalid header field value
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
	// 尝试通过 specToken → tyrzLogin 获取 JWT token
	if token, matchedSSOToken, err := authMgrForJWT.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
		jwtToken = token
		ssoToken = matchedSSOToken
		lg.Info("成功获取 JWT token")
		// 更新到 account 对象，以便后续使用
		account.JWTToken = token
	} else if err != nil {
		lg.Error("获取 JWT token 失败:", err)
		// 如果获取失败但已有旧 token，继续使用旧的
		if jwtToken == "" {
			lg.Error("没有可用的 JWT token，部分任务可能无法执行")
		}
	}
	if jwtToken != "" {
		client.SetJWTToken(jwtToken)
	}
	if ssoToken != "" {
		client.SetSSOToken(ssoToken)
	}

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

// NewTaskRunnerWithRetry 创建任务运行器（带JWT获取重试和自动禁用功能）
func (s *TaskService) NewTaskRunnerWithRetry(account *models.Account, storage tasks.Storage, authMgr *auth.Auth, disabledTasks map[string]bool) *TaskRunner {
	client := http.NewClient()
	lg := logger.NewLogger(logger.LevelInfo)

	// 检查 account.Auth 是否为空，空 auth 无法执行任何任务
	if account.Auth == "" {
		lg.Error("账号 Auth 为空，跳过认证设置")
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

	// 设置认证信息
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

	// 获取 JWT token - 带重试机制
	jwtToken := account.JWTToken
	ssoToken := ""
	var lastErr error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		if token, matchedSSOToken, err := authMgrForJWT.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
			jwtToken = token
			ssoToken = matchedSSOToken
			lg.Info("成功获取 JWT token")
			account.JWTToken = token
			// 成功获取后重置错误计数
			if account.JWTErrorCount > 0 {
				account.JWTErrorCount = 0
				s.accountRepo.Update(account)
			}
			break
		} else {
			lastErr = err
			lg.Error(fmt.Sprintf("获取 JWT token 失败 (尝试 %d/%d):", i+1, maxRetries), err)
			if i < maxRetries-1 {
				time.Sleep(time.Second * time.Duration(i+1))
			}
		}
	}

	// 如果重试3次后仍然失败
	if jwtToken == "" || lastErr != nil {
		// 增加错误计数
		account.JWTErrorCount++
		lg.Error(fmt.Sprintf("JWT获取失败次数: %d/3", account.JWTErrorCount))

		// 如果达到3次，禁用账号
		if account.JWTErrorCount >= 3 {
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
		}

		// 更新账号状态到数据库
		if err := s.accountRepo.Update(account); err != nil {
			lg.Error("更新账号JWT错误计数失败:", err)
		}
	}

	if jwtToken != "" {
		client.SetJWTToken(jwtToken)
	}
	if ssoToken != "" {
		client.SetSSOToken(ssoToken)
	}

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
func (r *TaskRunner) Run() []TaskResult {
	// 兼容旧调用方：统一转发到注册表驱动路径，避免并行维护两套任务编排逻辑。
	return r.RunSelected(defaultTaskCatalog.DefaultBatchCodes())
}

// getCurrentCloudCount 获取当前云朵总数（通过签到API）
func (r *TaskRunner) getCurrentCloudCount() int {
	cloudInfo, err := r.api.GetCloudInfo()
	if err != nil {
		return 0
	}
	if !cloudInfo.IsSuccess() {
		return 0
	}
	return cloudInfo.Result.Total
}

// GetCloudGained 获取本次执行获得的云朵数
func (r *TaskRunner) GetCloudGained() int {
	if r.finalCloudCount > r.initialCloudCount {
		return r.finalCloudCount - r.initialCloudCount
	}
	return 0
}

// GetFinalCloudCount 获取执行后的云朵总数
func (r *TaskRunner) GetFinalCloudCount() int {
	return r.finalCloudCount
}

// runSignInTask 执行签到任务
func (r *TaskRunner) runSignInTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewSignInTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "signin",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "签到任务执行成功"
	}

	return result
}

// runWeChatTask 执行微信任务
func (r *TaskRunner) runWeChatTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewWeChatTask(r.httpClient, r.logger)
	err := task.RunSignIn()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "wechat",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "微信任务执行成功"
	}

	return result
}

// runWxDrawTask 执行微信抽奖任务
func (r *TaskRunner) runWxDrawTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewWeChatTask(r.httpClient, r.logger)
	drawTimes := readTaskEnvInt("CAIYUN_TASK_WXDRAW_TIMES", 1)
	drawDelayMs := readTaskEnvInt("CAIYUN_TASK_WXDRAW_DELAY_MS", 500)
	err := task.RunDrawWithInterval(drawTimes, time.Duration(drawDelayMs)*time.Millisecond)
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "wxdraw",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "微信抽奖执行成功"
	}

	return result
}

// runShakeTask 执行摇一摇任务
func (r *TaskRunner) runShakeTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewShakeTask(r.httpClient, r.logger)
	shakeTimes := readTaskEnvInt("CAIYUN_TASK_SHAKE_TIMES", 15)
	shakeDelayMs := readTaskEnvInt("CAIYUN_TASK_SHAKE_DELAY_MS", 1000)
	err := task.RunWithConfig(shakeTimes, time.Duration(shakeDelayMs)*time.Millisecond)
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "shake",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "摇一摇任务执行成功"
	}

	return result
}

// runTodayCloudTask 执行今日云朵任务
func (r *TaskRunner) runTodayCloudTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewTodayCloudTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "todaycloud",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.CloudGained = task.TotalCloud()
		result.Message = fmt.Sprintf("今日获得%d次云朵，数量共计：%d", task.TodayCount(), task.TotalCloud())
	}

	return result
}

// runAiCloudTask 执行AI云朵任务
func (r *TaskRunner) runAiCloudTask() *TaskResult {
	startTime := time.Now()

	// 加载AI会话
	sessions, err := tasks.LoadAISessions(r.storage)
	if err != nil {
		return &TaskResult{
			TaskType:      "aicloud",
			ExecutionTime: int(time.Since(startTime).Milliseconds()),
			Status:        "failed",
			Message:       fmt.Sprintf("加载AI会话失败: %v", err),
		}
	}

	// 如果没有会话，跳过
	if len(sessions) == 0 {
		return &TaskResult{
			TaskType:      "aicloud",
			ExecutionTime: int(time.Since(startTime).Milliseconds()),
			Status:        "skipped",
			Message:       "没有AI会话，请先运行AI红包任务",
		}
	}

	userID, err := r.resolveUserID()
	if err != nil {
		return &TaskResult{
			TaskType:      "aicloud",
			ExecutionTime: int(time.Since(startTime).Milliseconds()),
			Status:        "failed",
			Message:       fmt.Sprintf("解析AI用户ID失败: %v", err),
		}
	}

	task := tasks.NewAICloudTask(r.httpClient, r.logger, r.storage, userID)
	err = task.Run(sessions)
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "aicloud",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "AI云朵任务执行成功"
	}

	return result
}

// runBlindBoxTask 执行盲盒任务
func (r *TaskRunner) runBlindBoxTask() *TaskResult {
	startTime := time.Now()

	// 执行盲盒任务
	task := tasks.NewBlindboxTask(r.httpClient, r.logger, r.storage)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "blindbox",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "盲盒任务执行成功"
	}

	return result
}

// runRedPacketTask 执行红包任务
func (r *TaskRunner) runRedPacketTask() *TaskResult {
	startTime := time.Now()

	userID, err := r.resolveUserID()
	if err != nil {
		return &TaskResult{
			TaskType:      "redpacket",
			ExecutionTime: int(time.Since(startTime).Milliseconds()),
			Status:        "failed",
			Message:       fmt.Sprintf("解析AI用户ID失败: %v", err),
		}
	}

	var sessions []tasks.AISession
	authClient := &AuthClientAdapter{
		authMgr: r.authMgr,
		phone:   r.account.Phone,
	}

	task := tasks.NewRedPacketTask(r.httpClient, r.logger, authClient, userID, &sessions)
	err = task.Run()

	if len(sessions) > 0 {
		if saveErr := tasks.SaveAISessions(r.storage, sessions); saveErr != nil {
			r.logger.Error("保存AI会话失败", saveErr)
		}
	}

	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "redpacket",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "红包任务执行成功"
	}

	return result
}

// AuthClientAdapter 适配器，将auth.Auth适配为tasks.AuthClient接口
type AuthClientAdapter struct {
	authMgr *auth.Auth
	phone   string
}

func (a *AuthClientAdapter) GetSSOToken(userID string) (string, error) {
	return a.authMgr.QuerySpecTokenForJWT(a.phone)
}

func (a *AuthClientAdapter) LoginMailWithSSO(ssoToken string) error {
	_, err := a.authMgr.LoginMail(ssoToken)
	return err
}

// runStoreTask 执行商店任务（兑换月卡）
func (r *TaskRunner) runStoreTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewExchangeMonthlyCardTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "store",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "商店任务执行成功"
	}

	return result
}

// runGardenTask 执行花园任务
func (r *TaskRunner) runGardenTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewGardenCheckinTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "garden",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "花园任务执行成功"
	}

	return result
}

// runCloudPhoneTask 执行云手机红包派对任务
func (r *TaskRunner) runCloudPhoneTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewCloudPhonePartyTask(r.httpClient, r.logger, r.storage)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "cloudphone",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "云手机红包派对任务执行成功"
	}

	return result
}

// runCloudBattleTask 执行云朵大战任务
func (r *TaskRunner) runCloudBattleTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewCloudBattleTask(r.httpClient, r.logger)
	gameTime := readTaskEnvInt("CAIYUN_TASK_CLOUDBATTLE_GAME_TIME", 30)
	err := task.RunWithGameTime(gameTime)
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "cloudbattle",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "云朵大战任务执行成功"
	}

	return result
}

// runInviteFriendsTask 执行邀请好友任务
func (r *TaskRunner) runInviteFriendsTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewInviteFriendsTask(r.httpClient, r.logger).SetPhone(r.account.Phone)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "invitefriends",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "邀请好友任务执行成功"
	}

	return result
}

// runReceiveTask 执行领取云朵任务
func (r *TaskRunner) runReceiveTask() *TaskResult {
	startTime := time.Now()

	resp, err := r.api.Receive()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "receive",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else if resp == nil || !resp.IsSuccess() {
		result.Status = "failed"
		if resp != nil && resp.MessageText() != "" {
			result.Message = resp.MessageText()
		} else {
			result.Message = "领取云朵失败"
		}
	} else {
		result.Status = "success"
		messageParts := []string{"领取云朵执行成功"}
		if payload, ok := resp.Result.(map[string]interface{}); ok {
			if total, ok := payload["total"]; ok {
				messageParts = append(messageParts, fmt.Sprintf("当前云朵%v", total))
			}
			if pendingPrizeCount, ok := payload["pendingPrizeCount"]; ok {
				messageParts = append(messageParts, fmt.Sprintf("待领奖品%v项", pendingPrizeCount))
			}
		}
		result.Message = strings.Join(messageParts, "，")
	}

	return result
}

// runMessagePushTask 执行消息推送奖励任务
func (r *TaskRunner) runMessagePushTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewMessagePushRewardTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "messagepush",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "消息推送奖励任务执行成功"
	}

	return result
}

// runRevivalRewardTask 执行复活卡奖励任务
func (r *TaskRunner) runRevivalRewardTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewRevivalRewardTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "revivalreward",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = task.Message()
		if strings.TrimSpace(result.Message) == "" {
			result.Message = "\u590d\u6d3b\u5361\u5956\u52b1\u6267\u884c\u6210\u529f"
		}
	}

	return result
}

// runBackupGiftTask 执行备份礼包任务
func (r *TaskRunner) runBackupGiftTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewBackupGiftTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "backupgift",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "备份礼包任务执行成功"
	}

	return result
}

// runTaskListTask 执行任务列表任务
func (r *TaskRunner) runTaskListTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewTaskListTask(r.httpClient, r.logger).
		SetStorage(r.storage).
		SetAccountContext(r.account.Phone, r.getRawAccountToken())
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "tasklist",
		ExecutionTime: int(duration),
	}

	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "任务列表执行成功"
	}

	return result
}

// runAfterTaskCleanup 收尾清理（删除临时上传文件和遗留分享链接）
func (r *TaskRunner) runAfterTaskCleanup() error {
	if r.storage == nil {
		return nil
	}

	var errs []string
	if err := r.cleanupTempShareLinks(); err != nil {
		errs = append(errs, err.Error())
	}
	if err := r.cleanupTempFiles(); err != nil {
		errs = append(errs, err.Error())
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

func (r *TaskRunner) cleanupTempFiles() error {
	fileIDs, err := tasks.LoadStringList(r.storage, tasks.KeyTempFiles)
	if err != nil || len(fileIDs) == 0 {
		return nil
	}

	r.logger.Debug("收尾清理临时文件", strings.Join(fileIDs, ","))
	fileAPI := api.NewFileAPI(r.httpClient)
	resp, err := fileAPI.DeleteFiles(fileIDs)
	if err != nil {
		r.logger.Error("收尾清理临时文件失败", err)
		return err
	}
	if resp != nil && resp.Success {
		_ = tasks.SaveStringList(r.storage, tasks.KeyTempFiles, nil)
		r.logger.Success("收尾清理临时文件成功")
		return nil
	}
	if resp != nil {
		err = fmt.Errorf("code=%s msg=%s", resp.Code, resp.Message)
		r.logger.Error("收尾清理临时文件失败", err)
		return err
	}
	return fmt.Errorf("删除临时文件返回为空")
}

func (r *TaskRunner) cleanupTempShareLinks() error {
	if strings.TrimSpace(r.account.Phone) == "" {
		return nil
	}

	linkIDs, err := tasks.LoadStringList(r.storage, tasks.KeyTempLinks)
	if err != nil || len(linkIDs) == 0 {
		return nil
	}

	r.logger.Debug("收尾清理分享链接", strings.Join(linkIDs, ","))
	resp, err := r.api.DelOutLink(r.account.Phone, linkIDs)
	if err != nil {
		r.logger.Error("收尾清理分享链接失败", err)
		return err
	}
	if resp != nil && resp.IsSuccess() {
		_ = tasks.SaveStringList(r.storage, tasks.KeyTempLinks, nil)
		r.logger.Success("收尾清理分享链接成功")
		return nil
	}
	err = fmt.Errorf("%v", resp)
	r.logger.Error("收尾清理分享链接失败", resp)
	return err
}

// getRawAccountToken 获取账号原始 token（优先数据库 token，其次从 Auth 解析）
func (r *TaskRunner) getRawAccountToken() string {
	if r.account == nil {
		return ""
	}
	if strings.TrimSpace(r.account.Token) != "" {
		return strings.TrimSpace(r.account.Token)
	}
	if strings.TrimSpace(r.account.Auth) == "" {
		return ""
	}

	info, err := auth.ParseToken(r.account.Auth)
	if err != nil || info == nil {
		return ""
	}
	return strings.TrimSpace(info.Token)
}

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

// ExecuteTaskForAccount 为指定账号执行所有任务
func (s *TaskService) ExecuteTaskForAccount(account *models.Account) ([]TaskResult, error) {
	// 使用 TokenManager 获取有效的 JWT Token
	if s.tokenMgr != nil {
		tokenInfo, err := s.tokenMgr.GetToken(account.ID)
		if err == nil && tokenInfo.JWTToken != "" {
			account.JWTToken = tokenInfo.JWTToken
		}
	}

	taskCodes := resolveConfiguredTaskCodes(s.taskConfigRepo)
	runner := s.NewTaskRunnerWithRetry(account, buildAccountScopedStorage(s.storage, account.ID), s.authMgr, nil)
	results := runner.RunSelected(taskCodes)

	// 计算本次获得的云朵数
	totalGained := runner.GetCloudGained()
	finalCloudCount := runner.GetFinalCloudCount()

	// 更新账号的云朵数
	if finalCloudCount > 0 {
		s.accountRepo.UpdateCloudCount(account.ID, finalCloudCount)
	}

	// 将总获得云朵数分配到 receive 任务的日志中（如果有的话）
	// 如果某个任务已经显式给出了 cloud_gained，则不再做二次分配，避免重复统计。
	gainAssigned := false
	hasExplicitGain := false
	for _, result := range results {
		if result.Status == "success" && result.CloudGained > 0 {
			hasExplicitGain = true
			break
		}
	}

	// 保存任务日志（跳过已下架的任务，不写入日志）
	for i, result := range results {
		// 已下架的任务不写入日志
		if result.Status == "skipped" {
			continue
		}

		cloudGained := result.CloudGained
		if !hasExplicitGain && !gainAssigned && totalGained > 0 {
			// 优先分配给 receive 任务
			if result.TaskType == "receive" && result.Status == "success" {
				cloudGained = totalGained
				gainAssigned = true
			}
		}

		log := &models.TaskLog{
			UserID:        account.UserID,
			AccountID:     account.ID,
			TaskType:      result.TaskType,
			Status:        result.Status,
			Message:       result.Message,
			CloudGained:   cloudGained,
			ExecutionTime: result.ExecutionTime,
		}

		if err := s.taskLogRepo.Create(log); err != nil {
			continue
		}

		// 更新 results 中的 CloudGained 以便返回
		results[i].CloudGained = cloudGained
	}

	// 如果没有分配到 receive 任务，分配到签到任务
	if !hasExplicitGain && !gainAssigned && totalGained > 0 {
		for i, result := range results {
			if result.TaskType == "signin" && result.Status == "success" {
				// 更新已保存的日志
				results[i].CloudGained = totalGained
				// 更新数据库中的记录
				s.taskLogRepo.UpdateCloudGained(account.UserID, account.ID, "signin", totalGained)
				break
			}
		}
	}

	// 通过WebSocket推送任务完成通知给用户
	hub := ws.GetHub()

	// 推送每个任务的结果（跳过已下架的任务）
	for _, result := range results {
		if result.Status == "skipped" {
			continue
		}
		hub.SendToUser(account.UserID, ws.Message{
			Type: "task_complete",
			Data: map[string]interface{}{
				"account_id":     account.ID,
				"phone":          account.Phone,
				"task_type":      result.TaskType,
				"status":         result.Status,
				"message":        result.Message,
				"cloud_gained":   result.CloudGained,
				"execution_time": result.ExecutionTime,
			},
		})
	}

	// 推送汇总信息
	hub.SendToUser(account.UserID, ws.Message{
		Type: "task_summary",
		Data: map[string]interface{}{
			"account_id":   account.ID,
			"phone":        account.Phone,
			"total_gained": totalGained,
			"cloud_count":  finalCloudCount,
			"task_count":   len(results),
			"completed_at": time.Now().Format("2006-01-02 15:04:05"),
		},
	})

	// 保存云朵统计数据（确保趋势图有数据）
	if finalCloudCount > 0 && s.cloudStatsRepo != nil {
		today := time.Now().Format("2006-01-02")
		stats := &models.CloudStats{
			UserID:     account.UserID,
			AccountID:  account.ID,
			Date:       today,
			CloudCount: finalCloudCount,
			CloudDiff:  totalGained,
		}
		s.cloudStatsRepo.UpsertByAccountIDAndDate(stats)
	}

	return results, nil
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

// HasExecutedToday 检查账号今日是否已执行过任务
func (s *TaskService) HasExecutedToday(accountID uint) bool {
	// 使用北京时间
	cstZone := time.FixedZone("CST", 8*3600)
	now := time.Now().In(cstZone)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, cstZone)
	tomorrow := today.Add(24 * time.Hour)
	var count int64
	s.taskLogRepo.CountByAccountIDAndDateRange(accountID, today, tomorrow, &count)
	return count > 0
}
