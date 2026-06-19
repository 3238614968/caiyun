package services

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	coretasks "caiyun/internal/core/tasks"
	"caiyun/internal/models"
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

	codes := defaultTaskCatalog.ResolveBatchCodes(configs)
	if len(codes) > 0 {
		return codes
	}

	// 兼容旧版 task_configs 表（无 run_in_batch 列）场景：
	// 此时 RunInBatch 全部为 false，回退为“按 is_enabled + sort_order 执行”。
	legacyCodes := resolveLegacyEnabledCodes(configs)
	if len(legacyCodes) > 0 {
		return legacyCodes
	}

	return defaultTaskCatalog.DefaultBatchCodes()
}

func resolveLegacyEnabledCodes(configs []*models.TaskConfig) []string {
	sorted := make([]*models.TaskConfig, 0, len(configs))
	sorted = append(sorted, configs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].SortOrder == sorted[j].SortOrder {
			return sorted[i].TaskType < sorted[j].TaskType
		}
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	result := make([]string, 0, len(sorted))
	seen := make(map[string]bool, len(sorted))
	for _, cfg := range sorted {
		if cfg == nil || !cfg.IsEnabled {
			continue
		}
		code := defaultTaskCatalog.Normalize(cfg.TaskType)
		if code == "" || seen[code] {
			continue
		}
		if _, ok := defaultTaskCatalog.Get(code); !ok {
			continue
		}
		seen[code] = true
		result = append(result, code)
	}
	return result
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
	return r.RunSelectedContext(context.Background(), taskCodes)
}

// RunSelectedContext 按指定任务编码执行任务，并在每个任务边界响应 ctx 取消。
// 单个上游 HTTP 调用仍由 HTTP 客户端自身超时兜底；ctx 到期后不会继续启动后续任务。
func (r *TaskRunner) RunSelectedContext(ctx context.Context, taskCodes []string) []TaskResult {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(taskCodes) == 0 {
		taskCodes = defaultTaskCatalog.DefaultBatchCodes()
	}

	results := make([]TaskResult, 0, len(taskCodes))
	r.initialCloudCount = r.getCurrentCloudCount()

	for _, code := range taskCodes {
		if err := ctx.Err(); err != nil {
			taskType := defaultTaskCatalog.Normalize(code)
			if taskType == "" {
				taskType = code
			}
			results = append(results, TaskResult{TaskType: taskType, Status: "failed", Message: err.Error()})
			break
		}

		normalizedCode := defaultTaskCatalog.Normalize(code)
		if normalizedCode == "" {
			results = append(results, TaskResult{TaskType: code, Status: "failed", Message: "任务编码为空"})
			continue
		}

		if r.disabledTasks != nil && r.disabledTasks[normalizedCode] {
			results = append(results, TaskResult{TaskType: normalizedCode, Status: "skipped", Message: "任务已下架"})
			continue
		}

		snapshot := r.logger.Snapshot()
		result, err := defaultTaskCatalog.Execute(r, normalizedCode)
		if err != nil {
			if result == nil {
				result = &TaskResult{TaskType: normalizedCode, Status: "failed", Message: err.Error()}
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

		if result == nil {
			result = &TaskResult{TaskType: normalizedCode, Status: "failed", Message: "任务返回为空"}
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

func (r *TaskRunner) runCloudMultipleTask() *TaskResult {
	startTime := time.Now()
	task := coretasks.NewCloudMultipleTask(r.httpClient, r.logger)
	err := task.Run()
	duration := time.Since(startTime).Milliseconds()

	result := &TaskResult{
		TaskType:      "cloud_multiple",
		ExecutionTime: int(duration),
	}
	if err != nil {
		result.Status = "failed"
		result.Message = err.Error()
		return result
	}

	result.Status = "success"
	result.Message = task.Message()
	if strings.TrimSpace(result.Message) == "" {
		result.Message = "云朵翻倍执行成功"
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
