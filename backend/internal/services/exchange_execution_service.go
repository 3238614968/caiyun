package services

import (
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/utils"
	"caiyun/internal/ws"
	"context"
	"errors"
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
	Outcome string `json:"outcome,omitempty"`
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
			if err := ctx.Err(); err != nil {
				result.Message = "任务已取消"
				results[index] = result
				return
			}

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
				if errors.Is(err, ErrExchangeOutcomeUnknown) || (task.Status == string(models.ExchangeTaskFailed) && task.Outcome == string(models.ExchangeOutcomeUnknown)) {
					result.Outcome = string(models.ExchangeOutcomeUnknown)
				}
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

func (s *ExchangeService) settleUnknownTask(ctx context.Context, task *models.ExchangeTask, operationID string) error {
	cutoff := time.Now().Add(-exchangeTaskRunningTimeoutFromEnv())
	conflict := fmt.Errorf("%w: %s", ErrExchangeTaskConflict, exchangeTaskClaimConflictReason(string(models.ExchangeTaskRunning)))
	if !task.UpdatedAt.Before(cutoff) {
		return conflict
	}
	if err := s.exchangeTaskRepo.WithContext(ctx).FinalizeStaleSubmission(task, cutoff, operationID); errors.Is(err, repository.ErrExchangeTaskExecutionLost) {
		if completed, readErr := s.completedTaskResult(ctx, operationID, task.ID); completed || readErr != nil {
			return readErr
		}
		return conflict
	} else if err != nil {
		return err
	}
	const message = "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
	createExchangeSystemLog(s.taskLogRepo, task.UserID, task.ExchangeAccount.AccountID, task.PrizeName, exchangeAccountName(&task.ExchangeAccount), false, message, 0)
	s.sendToUser(task.UserID, ws.Message{Type: "exchange_result", Data: map[string]interface{}{
		"task_id": task.ID, "prize_name": task.PrizeName, "success": false,
		"outcome": string(models.ExchangeOutcomeUnknown), "message": message,
	}})
	return newExchangeOutcomeUnknownError(message)
}

// executeSingleTask 执行单个抢兑任务（带重试机制）
func (s *ExchangeService) executeSingleTask(task *models.ExchangeTask) {
	_ = s.executeSingleTaskContext(context.Background(), task)
}

func (s *ExchangeService) executeSingleTaskContext(ctx context.Context, task *models.ExchangeTask) error {
	return s.executeTaskWithSession(ctx, task, &exchangePreparedSession{})
}

func (s *ExchangeService) executeTaskWithSession(ctx context.Context, task *models.ExchangeTask, preparedSession *exchangePreparedSession) (execErr error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if task == nil || task.ID == 0 {
		return ErrExchangeInvalidInput
	}
	operationID := exchangeOperationContextID(ctx)
	if completed, err := s.completedTaskResult(ctx, operationID, task.ID); completed || err != nil {
		return err
	}
	if task.Status == string(models.ExchangeTaskRunning) && task.Outcome == string(models.ExchangeOutcomeUnknown) {
		return s.settleUnknownTask(ctx, task, operationID)
	}
	claim, err := claimExchangeTaskManually(s.exchangeTaskRepo.WithContext(ctx), task, time.Now(), exchangeTaskRunningTimeoutFromEnv(), operationID)
	if err != nil {
		log.Printf("【抢兑任务】任务 %d 抢占执行权失败: %v", task.ID, err)
		return err
	}
	task = claim.task
	if !claim.started {
		if completed, err := s.completedTaskResult(ctx, operationID, task.ID); completed || err != nil {
			return err
		}
		log.Printf("【抢兑任务】任务 %d 未取得执行权: 当前状态=%s，原因=%s", task.ID, task.Status, claim.reason)
		s.sendToUser(task.UserID, ws.Message{Type: "exchange_skipped", Data: map[string]interface{}{
			"task_id": task.ID, "account_name": exchangeAccountName(&task.ExchangeAccount),
			"product_name": task.PrizeName, "success": false, "message": claim.reason,
		}})
		return fmt.Errorf("%w: %s", ErrExchangeTaskConflict, claim.reason)
	}
	executionToken := claim.token
	stopHeartbeat := startExchangeLeaseHeartbeat(ctx, s.exchangeTaskRepo.WithContext(ctx), task.ID, executionToken, exchangeTaskRunningTimeoutFromEnv())
	defer stopHeartbeat()
	finished := false
	var result exchangeAttemptResult
	defer func() {
		if recovered := recover(); recovered != nil {
			if finished {
				execErr = nil
				return
			}
			message := fmt.Sprintf("任务执行异常，已自动标记失败: %v", recovered)
			log.Printf("【抢兑任务】任务 %d 执行 panic: %v", task.ID, recovered)
			result = preflightExchangeResult(message, true)
			result.stop = true
			execErr = s.finalizeTaskExecution(task, result, executionToken, operationID)
			finished = execErr == nil || errors.Is(execErr, ErrExchangeOutcomeUnknown)
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
			return
		}
		// Unknown submissions are settled independently; cancellation must not
		// discard the durable boundary or replay the upstream request.
		result = unknownExchangeResult("兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换", 0)
		execErr = s.finalizeTaskExecution(task, result, executionToken, operationID)
		finished = execErr == nil || errors.Is(execErr, ErrExchangeOutcomeUnknown)
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
		if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, preflightExchangeResult(message, true), models.ExchangeTaskFailed); persistErr != nil {
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
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, preflightExchangeResult(message, true), models.ExchangeTaskFailed); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeCloudAccountMissing, message)
		}
		if !cloudAccount.IsActive {
			message := "云盘账号已失效，请重新登录后再启用任务"
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, preflightExchangeResult(message, true), models.ExchangeTaskPending); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeAccountDisabled, message)
		}
		if strings.TrimSpace(cloudAccount.Auth) == "" {
			message := "云盘账号认证为空，请重新登录"
			if persistErr := s.persistExchangeOutcome(ctx, task, executionToken, preflightExchangeResult(message, true), models.ExchangeTaskPending); persistErr != nil {
				return persistErr
			}
			finished = true
			return fmt.Errorf("%w: %s", ErrExchangeCredentialsMissing, message)
		}
	}

	log.Printf("【抢兑任务】开始执行任务 %d，账号: %s，商品: %s", task.ID, maskedAccountName, task.PrizeName)
	maxRetries := effectiveExchangeMaxRetries(task.MaxRetries)

	var attemptsUsed int
	releaseMonthlySeriesLock := func() {}
	keepMonthlySeriesLock := false
	if locked, release, reason := acquireMonthlySeriesLock(s.lockStore, s.productRepo, task, time.Now()); !locked {
		result = preflightExchangeResult(reason, false)
	} else {
		releaseMonthlySeriesLock = release
		// Accepted and unknown outcomes may both represent an upstream monthly
		// claim. Never release that protection before reconciliation.
		defer func() {
			if !keepMonthlySeriesLock {
				releaseMonthlySeriesLock()
			}
		}()
		var retryErr error
		result, attemptsUsed, retryErr = runExchangeWithRetries(
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
			func() exchangeAttemptResult {
				prizeID := s.resolveTaskPrizeID(task)
				if !isUsableExchangePrizeID(prizeID) {
					return preflightExchangeResult("商品已下架或不存在，请更新商品列表后重新创建抢兑任务", true)
				}
				return performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.lockStore, preparedSession,
					newExchangeSubmissionGuard(s.exchangeTaskRepo.WithContext(ctx), task.ID, executionToken))
			},
		)
		if retryErr != nil && result.outcome == models.ExchangeOutcomeUnknown {
			keepMonthlySeriesLock = exchangeResultPreservesMonthlySeries(result)
			return s.finalizeTaskExecution(task, result, executionToken, operationID)
		}
		if retryErr != nil {
			return retryErr
		}
	}

	if result.guardErr != nil {
		if err := s.exchangeTaskRepo.WithContext(ctx).TransitionRunning(task.ID, executionToken, string(models.ExchangeTaskPending), result.message); err != nil {
			return err
		}
		finished = true
		return result.guardErr
	}
	if result.outcome != "" && result.outcome != models.ExchangeOutcomeNotSubmitted {
		keepMonthlySeriesLock = exchangeResultPreservesMonthlySeries(result)
		err := s.finalizeTaskExecution(task, result, executionToken, operationID)
		if err != nil && !errors.Is(err, ErrExchangeOutcomeUnknown) && !errors.Is(err, ErrExchangeExecutionFailed) {
			return err
		}
		finished = true
		s.notifyExchangeCompleted(ctx, task, result, attemptsUsed)
		return err
	}
	finalStatus := exchangeTaskFinalStatus(task, result)
	if result.success {
		log.Printf("【抢兑任务】任务 %d 执行成功，账号: %s", task.ID, maskedAccountName)
	} else {
		log.Printf("【抢兑任务】任务 %d 执行结束，账号: %s，结果: %s", task.ID, maskedAccountName, result.message)
		if finalStatus == models.ExchangeTaskPending {
			log.Printf("【抢兑任务】任务 %d 本场失败，等待下一个抢兑时间", task.ID)
		}
	}
	keepMonthlySeriesLock = exchangeResultPreservesMonthlySeries(result)
	task.RetryCount = attemptsUsed
	if err := s.persistExchangeOutcome(context.Background(), task, executionToken, result, finalStatus); err != nil {
		return err
	}
	finished = true
	if result.outcome == models.ExchangeOutcomeRejected || result.outcome == models.ExchangeOutcomeNotSubmitted {
		releaseMonthlySeriesLock()
	}
	keepMonthlySeriesLock = true

	operation, _ := ctx.Value(taskOperationKey{}).(taskOperation)
	s.sendToUser(task.UserID, ws.Message{Type: "exchange_complete", Data: map[string]interface{}{
		"task_id": task.ID, "account_name": accountName, "product_name": task.PrizeName,
		"success": result.success, "outcome": string(result.outcome), "message": result.message, "retry_count": attemptsUsed,
		"operation_id": operation.id, "operation_attempt": operation.attempt,
	}})
	if result.outcome == models.ExchangeOutcomeUnknown {
		return newExchangeOutcomeUnknownError(result.message)
	}
	if !result.success {
		return fmt.Errorf("%w: %s", ErrExchangeExecutionFailed, strings.TrimSpace(result.message))
	}
	return nil
}

func (s *ExchangeService) notifyExchangeCompleted(ctx context.Context, task *models.ExchangeTask, result exchangeAttemptResult, attemptsUsed int) {
	if task == nil {
		return
	}
	operation, _ := ctx.Value(taskOperationKey{}).(taskOperation)
	s.sendToUser(task.UserID, ws.Message{Type: "exchange_complete", Data: map[string]interface{}{
		"task_id": task.ID, "account_name": exchangeAccountName(&task.ExchangeAccount), "product_name": task.PrizeName,
		"success": result.success, "outcome": string(result.outcome), "message": result.message, "retry_count": attemptsUsed,
		"operation_id": operation.id, "operation_attempt": operation.attempt,
	}})
}

// shouldRetry 判断是否需要重试（使用公共函数）
func (s *ExchangeService) shouldRetry(message string) bool {
	if strings.TrimSpace(message) == "" {
		return true
	}
	return isExchangeRetryableFailure(message)
}

func (s *ExchangeService) doExchangeContext(ctx context.Context, account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	result := performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.lockStore, &exchangePreparedSession{})
	return result.success, result.message, result.execTime
}
func (s *ExchangeService) doExchange(account *models.ExchangeAccount, prizeID string) (bool, string, int) {
	return performExchangeWithControls(account, prizeID, s.tokenMgr, s.lockStore)
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
