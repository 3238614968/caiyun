package services

import (
	"caiyun/internal/core/tasks"
	"fmt"
	"strings"
	"time"
)

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
