package services

import (
	"errors"
	"fmt"
	"time"

	"caiyun/internal/core/auth"
	"caiyun/internal/core/tasks"
)

func cocTaskResult(code string, started time.Time, err error, message, fallback string) *TaskResult {
	result := activityTaskResult(code, started, err, message, fallback)
	var issuerError *auth.SpecTokenError
	if errors.As(err, &issuerError) && issuerError.SourceID == "056002" && issuerError.TargetUnavailable() {
		result.Status = "pending"
		issuerCode := issuerError.Code
		if issuerError.ResultCode != "" {
			issuerCode += "/" + issuerError.ResultCode
		}
		if issuerError.TicketNotIssued() {
			result.Message = fmt.Sprintf("云盘未签发中国移动活动票据（%s：%s），任务未执行；请在中国移动 App 内打开对应活动核对", issuerError.SourceID, issuerCode)
		} else {
			result.Message = fmt.Sprintf("中国移动活动出票服务暂不可用（%s），任务未执行，待服务恢复后重试", issuerCode)
		}
	}
	return result
}

// runCoc3Task 执行中国移动权益市场任务（coc3）。
func (r *TaskRunner) runCoc3Task() *TaskResult {
	startTime := time.Now()
	task := tasks.NewCoc3Task(r.httpClient, r.logger).
		SetAccountContext(r.account.Phone, r.account.Auth)
	err := task.Run()
	result := cocTaskResult("coc3", startTime, err, task.Message(), "中国移动权益市场任务执行完成")
	if task.Message() != "" {
		result.Message = task.Message()
	}
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}

func (r *TaskRunner) runCoc3ScanTask() *TaskResult {
	started := time.Now()
	task := tasks.NewCoc3ScanTask(r.httpClient, r.logger).SetAccountContext(r.account.Phone, r.account.Auth)
	err := task.Run()
	return cocTaskResult("coc3_scan", started, err, task.Message(), "COC3活动只读扫描完成")
}

func (r *TaskRunner) runCocWeb6Task() *TaskResult {
	started := time.Now()
	task := tasks.NewCocWeb6Task(r.httpClient, r.logger).SetAccountContext(r.account.Phone, r.account.Auth)
	err := task.Run()
	result := cocTaskResult("coc_web6", started, err, task.Message(), "COC web6活动执行完成")
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}

func (r *TaskRunner) runActivityCollectTask() *TaskResult {
	started := time.Now()
	task := tasks.NewActivityCollectTask(r.httpClient)
	err := task.Run()
	result := activityTaskResult("activity_collect", started, err, task.Message(), "活动收藏已检查")
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}
