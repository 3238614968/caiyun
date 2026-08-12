package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/core/tasks"
	"fmt"
	"time"
)

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
