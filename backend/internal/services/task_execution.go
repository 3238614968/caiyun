package services

import (
	"fmt"
	"strings"
	"time"

	coretasks "caiyun/internal/core/tasks"
	"caiyun/internal/repository"
)

func resolveConfiguredTaskCodes(taskConfigRepo *repository.TaskConfigRepository) []string {
	if taskConfigRepo == nil {
		return defaultTaskCatalog.DefaultBatchCodes()
	}

	configs, err := taskConfigRepo.List()
	if err != nil || len(configs) == 0 {
		return defaultTaskCatalog.DefaultBatchCodes()
	}
	return defaultTaskCatalog.ResolveBatchCodes(configs)
}

func buildAccountScopedStorage(base coretasks.Storage, accountID uint) coretasks.Storage {
	if base == nil {
		return coretasks.NewMemoryStore()
	}

	store := coretasks.NewScopedStorage(base, fmt.Sprintf("account:%d", accountID))
	coretasks.ResetKeys(store, coretasks.KeyAISessions, coretasks.KeyAICloudNum, coretasks.KeyUserID)
	return store
}

func (r *TaskRunner) RunSelected(taskCodes []string) []TaskResult {
	results := make([]TaskResult, 0, len(taskCodes))
	r.initialCloudCount = r.getCurrentCloudCount()

	for _, code := range taskCodes {
		snapshot := r.logger.Snapshot()
		result, err := defaultTaskCatalog.Execute(r, code)
		if err != nil {
			if result == nil {
				result = &TaskResult{TaskType: defaultTaskCatalog.Normalize(code), Status: "failed", Message: err.Error()}
			} else {
				result.Status = "failed"
				if strings.TrimSpace(result.Message) == "" {
					result.Message = err.Error()
				}
			}
		} else if result != nil {
			if hasErrors, lastErr := r.logger.ErrorsSince(snapshot); hasErrors && (result.Status == "" || result.Status == "success") {
				result.Status = "failed"
				if lastErr == "" {
					lastErr = "任务执行过程中出现错误"
				}
				if strings.TrimSpace(result.Message) == "" || strings.Contains(result.Message, "执行成功") {
					result.Message = lastErr
				}
			}
		}
		results = append(results, *result)
	}

	r.finalCloudCount = r.getCurrentCloudCount()
	return results
}

func (r *TaskRunner) runTaskExpansionRewardTask() *TaskResult {
	startTime := time.Now()
	task := coretasks.NewTaskExpansionRewardTask(r.httpClient, r.logger).SetStorage(r.storage)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "task_expansion_reward",
		ExecutionTime: int(duration),
	}
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "备份翻倍奖励执行成功"
	}
	return result
}

func (r *TaskRunner) runAfterTaskTask() *TaskResult {
	startTime := time.Now()
	err := r.runAfterTaskCleanup()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "after_task",
		ExecutionTime: int(duration),
	}
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
	} else {
		result.Status = "success"
		result.Message = "收尾清理执行成功"
	}
	return result
}

func (r *TaskRunner) resolveUserID() (string, error) {
	if r.storage != nil {
		if userID, err := r.storage.Get(coretasks.KeyUserID); err == nil && strings.TrimSpace(userID) != "" {
			return strings.TrimSpace(userID), nil
		}
	}
	if r.account == nil {
		return "", fmt.Errorf("账号上下文为空")
	}

	rawToken := r.getRawAccountToken()
	if rawToken == "" {
		return "", fmt.Errorf("缺少账号 token，无法解析 userID")
	}

	userID, err := coretasks.ResolveUserID(r.api, rawToken, r.account.Phone)
	if err != nil {
		return "", err
	}
	if r.storage != nil {
		_ = r.storage.Set(coretasks.KeyUserID, userID)
	}
	return userID, nil
}
