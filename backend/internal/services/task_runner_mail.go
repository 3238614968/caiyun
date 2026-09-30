package services

import (
	"time"

	"caiyun/internal/core/tasks"
)

func (r *TaskRunner) runMailMutualTask() *TaskResult {
	started := time.Now()
	job := tasks.NewMailMutualTask(r.httpClient, r.logger).
		SetAccount(r.account.UserID, r.account.ID, r.account.Phone, r.account.Auth).
		SetPeers(r.mailPeers).
		SetDedupStore(r.mailDedup)
	err := job.Run()
	result := taskResultFromErr("mail_mutual", started, err, job.Message())
	if result.Status == "success" && result.Message == "" {
		result.Message = "邮箱互发任务已检查"
	}
	return result
}
