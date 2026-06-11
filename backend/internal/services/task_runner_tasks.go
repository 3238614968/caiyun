package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	"caiyun/internal/core/tasks"
	"fmt"
	"strings"
	"time"
)

func (r *TaskRunner) Run() []TaskResult {
	// 兼容旧调用方：统一转发到注册表驱动路径，避免并行维护两套任务编排逻辑。
	return r.RunSelected(defaultTaskCatalog.DefaultBatchCodes())
}

func taskResultFromErr(taskType string, startTime time.Time, err error, successMessage string) *TaskResult {
	result := &TaskResult{
		TaskType:      taskType,
		ExecutionTime: int(time.Since(startTime).Milliseconds()),
	}
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		return result
	}
	result.Status = "success"
	result.Message = successMessage
	return result
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
	return taskResultFromErr("signin", startTime, err, "签到任务执行成功")
}

// runWeChatTask 执行微信任务
func (r *TaskRunner) runWeChatTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewWeChatTask(r.httpClient, r.logger)
	err := task.RunSignIn()
	return taskResultFromErr("wechat", startTime, err, "微信任务执行成功")
}

// runWxDrawTask 执行微信抽奖任务
func (r *TaskRunner) runWxDrawTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewWeChatTask(r.httpClient, r.logger)
	drawTimes := readTaskEnvInt("CAIYUN_TASK_WXDRAW_TIMES", 1)
	drawDelayMs := readTaskEnvInt("CAIYUN_TASK_WXDRAW_DELAY_MS", 500)
	err := task.RunDrawWithInterval(drawTimes, time.Duration(drawDelayMs)*time.Millisecond)
	return taskResultFromErr("wxdraw", startTime, err, "微信抽奖执行成功")
}

// runShakeTask 执行摇一摇任务
func (r *TaskRunner) runShakeTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewShakeTask(r.httpClient, r.logger)
	shakeTimes := readTaskEnvInt("CAIYUN_TASK_SHAKE_TIMES", 15)
	shakeDelayMs := readTaskEnvInt("CAIYUN_TASK_SHAKE_DELAY_MS", 1000)
	err := task.RunWithConfig(shakeTimes, time.Duration(shakeDelayMs)*time.Millisecond)
	return taskResultFromErr("shake", startTime, err, "摇一摇任务执行成功")
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
	return taskResultFromErr("aicloud", startTime, err, "AI云朵任务执行成功")
}

// runBlindBoxTask 执行盲盒任务
func (r *TaskRunner) runBlindBoxTask() *TaskResult {
	startTime := time.Now()

	// 执行盲盒任务
	task := tasks.NewBlindboxTask(r.httpClient, r.logger, r.storage)
	err := task.Run()
	return taskResultFromErr("blindbox", startTime, err, "盲盒任务执行成功")
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

	return taskResultFromErr("redpacket", startTime, err, "红包任务执行成功")
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
	return taskResultFromErr("store", startTime, err, "商店任务执行成功")
}

// runGardenTask 执行花园任务
func (r *TaskRunner) runGardenTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewGardenCheckinTask(r.httpClient, r.logger)
	err := task.Run()
	return taskResultFromErr("garden", startTime, err, "花园任务执行成功")
}

// runCloudPhoneTask 执行云手机红包派对任务
func (r *TaskRunner) runCloudPhoneTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewCloudPhonePartyTask(r.httpClient, r.logger, r.storage)
	err := task.Run()
	return taskResultFromErr("cloudphone", startTime, err, "云手机红包派对任务执行成功")
}

// runCloudBattleTask 执行云朵大战任务
func (r *TaskRunner) runCloudBattleTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewCloudBattleTask(r.httpClient, r.logger)
	gameTime := readTaskEnvInt("CAIYUN_TASK_CLOUDBATTLE_GAME_TIME", 30)
	err := task.RunWithGameTime(gameTime)
	return taskResultFromErr("cloudbattle", startTime, err, "云朵大战任务执行成功")
}

// runInviteFriendsTask 执行邀请好友任务
func (r *TaskRunner) runInviteFriendsTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewInviteFriendsTask(r.httpClient, r.logger).SetPhone(r.account.Phone)
	err := task.Run()
	return taskResultFromErr("invitefriends", startTime, err, "邀请好友任务执行成功")
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
	return taskResultFromErr("messagepush", startTime, err, "消息推送奖励任务执行成功")
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
	return taskResultFromErr("backupgift", startTime, err, "备份礼包任务执行成功")
}

// runTaskListTask 执行任务列表任务
func (r *TaskRunner) runTaskListTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewTaskListTask(r.httpClient, r.logger).
		SetStorage(r.storage).
		SetAccountContext(r.account.Phone, r.getRawAccountToken())
	err := task.Run()
	return taskResultFromErr("tasklist", startTime, err, "任务列表执行成功")
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
		return fmt.Errorf("%s", strings.Join(errs, "; "))
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
	if resp != nil {
		err = fmt.Errorf("code=%v msg=%s", resp.Code, resp.MessageText())
	} else {
		err = fmt.Errorf("删除分享链接返回为空")
	}
	r.logger.Error("收尾清理分享链接失败", err)
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
