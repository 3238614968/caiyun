package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"strings"
	"time"
)

// ExecuteExchangeTask 执行抢兑任务 (立即执行)
func (s *ExchangeService) ExecuteExchangeTask(taskID uint, userID uint) error {
	return s.ExecuteExchangeTaskContext(context.Background(), taskID, userID)
}

// ExecuteExchangeTaskContext executes a task while propagating cancellation to
// database lookups, rate-limit waits, retry delays and upstream HTTP requests.
func (s *ExchangeService) ExecuteExchangeTaskContext(ctx context.Context, taskID uint, userID uint) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	task, err := s.exchangeTaskRepo.WithContext(ctx).GetByID(taskID)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return fmt.Errorf("任务不存在")
	}
	if task.UserID != userID {
		return fmt.Errorf("无权操作该任务")
	}

	s.executeSingleTaskSafelyContext(ctx, task)
	return ctx.Err()
}

// BatchExecuteResult 批量执行结果
type BatchExecuteResult struct {
	TaskID  uint   `json:"task_id"`
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// BatchExecuteExchangeTasks 批量执行抢兑任务（使用工作池模式优化）
func (s *ExchangeService) BatchExecuteExchangeTasks(taskIDs []uint, userID uint) []BatchExecuteResult {
	return s.BatchExecuteExchangeTasksContext(context.Background(), taskIDs, userID)
}

// BatchExecuteExchangeTasksContext executes a bounded batch and propagates
// cancellation independently to every worker.
func (s *ExchangeService) BatchExecuteExchangeTasksContext(ctx context.Context, taskIDs []uint, userID uint) []BatchExecuteResult {
	if ctx == nil {
		ctx = context.Background()
	}
	results := make([]BatchExecuteResult, len(taskIDs))

	// 获取并发配置
	concurrency := 5
	if config, err := s.GetExchangeConcurrency(); err == nil && config > 0 {
		concurrency = config
	}

	// 使用工作池模式控制并发
	executor := utils.NewConcurrentExecutor(concurrency)

	for i, taskID := range taskIDs {
		index := i // 捕获索引
		id := taskID

		executor.Execute(func() {
			result := BatchExecuteResult{TaskID: id}
			if err := ctx.Err(); err != nil {
				result.Success = false
				result.Message = err.Error()
				results[index] = result
				return
			}

			// 验证任务归属
			task, err := s.exchangeTaskRepo.WithContext(ctx).GetByID(id)
			if err != nil {
				result.Success = false
				if ctxErr := ctx.Err(); ctxErr != nil {
					result.Message = ctxErr.Error()
				} else {
					result.Message = "任务不存在"
				}
				results[index] = result
				return
			}

			if task.UserID != userID {
				result.Success = false
				result.Message = "无权操作该任务"
				results[index] = result
				return
			}

			// 执行任务
			s.executeSingleTaskSafelyContext(ctx, task)
			if err := ctx.Err(); err != nil {
				result.Success = false
				result.Message = err.Error()
			} else {
				result.Success = true
				result.Message = "任务执行完成"
			}
			results[index] = result
		})
	}

	executor.Wait()
	return results
}

// executeSingleTaskSafely 为抢兑执行提供 panic 兜底，避免异常导致进程退出或任务永久停在 running。
func (s *ExchangeService) executeSingleTaskSafely(task *models.ExchangeTask) {
	s.executeSingleTaskSafelyContext(context.Background(), task)
}

func (s *ExchangeService) executeSingleTaskSafelyContext(ctx context.Context, task *models.ExchangeTask) {
	defer func() {
		if r := recover(); r != nil {
			taskID := uint(0)
			if task != nil {
				taskID = task.ID
			}
			message := fmt.Sprintf("任务执行异常，已自动标记失败: %v", r)
			log.Printf("【抢兑任务】任务 %d 执行 panic: %v", taskID, r)
			if taskID > 0 {
				s.updateExchangeTaskLastResult(taskID, message)
				s.updateExchangeTaskStatus(taskID, string(models.ExchangeTaskFailed))
			}
		}
	}()
	s.executeSingleTaskContext(ctx, task)
}

// executeSingleTask 执行单个抢兑任务（带重试机制）
func (s *ExchangeService) executeSingleTask(task *models.ExchangeTask) {
	s.executeSingleTaskContext(context.Background(), task)
}

func (s *ExchangeService) executeSingleTaskContext(ctx context.Context, task *models.ExchangeTask) {
	if ctx == nil {
		ctx = context.Background()
	}
	if task == nil || task.ID == 0 || ctx.Err() != nil {
		return
	}
	started, err := s.exchangeTaskRepo.WithContext(ctx).TryMarkRunning(task.ID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 抢占执行权失败: %v", task.ID, err)
		return
	}
	if !started {
		log.Printf("【抢兑任务】任务 %d 已被其他进程执行或状态不可运行，跳过", task.ID)
		return
	}
	finished := false
	defer func() {
		if finished || ctx.Err() == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		released, releaseErr := s.exchangeTaskRepo.WithContext(cleanupCtx).ReleaseRunning(task.ID, "任务因执行上下文取消，已恢复为待执行")
		if releaseErr != nil {
			log.Printf("【抢兑任务】取消后恢复任务失败: task_id=%d err=%v", task.ID, releaseErr)
		} else if released {
			log.Printf("【抢兑任务】任务 %d 因上下文取消已恢复为 pending", task.ID)
		}
	}()
	if ctx.Err() != nil {
		return
	}

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		s.updateExchangeTaskLastResult(task.ID, reason)
		s.updateExchangeTaskStatus(task.ID, string(models.ExchangeTaskPending))
		log.Printf("【抢兑月度保护】任务 %d 跳过执行: %s", task.ID, reason)
		s.hub.SendToUser(task.UserID, ws.Message{
			Type: "exchange_skipped",
			Data: map[string]interface{}{
				"task_id":      task.ID,
				"account_name": exchangeAccountName(&task.ExchangeAccount),
				"product_name": task.PrizeName,
				"success":      false,
				"message":      reason,
			},
		})
		finished = true
		return
	}

	// 获取兑换账号
	account, err := s.exchangeAccountRepo.WithContext(ctx).GetByID(task.ExchangeAccountID)
	if err != nil {
		finished = s.persistExchangeOutcome(ctx, task, false, "获取兑换账号失败", 0, models.ExchangeTaskFailed) == nil
		return
	}

	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}
	maskedAccountName := maskExchangeAccountName(accountName)

	if s.accountRepo != nil && account.AccountID > 0 {
		cloudAccount, err := s.accountRepo.WithContext(ctx).GetByID(account.AccountID)
		if err != nil {
			finished = s.persistExchangeOutcome(ctx, task, false, "云盘账号不存在或已删除", 0, models.ExchangeTaskFailed) == nil
			return
		}
		if !cloudAccount.IsActive {
			finished = s.persistExchangeOutcome(ctx, task, false, "云盘账号已失效，请重新登录后再启用任务", 0, models.ExchangeTaskPending) == nil
			return
		}
		if cloudAccount.Auth == "" {
			finished = s.persistExchangeOutcome(ctx, task, false, "云盘账号认证为空，请重新登录", 0, models.ExchangeTaskPending) == nil
			return
		}
	}

	log.Printf("【抢兑任务】开始执行任务 %d，账号: %s，商品: %s", task.ID, maskedAccountName, task.PrizeName)

	// 执行抢兑（带重试）
	maxRetries := task.MaxRetries
	if maxRetries <= 0 {
		maxRetries = constants.DefaultMaxRetries // 使用常量：默认重试3次
	}

	var success bool
	var message string
	var execTime int
	attemptsUsed := 0
	releaseSeriesLockOnFailure := func() {}

	if locked, release, reason := s.acquireMonthlySeriesLock(task, time.Now()); !locked {
		success, message, execTime = false, reason, 0
	} else {
		releaseSeriesLockOnFailure = release

		for attempt := 0; attempt <= maxRetries; attempt++ {
			attemptsUsed = attempt
			if ctx.Err() != nil {
				return
			}
			if attempt > 0 {
				log.Printf("【抢兑任务】任务 %d 第 %d 次重试...", task.ID, attempt)
				// 更新重试次数
				now := time.Now()
				s.updateExchangeTaskRetryCount(task.ID, attempt, &now)
				// 重试间隔：指数退避
				if err := sleepExchangeContext(ctx, time.Duration(attempt*2)*time.Second); err != nil {
					return
				}
			}

			prizeID := s.resolveTaskPrizeID(task)
			if !isUsableExchangePrizeID(prizeID) {
				success, message, execTime = false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0
			} else {
				success, message, execTime = s.doExchangeContext(ctx, account, prizeID)
			}

			// 如果成功，或者错误不需要重试，则退出循环
			if success || !s.shouldRetry(message) {
				break
			}
		}
	}

	// Derive the final state before persistence so the immutable result record,
	// attempt counters and task status commit as one database transaction.
	finalStatus := models.ExchangeTaskPending
	if success {
		log.Printf("【抢兑任务】任务 %d 执行成功，账号: %s", task.ID, maskedAccountName)
		if isSingleRunExchangeTask(task.TaskType) && task.MaxAttempts > 0 && task.AttemptedCount+1 >= task.MaxAttempts {
			finalStatus = models.ExchangeTaskCompleted
		}
	} else {
		log.Printf("【抢兑任务】任务 %d 执行失败，账号: %s，原因: %s", task.ID, maskedAccountName, message)
		switch {
		case strings.Contains(message, "奖品单日已耗尽"),
			strings.Contains(message, "奖品已兑完"),
			strings.Contains(message, "商品已下架或不存在"),
			strings.Contains(message, "商品ID不是可兑换 prizeId"):
			finalStatus = models.ExchangeTaskCompleted
		case attemptsUsed >= maxRetries:
			finalStatus = models.ExchangeTaskFailed
			log.Printf("【抢兑任务】任务 %d 重试次数已用尽，标记为失败", task.ID)
		}
	}

	if err := s.persistExchangeOutcome(ctx, task, success, message, execTime, finalStatus); err != nil {
		return
	}
	finished = true
	if !success {
		releaseSeriesLockOnFailure()
	}

	// 推送 WebSocket 通知
	s.hub.SendToUser(task.UserID, ws.Message{
		Type: "exchange_complete",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"account_name": accountName,
			"product_name": task.PrizeName,
			"success":      success,
			"message":      message,
			"retry_count":  task.RetryCount,
		},
	})
}

func (s *ExchangeService) updateExchangeTaskStatus(taskID uint, status string) {
	if err := s.exchangeTaskRepo.UpdateStatus(taskID, status); err != nil {
		log.Printf("【抢兑任务】更新任务状态失败: task_id=%d status=%s err=%v", taskID, status, err)
	}
}

func (s *ExchangeService) updateExchangeTaskRetryCount(taskID uint, retryCount int, lastRetryAt *time.Time) {
	if err := s.exchangeTaskRepo.UpdateRetryCount(taskID, retryCount, lastRetryAt); err != nil {
		log.Printf("【抢兑任务】更新任务重试次数失败: task_id=%d retry=%d err=%v", taskID, retryCount, err)
	}
}

func (s *ExchangeService) updateExchangeTaskLastResult(taskID uint, result string) {
	if err := s.exchangeTaskRepo.UpdateLastResult(taskID, result); err != nil {
		log.Printf("【抢兑任务】更新任务最后结果失败: task_id=%d err=%v", taskID, err)
	}
}

// shouldRetry 判断是否需要重试（使用公共函数）
func (s *ExchangeService) shouldRetry(message string) bool {
	if strings.TrimSpace(message) == "" {
		return true
	}
	return isExchangeRetryableFailure(message)
}

// doExchange 执行兑换请求
func (s *ExchangeService) doExchange(account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	return performExchangeWithControls(account, prizeID, s.tokenMgr, s.lockStore)
}

func (s *ExchangeService) doExchangeContext(ctx context.Context, account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	return performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.lockStore)
}

func (s *ExchangeService) resolveTaskPrizeID(task *models.ExchangeTask) string {
	prizeID := taskExchangePrizeID(task)
	if isUsableExchangePrizeID(prizeID) {
		return prizeID
	}
	if s.productRepo == nil || task == nil || task.PrizeName == "" {
		return prizeID
	}
	product, err := s.productRepo.FindExchangeableReplacement(task.PrizeName, task.PrizeID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 查询商品替换失败: %v", task.ID, err)
		return prizeID
	}
	if product == nil || !isUsableExchangePrizeID(product.PrizeID) {
		return prizeID
	}
	if task.PrizeID != product.PrizeID || task.ProductID != product.ID {
		task.PrizeID = product.PrizeID
		task.ProductID = product.ID
		task.Product = *product
		if err := s.exchangeTaskRepo.UpdatePrizeSnapshot(task.ID, product.ID, product.PrizeID, product.PrizeName); err != nil {
			log.Printf("【抢兑任务】任务 %d 更新商品快照失败: %v", task.ID, err)
		}
		log.Printf("【抢兑任务】任务 %d 已自动修正商品ID为 %s，避免使用历史 memo 导致 404", task.ID, product.PrizeID)
	}
	return product.PrizeID
}

// persistExchangeOutcome atomically creates the immutable result record and
// advances the task counters/final status. A partial write is never exposed:
// any failure rolls back all three mutations and leaves the running task
// recoverable by the Worker lease/retry path.
func (s *ExchangeService) persistExchangeOutcome(ctx context.Context, task *models.ExchangeTask, success bool, message string, execTimeMs int, status models.ExchangeTaskStatus) error {
	if task == nil || task.ID == 0 {
		return fmt.Errorf("refuse to persist empty exchange task")
	}
	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            string(models.ExchangeRecordSuccess),
		Message:           message,
		ExecutionTimeMs:   execTimeMs,
	}
	if !success {
		record.Status = string(models.ExchangeRecordFailed)
	}

	err := s.withinTransaction(ctx, func(txService *ExchangeService) error {
		if err := txService.exchangeRecordRepo.Create(record); err != nil {
			return fmt.Errorf("create exchange record: %w", err)
		}
		if err := txService.exchangeTaskRepo.UpdateAttempt(task.ID, success, message); err != nil {
			return fmt.Errorf("update exchange attempt: %w", err)
		}
		if err := txService.exchangeTaskRepo.UpdateStatus(task.ID, string(status)); err != nil {
			return fmt.Errorf("update exchange status: %w", err)
		}
		return nil
	})
	if err != nil {
		log.Printf("【抢兑任务】原子保存结果失败: task_id=%d status=%s err=%v", task.ID, status, err)
		return err
	}

	task.AttemptedCount++
	task.LastResult = message
	task.Status = string(status)
	if success {
		task.SuccessCount++
	} else {
		task.FailCount++
	}
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTimeMs,
	)
	return nil
}

// recordExchangeResult 记录抢兑结果
func (s *ExchangeService) recordExchangeResult(task *models.ExchangeTask, success bool, message string, execTimeMs int) {
	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            string(models.ExchangeRecordSuccess),
		Message:           message,
		ExecutionTimeMs:   execTimeMs,
	}

	if !success {
		record.Status = string(models.ExchangeRecordFailed)
	}

	if err := s.exchangeRecordRepo.Create(record); err != nil {
		log.Printf("【抢兑任务】创建兑换记录失败: task_id=%d account_id=%d prize=%s err=%v", task.ID, task.ExchangeAccountID, task.PrizeName, err)
	}
	if err := s.exchangeTaskRepo.UpdateAttempt(task.ID, success, message); err != nil {
		log.Printf("【抢兑任务】更新任务尝试结果失败: task_id=%d success=%t err=%v", task.ID, success, err)
	}
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTimeMs,
	)
}
