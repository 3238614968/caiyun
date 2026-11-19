package services

import (
	"time"

	"caiyun/internal/core/tasks"
)

// runRedepositTask 执行云朵中心转存类任务槽（机制式跟进轮换）。
func (r *TaskRunner) runRedepositTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewRedepositTask(r.httpClient, r.logger)
	err := task.Run()
	result := activityTaskResult("redeposit", startTime, err, task.Message(), "转存任务执行完成")
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}
