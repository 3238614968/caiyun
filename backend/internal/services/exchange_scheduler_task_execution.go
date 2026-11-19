package services

import (
	"caiyun/internal/models"
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"
)

func (s *ExchangeScheduler) executeTask(ctx context.Context, task *models.ExchangeTask) (execution exchangeTaskExecutionResult) {
	if ctx == nil {
		ctx = context.Background()
	}
	execution.outcome = models.ExchangeOutcomeNotSubmitted
	execution.skipped = true
	defer func() {
		if recovered := recover(); recovered != nil {
			execution.skipped = false
			execution.exchangeAttemptResult = preflightExchangeResult(fmt.Sprintf("任务执行异常，已自动标记失败: %v", recovered), true)
			log.Printf("【抢兑调度器】任务 %d 执行 panic: %v\n%s", task.ID, recovered, debug.Stack())
		}
	}()
	if err := ctx.Err(); err != nil {
		execution.message = err.Error()
		return execution
	}
	if task != nil && task.Status == string(models.ExchangeTaskRunning) && task.Outcome == string(models.ExchangeOutcomeUnknown) {
		execution.skipped = false
		execution.exchangeAttemptResult = unknownExchangeResult("兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换", 0)
		return execution
	}
	if !s.schedulingEnabled(ctx) {
		execution.message = "抢兑开关未启用或不可用，跳过本轮"
		return execution
	}
	taskRepo := s.exchangeTaskRepo.WithContext(ctx)
	started, executionToken, err := taskRepo.TryMarkRunning(task.ID)
	if err != nil {
		execution.message = fmt.Sprintf("抢占任务执行权失败: %v", err)
		return execution
	}
	if !started {
		execution.message = "任务已被其他实例执行或状态不再是待执行"
		execution.persistenceErr = s.finalizeTaskResult(task, execution)
		return execution
	}
	execution.executionToken = executionToken
	execution.skipped = false
	stopHeartbeat := startExchangeLeaseHeartbeat(ctx, taskRepo, task.ID, executionToken, exchangeTaskRunningTimeoutFromEnv())
	task.RetryCount = 0
	task.LastRetryAt = nil
	completed := false
	defer func() {
		stopHeartbeat()
		if completed || ctx.Err() == nil || executionToken == "" {
			return
		}
		s.releaseCanceledTask(task, executionToken, ctx.Err(), &execution)
	}()

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		if err := taskRepo.TransitionRunning(task.ID, executionToken, string(models.ExchangeTaskPending), reason); err != nil {
			execution.message = fmt.Sprintf("更新跳过状态失败: %v", err)
			return execution
		}
		completed = true
		execution.message = reason
		execution.skipped = true
		return execution
	}

	if task.Product.ID > 0 && (!task.Product.IsActive || task.Product.StockStatus != "available" || task.Product.DailyRemainderCount <= 0) {
		log.Printf("【抢兑调度器】任务 %d 本地商品快照显示可能不可抢兑，仍以实时接口为准", task.ID)
	}
	account, err := s.exchangeAccountRepo.WithContext(ctx).GetByID(task.ExchangeAccountID)
	if err != nil {
		execution.message = fmt.Sprintf("获取兑换账号失败: %v", err)
		return execution
	}
	if !account.IsActive {
		execution.message = "账号已禁用"
		return execution
	}
	if task.ExchangeAccount.Account.ID > 0 && !task.ExchangeAccount.Account.IsActive {
		execution.message = "云盘账号已失效，请重新登录后再启用任务"
		return execution
	}
	prizeID := s.resolveTaskPrizeID(task)
	if !isUsableExchangePrizeID(prizeID) {
		execution.message = "商品已下架或不存在，请更新商品列表后重新创建抢兑任务"
		return execution
	}
	locked, release, reason := acquireMonthlySeriesLock(s.leaseStore, s.productRepo, task, time.Now())
	if !locked {
		execution.message = reason
		return execution
	}
	keepMonthlySeriesLock := false
	defer func() {
		if !keepMonthlySeriesLock {
			release()
		}
	}()

	maxRetries := effectiveExchangeMaxRetries(task.MaxRetries)
	preparedSession := s.takeWarmSession(task.ID, account.AccountID, account.Auth)
	var retryErr error
	execution.skipped = false
	execution.exchangeAttemptResult, execution.attemptsUsed, retryErr = runExchangeWithRetries(
		ctx,
		maxRetries,
		func(attempt int) error {
			log.Printf("【抢兑调度器】任务 %d 第 %d 次重试...", task.ID, attempt)
			now := time.Now()
			if err := taskRepo.UpdateRetryCountOwned(task.ID, executionToken, attempt, &now); err != nil {
				return fmt.Errorf("更新任务重试次数失败: %w", err)
			}
			return sleepExchangeContext(ctx, time.Duration(attempt*2)*time.Second)
		},
		func() exchangeAttemptResult {
			prizeID = s.resolveTaskPrizeID(task)
			if !isUsableExchangePrizeID(prizeID) {
				return preflightExchangeResult("商品已下架或不存在，请更新商品列表后重新创建抢兑任务", true)
			}
			return performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.leaseStore, preparedSession,
				newExchangeSubmissionGuard(taskRepo, task.ID, executionToken))
		},
	)
	// Persistence belongs to the outer product-group settlement exactly once.
	if retryErr != nil && execution.outcome != models.ExchangeOutcomeUnknown {
		execution.message = retryErr.Error()
	}
	keepMonthlySeriesLock = exchangeResultPreservesMonthlySeries(execution.exchangeAttemptResult)
	if execution.guardErr != nil {
		execution.persistenceErr = taskRepo.TransitionRunning(task.ID, executionToken, string(models.ExchangeTaskPending), execution.message)
		completed = execution.persistenceErr == nil
		execution.skipped = completed
		return execution
	}
	completed = true
	execution.persistenceErr = s.persistSchedulerResult(context.Background(), task, execution)
	execution.skipped = false
	return execution
}

func (s *ExchangeScheduler) releaseCanceledTask(task *models.ExchangeTask, executionToken string, cause error, execution *exchangeTaskExecutionResult) {
	if s == nil || s.exchangeTaskRepo == nil || executionToken == "" || task == nil {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	released, err := s.exchangeTaskRepo.WithContext(cleanupCtx).ReleaseRunning(task.ID, executionToken, fmt.Sprintf("调度器停止，任务待恢复: %v", cause))
	if err != nil {
		log.Printf("【抢兑调度器】取消后释放执行权失败: task_id=%d err=%v", task.ID, err)
		return
	}
	if released {
		execution.skipped = true
		execution.message = fmt.Sprintf("调度器执行已取消: %v", cause)
		return
	}
	execution.skipped = false
	execution.exchangeAttemptResult = unknownExchangeResult("兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换", 0)
	cleanupCtx, cancel = context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	execution.persistenceErr = s.persistSchedulerResult(cleanupCtx, task, *execution)
	if execution.persistenceErr != nil {
		log.Printf("【抢兑调度器】取消后记录待核对结果失败: task_id=%d err=%v", task.ID, execution.persistenceErr)
	}
}

func (s *ExchangeScheduler) persistSchedulerResult(ctx context.Context, task *models.ExchangeTask, execution exchangeTaskExecutionResult) error {
	if ctx == nil {
		ctx = context.Background()
	}
	cleanupCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	ctx = cleanupCtx
	task.ExecutionToken = execution.executionToken
	settlement := execution.settlement(models.ExchangeTaskFailed)
	if execution.outcome != models.ExchangeOutcomeUnknown {
		settlement.Status = exchangeTaskFinalStatus(task, execution.exchangeAttemptResult)
	}
	for attempt := 0; attempt < 3; attempt++ {
		if ctx != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if err := s.exchangeTaskRepo.WithContext(ctx).FinalizeOwned(task, execution.executionToken, settlement); err != nil {
			if attempt == 2 {
				return err
			}
			continue
		}
		break
	}
	task.Outcome = string(settlement.Outcome)
	task.ExecutionToken = ""
	task.AttemptedCount++
	task.LastResult = settlement.Message
	task.Status = string(settlement.Status)
	if execution.success {
		task.SuccessCount++
	} else {
		task.FailCount++
	}
	s.recordResult(task, execution.exchangeAttemptResult, settlement.ExecutionTimeMs)
	return nil
}

func (s *ExchangeScheduler) resolveTaskPrizeID(task *models.ExchangeTask) string {
	prizeID := taskExchangePrizeID(task)
	if isUsableExchangePrizeID(prizeID) {
		return prizeID
	}
	if s.productRepo == nil || task == nil || task.PrizeName == "" {
		return prizeID
	}
	product, err := s.productRepo.FindExchangeableReplacement(task.PrizeName, task.PrizeID)
	if err != nil {
		log.Printf("【抢兑调度器】任务 %d 查询商品替换失败: %v", task.ID, err)
		return prizeID
	}
	if product == nil || !isUsableExchangePrizeID(product.PrizeID) {
		return prizeID
	}
	if task.PrizeID != product.PrizeID || task.ProductID != product.ID {
		task.PrizeID = product.PrizeID
		task.ProductID = product.ID
		task.Product = *product
		_ = s.exchangeTaskRepo.UpdatePrizeSnapshot(task.ID, product.ID, product.PrizeID, product.PrizeName)
		log.Printf("【抢兑调度器】任务 %d 已自动修正商品ID为 %s，避免使用历史 memo 导致 404", task.ID, product.PrizeID)
	}
	return product.PrizeID
}
