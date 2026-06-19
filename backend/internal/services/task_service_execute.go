package services

import (
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ExecuteTaskForAccount 为指定账号执行所有已配置批量任务。
func (s *TaskService) ExecuteTaskForAccount(account *models.Account) ([]TaskResult, error) {
	return s.ExecuteTaskForAccountContext(context.Background(), account)
}

// ExecuteTaskForAccountContext 为指定账号执行所有已配置批量任务，并在任务边界响应取消。
func (s *TaskService) ExecuteTaskForAccountContext(ctx context.Context, account *models.Account) ([]TaskResult, error) {
	return s.executeTaskCodesForAccount(ctx, account, resolveConfiguredTaskCodes(s.taskConfigRepo))
}

// ExecuteSelectedTaskForAccount 为队列消息执行指定任务类型。
// taskType 为 all/all_tasks/空时保持历史行为，执行完整批量任务。
func (s *TaskService) ExecuteSelectedTaskForAccount(account *models.Account, taskType string) ([]TaskResult, error) {
	return s.ExecuteSelectedTaskForAccountContext(context.Background(), account, taskType)
}

// ExecuteSelectedTaskForAccountContext 为队列消息执行指定任务类型，并在任务边界响应取消。
func (s *TaskService) ExecuteSelectedTaskForAccountContext(ctx context.Context, account *models.Account, taskType string) ([]TaskResult, error) {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" || taskType == "all" || taskType == "all_tasks" {
		return s.ExecuteTaskForAccountContext(ctx, account)
	}

	code := defaultTaskCatalog.Normalize(taskType)
	if code == "" {
		return nil, fmt.Errorf("任务类型为空")
	}
	if _, ok := defaultTaskCatalog.Get(code); !ok {
		return nil, fmt.Errorf("未知任务类型: %s", taskType)
	}
	return s.executeTaskCodesForAccount(ctx, account, []string{code})
}

func (s *TaskService) executeTaskCodesForAccount(ctx context.Context, account *models.Account, taskCodes []string) ([]TaskResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// 使用 TokenManager 获取有效的 JWT Token
	if s.tokenMgr != nil {
		tokenInfo, err := s.tokenMgr.GetToken(account.ID)
		if err == nil && tokenInfo.JWTToken != "" {
			account.JWTToken = tokenInfo.JWTToken
		}
	}

	runner := s.NewTaskRunnerWithRetry(account, buildAccountScopedStorage(s.storage, account.ID), s.authMgr, nil)
	results := runner.RunSelectedContext(ctx, taskCodes)

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
	var persistenceErrors []string
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

		logEntry := &models.TaskLog{
			UserID:        account.UserID,
			AccountID:     account.ID,
			TaskType:      result.TaskType,
			Status:        result.Status,
			Message:       result.Message,
			CloudGained:   cloudGained,
			ExecutionTime: result.ExecutionTime,
		}

		if err := s.taskLogRepo.Create(logEntry); err != nil {
			log.Printf("保存任务日志失败: account_id=%d task_type=%s: %v", account.ID, result.TaskType, err)
			persistenceErrors = append(persistenceErrors, fmt.Sprintf("task_log:%s:%v", result.TaskType, err))
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
				if err := s.taskLogRepo.UpdateCloudGained(account.UserID, account.ID, "signin", totalGained); err != nil {
					log.Printf("更新签到任务云朵数失败: account_id=%d: %v", account.ID, err)
					persistenceErrors = append(persistenceErrors, fmt.Sprintf("task_log_update:signin:%v", err))
				}
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
		if err := s.cloudStatsRepo.UpsertByAccountIDAndDate(stats); err != nil {
			log.Printf("保存云朵统计失败: account_id=%d date=%s: %v", account.ID, today, err)
			persistenceErrors = append(persistenceErrors, fmt.Sprintf("cloud_stats:%v", err))
		}
	}

	if len(persistenceErrors) > 0 {
		return results, fmt.Errorf("任务执行完成但部分结果持久化失败: %s", strings.Join(persistenceErrors, "; "))
	}
	if err := ctx.Err(); err != nil {
		return results, err
	}
	return results, nil
}
