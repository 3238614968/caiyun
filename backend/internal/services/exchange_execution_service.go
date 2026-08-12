package services

import (
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
		return ErrExchangeTaskNotFound
	}
	if task.UserID != userID {
		return ErrExchangePermissionDenied
	}

	return s.executeSingleTaskSafelyContext(ctx, task)
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

	concurrency := 5
	if config, err := s.GetExchangeConcurrency(); err == nil && config > 0 {
		concurrency = config
	}

	executor := utils.NewConcurrentExecutor(concurrency)
	for i, taskID := range taskIDs {
		index := i
		id := taskID
		executor.Execute(func() {
			result := BatchExecuteResult{TaskID: id}
			if err := ctx.Err(); err != nil {
				result.Message = "任务已取消"
				results[index] = result
				return
			}

			task, err := s.exchangeTaskRepo.WithContext(ctx).GetByID(id)
			if err != nil {
				if ctxErr := ctx.Err(); ctxErr != nil {
					result.Message = "任务已取消"
				} else {
					result.Message = exchangeErrorPublicMessage(ErrExchangeTaskNotFound)
				}
				results[index] = result
				return
			}
			if task.UserID != userID {
				result.Message = exchangeErrorPublicMessage(ErrExchangePermissionDenied)
				results[index] = result
				return
			}

			if err := s.executeSingleTaskSafelyContext(ctx, task); err != nil {
				result.Message = exchangeErrorPublicMessage(err)
			} else {
				result.Success = true
				result.Message = "任务执行成功"
			}
			results[index] = result
		})
	}

	executor.Wait()
	return results
}

// executeSingleTaskSafely 为抢兑执行提供 panic 兜底，避免异常导致进程退出或任务永久停在 running。
func (s *ExchangeService) executeSingleTaskSafely(task *models.ExchangeTask) {
	_ = s.executeSingleTaskSafelyContext(context.Background(), task)
}

func (s *ExchangeService) executeSingleTaskSafelyContext(ctx context.Context, task *models.ExchangeTask) (execErr error) {
	defer func() {
		if r := recover(); r != nil {
			taskID := uint(0)
			if task != nil {
				taskID = task.ID
			}
			message := fmt.Sprintf("任务执行异常，已自动标记失败: %v", r)
			log.Printf("【抢兑任务】任务 %d 执行 panic: %v", taskID, r)
			// This outer recovery only covers failures before executeSingleTaskContext
			// has acquired an execution token. Do not issue an unowned status write:
			// a replacement Worker may already own the task.
			execErr = fmt.Errorf("%w: %s", ErrExchangeExecutionFailed, message)
		}
	}()
	return s.executeSingleTaskContext(ctx, task)
}

// executeSingleTask 执行单个抢兑任务（带重试机制）
func (s *ExchangeService) executeSingleTask(task *models.ExchangeTask) {
	_ = s.executeSingleTaskContext(context.Background(), task)
}

func (s *ExchangeService) executeSingleTaskContext(ctx context.Context, task *models.ExchangeTask) (execErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if task == nil || task.ID == 0 {
		return ErrExchangeInvalidInput
	}
	started, executionToken, err := s.exchangeTaskRepo.WithContext(ctx).TryMarkRunning(task.ID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 抢占执行权失败: %v", task.ID, err)
		return fmt.Errorf("抢占任务执行权: %w", err)
	}
	if !started {
		latest, latestErr := s.exchangeTaskRepo.WithContext(ctx).GetByID(task.ID)
		if latestErr == nil && latest != nil {
			if latest.Status == string(models.ExchangeTaskRunning) && time.Since(latest.UpdatedAt) > exchangeTaskRunningTimeoutFromEnv() {
				released, releaseErr := s.exchangeTaskRepo.WithContext(ctx).ReleaseRunning(task.ID, latest.ExecutionToken, "任务执行超时，手动执行前已恢复为待执行")
				if releaseErr != nil {
					log.Printf("【抢兑任务】恢复超时 running 任务失败: task_id=%d err=%v", task.ID, releaseErr)
				} else if released {
					started, executionToken, err = s.exchangeTaskRepo.WithContext(ctx).TryMarkRunning(task.ID)
					if err == nil && started {
						task = latest
					}
				}
			}
		}
		if !started {
			reason := "任务正在执行或当前状态不可执行，请稍后刷新任务结果"
			if latestErr == nil && latest != nil {
				if strings.TrimSpace(latest.LastResult) != "" {
					reason = latest.LastResult
				} else if latest.Status == string(models.ExchangeTaskRunning) {
					reason = "任务正在执行，请稍后查看结果"
				}
			}
			log.Printf("【抢兑任务】任务 %d 未取得执行权: %s", task.ID, reason)
			s.sendToUser(task.UserID, ws.Message{Type: "exchange_skipped", Data: map[string]interface{}{
				"task_id": task.ID, "account_name": exchangeAccountName(&task.ExchangeAccount),
				"product_name": task.PrizeName, "success": false, "message": reason,
			}})
			return fmt.Errorf("%w: %s", ErrExchangeTaskConflict, reason)
		}
	}
	finished := false
	defer func() {
		if recovered := recover(); recovered != nil {
			message := fmt.Sprintf("任务执行异常，已自动标记失败: %v", recovered)
			log.Printf("【抢兑任务】任务 %d 执行 panic: %v", task.ID, recovered)
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if persistErr := s.persistExchangeOutcome(cleanupCtx, task, executionToken, false, message, 0, models.ExchangeTaskFailed); persistErr != nil {
				log.Printf("【抢兑任务】panic 结果持久化失败: task_id=%d err=%v", task.ID, persistErr)
			} else {
				finished = true
			}
			execErr = fmt.Errorf("%w: %s", ErrExchangeExecutionFailed, message)
			return
		}
		if finished || ctx.Err() == nil {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		released, releaseErr := s.exchangeTaskRepo.WithContext(cleanupCtx).ReleaseRunning(task.ID, executionToken, "任务因执行上下文取消，已恢复为待执行")
		if releaseErr != nil {
			log.Printf("【抢兑任务】取消后恢复任务失败: task_id=%d err=%v", task.ID, releaseErr)
		} else if released {
			log.Printf("【抢兑任务】任务 %d 因上下文取消已恢复为 pending", task.ID)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		if err := s.exchangeTaskRepo.WithContext(ctx).TransitionRunning(task.ID, executionToken, string(models.ExchangeTaskPending), reason); err != nil {
			return fmt.Errorf("释放月度保护任务执行权: %w", err)
		}
		log.Printf("【抢兑月度保护】任务 %d 跳过执行: %s", task.ID, reason)
		s.sendToUser(task.UserID, ws.Message{Type: "exchange_skipped", Data: map[string]interface{}{
			"task_id": task.ID, "account_name": exchangeAccountName(&task.ExchangeAccount),
			"product_name": task.PrizeName, "success": false, "message": reason,
		}})
		finished = true
		return fmt.Errorf("%w: %s", ErrExchangeMonthlyLimitReached, reason)
	}

	account, err := s.exchangeAccountRepo.WithContext(ctx).GetByID(task.ExchangeAccountID)
	if err != nil {
		message := "获取兑换账号失败"
		if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, false, message, 0, models.ExchangeTaskFailed); persistErr != nil {
			return persistErr
		}
		finished = true
		return fmt.Errorf("%w: %s", ErrExchangeRuleNotFound, message)
	}

	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}
	maskedAccountName := maskExchangeAccountName(accountName)

	if s.accountRepo != nil && account.AccountID > 0 {
		cloudAccount, err := s.accountRepo.WithContext(ctx).GetByID(account.AccountID)
		if err != nil {
			message := "云盘账号不存在或已删除"
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, false, message, 0, models.ExchangeTaskFailed); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeCloudAccountMissing, message)
		}
		if !cloudAccount.IsActive {
			message := "云盘账号已失效，请重新登录后再启用任务"
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, false, message, 0, models.ExchangeTaskPending); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeAccountDisabled, message)
		}
		if strings.TrimSpace(cloudAccount.Auth) == "" {
			message := "云盘账号认证为空，请重新登录"
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, false, message, 0, models.ExchangeTaskPending); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeCredentialsMissing, message)
		}
	}

	log.Printf("【抢兑任务】开始执行任务 %d，账号: %s，商品: %s", task.ID, maskedAccountName, task.PrizeName)
	maxRetries := effectiveExchangeMaxRetries(task.MaxRetries)

	var success bool
	var message string
	var execTime int
	attemptsUsed := 0
	releaseMonthlySeriesLock := func() {}
	keepMonthlySeriesLock := false
	if locked, release, reason := acquireMonthlySeriesLock(s.lockStore, s.productRepo, task, time.Now()); !locked {
		success, message, execTime = false, reason, 0
	} else {
		releaseMonthlySeriesLock = release
		// Context cancellation, a persistence error or a panic can occur after
		// a monthly lock has been acquired.  Failures must release only their
		// own token; successful claims intentionally remain until month end.
		defer func() {
			if !keepMonthlySeriesLock {
				releaseMonthlySeriesLock()
			}
		}()
		var retryErr error
		success, message, execTime, attemptsUsed, retryErr = runExchangeWithRetries(
			ctx,
			maxRetries,
			func(attempt int) error {
				log.Printf("【抢兑任务】任务 %d 第 %d 次重试...", task.ID, attempt)
				now := time.Now()
				if err := s.exchangeTaskRepo.WithContext(ctx).UpdateRetryCountOwned(task.ID, executionToken, attempt, &now); err != nil {
					return fmt.Errorf("更新任务重试次数: %w", err)
				}
				return sleepExchangeContext(ctx, time.Duration(attempt*2)*time.Second)
			},
			func() (bool, string, int) {
				prizeID := s.resolveTaskPrizeID(task)
				if !isUsableExchangePrizeID(prizeID) {
					return false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0
				}
				return s.doExchangeContext(ctx, account, prizeID)
			},
		)
		if retryErr != nil {
			return retryErr
		}
	}

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

	// Upstream acceptance is already an externally visible monthly claim. Keep
	// the lease even if the following database finalization is temporarily
	// unavailable; releasing it here would permit a second redemption in the
	// same month before the recovery path can persist the first result.
	if success {
		keepMonthlySeriesLock = true
	}
	if err := s.persistExchangeOutcome(ctx, task, executionToken, success, message, execTime, finalStatus); err != nil {
		return err
	}
	finished = true
	if !success {
		releaseMonthlySeriesLock()
	}
	// A terminally persisted result is no longer an in-flight lock: failures
	// were released above, while successful monthly claims remain in Redis.
	keepMonthlySeriesLock = true

	s.sendToUser(task.UserID, ws.Message{Type: "exchange_complete", Data: map[string]interface{}{
		"task_id": task.ID, "account_name": accountName, "product_name": task.PrizeName,
		"success": success, "message": message, "retry_count": task.RetryCount,
	}})
	if !success {
		return fmt.Errorf("%w: %s", ErrExchangeExecutionFailed, strings.TrimSpace(message))
	}
	return nil
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
func (s *ExchangeService) persistExchangeOutcome(ctx context.Context, task *models.ExchangeTask, executionToken string, success bool, message string, execTimeMs int, status models.ExchangeTaskStatus) error {
	if task == nil || task.ID == 0 {
		return fmt.Errorf("refuse to persist empty exchange task")
	}
	err := s.exchangeTaskRepo.WithContext(ctx).FinalizeOwned(task, executionToken, success, message, status, execTimeMs)
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
