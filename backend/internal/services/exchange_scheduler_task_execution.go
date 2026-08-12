package services

import (
	"caiyun/internal/models"
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"time"
)

func (s *ExchangeScheduler) executeTask(ctx context.Context, task *models.ExchangeTask) (success bool, message string, execTime int, executionToken string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err.Error(), -1, ""
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			success = false
			message = fmt.Sprintf("任务执行异常，已自动标记失败: %v", recovered)
			execTime = 0
			log.Printf("【抢兑调度器】任务 %d 执行 panic: %v\n%s", task.ID, recovered, debug.Stack())
		}
	}()
	taskRepo := s.exchangeTaskRepo.WithContext(ctx)
	started, executionToken, err := taskRepo.TryMarkRunning(task.ID)
	if err != nil {
		return false, fmt.Sprintf("抢占任务执行权失败: %v", err), 0, ""
	}
	if !started {
		return false, "任务已被其他实例执行或状态不再是待执行", -1, ""
	}
	// If the scheduler is stopping after this Worker acquired the fencing token,
	// return the task to pending using a short independent cleanup context.  A
	// canceled request context must never leave this execution stranded in
	// running, and ReleaseRunning's token condition protects a newer Worker.
	completed := false
	defer func() {
		if completed || ctx.Err() == nil || executionToken == "" {
			return
		}
		s.releaseCanceledTask(task.ID, executionToken, ctx.Err())
		success = false
		message = fmt.Sprintf("调度器执行已取消: %v", ctx.Err())
		execTime = -1
	}()

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		if err := taskRepo.TransitionRunning(task.ID, executionToken, string(models.ExchangeTaskPending), reason); err != nil {
			return false, fmt.Sprintf("更新跳过状态失败: %v", err), 0, executionToken
		}
		completed = true
		return false, reason, -1, executionToken
	}

	// Product snapshots loaded before the refresh window may be stale.
	// Keep the snapshot for diagnostics, but always call the real exchange API.
	if task.Product.ID > 0 && (!task.Product.IsActive || task.Product.StockStatus != "available" || task.Product.DailyRemainderCount <= 0) {
		log.Printf(
			"【抢兑调度器】任务 %d 本地商品快照显示可能不可抢兑: active=%t, stock=%s, remain=%d；仍继续请求，以实时接口结果为准",
			task.ID,
			task.Product.IsActive,
			task.Product.StockStatus,
			task.Product.DailyRemainderCount,
		)
	}

	account, err := s.exchangeAccountRepo.WithContext(ctx).GetByID(task.ExchangeAccountID)
	if err != nil {
		return false, fmt.Sprintf("获取兑换账号失败: %v", err), 0, executionToken
	}

	if !account.IsActive {
		return false, "账号已禁用", 0, executionToken
	}

	if task.ExchangeAccount.Account.ID > 0 && !task.ExchangeAccount.Account.IsActive {
		return false, "云盘账号已失效，请重新登录后再启用任务", 0, executionToken
	}

	prizeID := s.resolveTaskPrizeID(task)
	if !isUsableExchangePrizeID(prizeID) {
		return false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0, executionToken
	}

	locked, release, reason := acquireMonthlySeriesLock(s.leaseStore, s.productRepo, task, time.Now())
	if !locked {
		return false, reason, 0, executionToken
	}
	keepMonthlySeriesLock := false
	defer func() {
		if !keepMonthlySeriesLock {
			release()
		}
	}()

	maxRetries := effectiveExchangeMaxRetries(task.MaxRetries)
	success, message, execTime, task.RetryCount, err = runExchangeWithRetries(
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
		func() (bool, string, int) {
			prizeID = s.resolveTaskPrizeID(task)
			if !isUsableExchangePrizeID(prizeID) {
				return false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0
			}
			return performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.leaseStore)
		},
	)
	if err != nil {
		if ctx.Err() != nil {
			return false, ctx.Err().Error(), -1, executionToken
		}
		return false, err.Error(), 0, executionToken
	}
	if success {
		// A success is a deliberate month-long claim; all false/panic return
		// paths release their owner token through the deferred closure.
		keepMonthlySeriesLock = true
	}
	completed = true
	return success, message, execTime, executionToken
}

func (s *ExchangeScheduler) releaseCanceledTask(taskID uint, executionToken string, cause error) {
	if s == nil || s.exchangeTaskRepo == nil || executionToken == "" {
		return
	}
	cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	released, err := s.exchangeTaskRepo.WithContext(cleanupCtx).ReleaseRunning(taskID, executionToken, fmt.Sprintf("调度器停止，任务待恢复: %v", cause))
	if err != nil {
		log.Printf("【抢兑调度器】任务 %d 取消后释放执行权失败: %v", taskID, err)
		return
	}
	if !released {
		log.Printf("【抢兑调度器】任务 %d 取消后未释放执行权（已由其他 Worker 接管或已完成）", taskID)
	}
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
