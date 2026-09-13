package services

import (
	"fmt"
	"strings"
	"time"

	coretasks "caiyun/internal/core/tasks"
)

// activityTaskResult 统一构造活动任务的执行结果：优先使用任务自述消息，
// 失败时回退到错误信息。
func activityTaskResult(taskType string, startTime time.Time, err error, message, successFallback string) *TaskResult {
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
	result.Message = strings.TrimSpace(message)
	if result.Message == "" {
		result.Message = successFallback
	}
	return result
}

func (r *TaskRunner) activityAccountContext() (string, string, error) {
	if r.account == nil {
		return "", "", fmt.Errorf("账号上下文为空")
	}
	return r.account.Phone, r.getRawAccountToken(), nil
}

// runTokenPKTask 执行算力大作战任务。
func (r *TaskRunner) runTokenPKTask() *TaskResult {
	startTime := time.Now()
	phone, authToken, err := r.activityAccountContext()
	if err != nil {
		return activityTaskResult("token_pk", startTime, err, "", "")
	}
	task := coretasks.NewTokenPKTask(r.httpClient, r.logger)
	task.SetStorage(r.storage)
	task.SetAccountContext(phone, authToken)
	err = task.Run()
	return activityTaskResult("token_pk", startTime, err, task.Message(), "算力大作战执行成功")
}

// runMakeWishTask 执行全网许愿赢好礼任务。
func (r *TaskRunner) runMakeWishTask() *TaskResult {
	startTime := time.Now()
	phone, authToken, err := r.activityAccountContext()
	if err != nil {
		return activityTaskResult("make_wish", startTime, err, "", "")
	}
	task := coretasks.NewMakeWishTask(r.httpClient, r.logger)
	task.SetStorage(r.storage)
	task.SetAccountContext(phone, authToken)
	err = task.Run()
	return activityTaskResult("make_wish", startTime, err, task.Message(), "许愿赢好礼执行成功")
}

// runFunAITask 执行趣玩AI抽奖任务。
func (r *TaskRunner) runFunAITask() *TaskResult {
	startTime := time.Now()
	phone, authToken, err := r.activityAccountContext()
	if err != nil {
		return activityTaskResult("fun_ai", startTime, err, "", "")
	}
	task := coretasks.NewFunAITask(r.httpClient, r.logger)
	task.SetStorage(r.storage)
	task.SetAccountContext(phone, authToken)
	err = task.Run()
	return activityTaskResult("fun_ai", startTime, err, task.Message(), "趣玩AI抽奖执行成功")
}

// runPosterTask 执行校园海报·AI体验活动任务。
func (r *TaskRunner) runPosterTask() *TaskResult {
	startTime := time.Now()
	phone, authToken, err := r.activityAccountContext()
	if err != nil {
		return activityTaskResult("poster_activity", startTime, err, "", "")
	}
	task := coretasks.NewPosterTask(r.httpClient, r.logger)
	task.SetStorage(r.storage)
	task.SetAccountContext(phone, authToken)
	err = task.Run()
	return activityTaskResult("poster_activity", startTime, err, task.Message(), "校园海报活动执行成功")
}
