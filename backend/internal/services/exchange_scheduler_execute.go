package services

import (
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

func (s *ExchangeScheduler) executeExchangeWithAutoSwitch(tasks []*models.ExchangeTask, period string) {
	// 按商品ID分组任务
	taskGroups := s.groupTasksByProduct(tasks)

	// 获取并发配置，并在所有商品组之间共享全局并发上限
	concurrency := s.getConfiguredConcurrency()
	limiter := make(chan struct{}, concurrency)

	// 为每个商品组创建执行器
	var wg sync.WaitGroup

	for prizeID, groupTasks := range taskGroups {
		wg.Add(1)
		go func(prizeID string, groupTasks []*models.ExchangeTask) {
			defer wg.Done()
			s.executeProductGroup(prizeID, groupTasks, limiter)
		}(prizeID, groupTasks)
	}

	wg.Wait()

	log.Printf("【抢兑调度器】%s时段抢兑执行完成", period)

	// 发送完成通知
	s.hub.Broadcast(ws.Message{
		Type: "exchange_completed",
		Data: map[string]interface{}{
			"period":  period,
			"message": fmt.Sprintf("%s 抢兑执行完成", humanizeExchangePeriod(period)),
		},
	})
}

// groupTasksByProduct 按商品ID分组任务
func (s *ExchangeScheduler) groupTasksByProduct(tasks []*models.ExchangeTask) map[string][]*models.ExchangeTask {
	groups := make(map[string][]*models.ExchangeTask)
	for _, task := range tasks {
		prizeID := s.resolveTaskPrizeID(task)
		if prizeID == "" {
			prizeID = taskExchangePrizeID(task)
		}
		groups[prizeID] = append(groups[prizeID], task)
	}
	return groups
}

// executeProductGroup 执行商品组的抢兑（自动切换账号）
func (s *ExchangeScheduler) executeProductGroup(prizeID string, tasks []*models.ExchangeTask, limiter chan struct{}) {
	log.Printf("【抢兑调度器】开始抢兑商品 %s，共 %d 个账号", prizeID, len(tasks))

	successMap := make(map[uint]bool)
	failureReasons := make(map[string]int)
	var successMutex sync.Mutex
	var reasonMutex sync.Mutex
	var stopMutex sync.RWMutex
	shouldStop := false
	stopReason := ""
	var wg sync.WaitGroup

	getStopReason := func() (bool, string) {
		stopMutex.RLock()
		defer stopMutex.RUnlock()
		return shouldStop, stopReason
	}
	setStopReason := func(reason string) {
		stopMutex.Lock()
		defer stopMutex.Unlock()
		if !shouldStop {
			shouldStop = true
			stopReason = reason
		}
	}
	recordFailureReason := func(reason string) {
		if reason == "" {
			reason = "未知错误"
		}
		reasonMutex.Lock()
		failureReasons[reason]++
		reasonMutex.Unlock()
	}

	for _, task := range tasks {
		if task == nil {
			continue
		}
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()

			accountName := exchangeAccountName(&task.ExchangeAccount)
			if accountName == "" {
				accountName = fmt.Sprintf("exchange-account-%d", task.ExchangeAccountID)
			}

			if stopped, reason := getStopReason(); stopped {
				if reason == "" {
					reason = "商品已无库存，跳过抢兑"
				}
				log.Printf("【抢兑调度器】任务 %d 跳过执行，商品 %s 已停止抢兑，原因: %s", task.ID, prizeID, reason)
				recordFailureReason(reason)
				s.finalizeTaskResult(task, false, reason, 0)
				return
			}

			successMutex.Lock()
			if successMap[task.ID] {
				successMutex.Unlock()
				return
			}
			successMutex.Unlock()

			if task.Product.ID > 0 {
				log.Printf(
					"【抢兑调度器】任务 %d 开始抢兑: 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s), 本地快照 active=%t, stock=%s, remain=%d",
					task.ID,
					accountName,
					task.ExchangeAccountID,
					task.ExchangeAccount.AccountID,
					task.PrizeName,
					task.PrizeID,
					task.Product.IsActive,
					task.Product.StockStatus,
					task.Product.DailyRemainderCount,
				)
			} else {
				log.Printf(
					"【抢兑调度器】任务 %d 开始抢兑: 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s)",
					task.ID,
					accountName,
					task.ExchangeAccountID,
					task.ExchangeAccount.AccountID,
					task.PrizeName,
					task.PrizeID,
				)
			}

			limiter <- struct{}{}
			if stopped, reason := getStopReason(); stopped {
				<-limiter
				if reason == "" {
					reason = "商品已无库存，跳过抢兑"
				}
				log.Printf("【抢兑调度器】任务 %d 获取执行槽后跳过，商品 %s 已停止抢兑，原因: %s", task.ID, prizeID, reason)
				recordFailureReason(reason)
				s.finalizeTaskResult(task, false, reason, 0)
				return
			}

			success, message, execTime := s.executeTask(task)
			<-limiter
			if execTime < 0 {
				log.Printf("【抢兑调度器】任务 %d 跳过执行: %s", task.ID, message)
				return
			}

			if success {
				successMutex.Lock()
				successMap[task.ID] = true
				successMutex.Unlock()

				log.Printf(
					"【抢兑调度器】任务 %d 抢兑成功: 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s), 耗时=%dms, 结果=%s",
					task.ID,
					accountName,
					task.ExchangeAccountID,
					task.ExchangeAccount.AccountID,
					task.PrizeName,
					task.PrizeID,
					execTime,
					message,
				)
			} else {
				reason := message
				if reason == "" {
					reason = "未知错误"
				}

				recordFailureReason(reason)

				log.Printf(
					"【抢兑调度器】任务 %d 抢兑失败: 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s), 原因=%s, 耗时=%dms",
					task.ID,
					accountName,
					task.ExchangeAccountID,
					task.ExchangeAccount.AccountID,
					task.PrizeName,
					task.PrizeID,
					reason,
					execTime,
				)

				if s.shouldStopExchange(reason) {
					log.Printf("【抢兑调度器】商品 %s 抢兑停止，原因: %s", prizeID, reason)
					setStopReason(reason)
				}
			}

			s.finalizeTaskResult(task, success, message, execTime)
		}()
	}

	wg.Wait()

	successCount := 0
	for _, success := range successMap {
		if success {
			successCount++
		}
	}
	failureCount := len(tasks) - successCount

	log.Printf("【抢兑调度器】商品 %s 抢兑完成，成功 %d/%d 个账号，失败 %d 个账号", prizeID, successCount, len(tasks), failureCount)
	for reason, count := range failureReasons {
		log.Printf("【抢兑调度器】商品 %s 失败原因统计: %s x%d", prizeID, reason, count)
	}
}

func (s *ExchangeScheduler) executeTask(task *models.ExchangeTask) (bool, string, int) {
	started, err := s.exchangeTaskRepo.TryMarkRunning(task.ID)
	if err != nil {
		return false, fmt.Sprintf("抢占任务执行权失败: %v", err), 0
	}
	if !started {
		return false, "任务已被其他实例执行或状态不再是待执行", -1
	}

	if skip, reason := s.monthlySeriesSkipReason(task); skip {
		_ = s.exchangeTaskRepo.UpdateLastResult(task.ID, reason)
		_ = s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
		return false, reason, -1
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

	account, err := s.exchangeAccountRepo.GetByID(task.ExchangeAccountID)
	if err != nil {
		return false, fmt.Sprintf("获取兑换账号失败: %v", err), 0
	}

	if !account.IsActive {
		return false, "账号已禁用", 0
	}

	if task.ExchangeAccount.Account.ID > 0 && !task.ExchangeAccount.Account.IsActive {
		return false, "云盘账号已失效，请重新登录后再启用任务", 0
	}

	prizeID := s.resolveTaskPrizeID(task)
	if !isUsableExchangePrizeID(prizeID) {
		return false, "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", 0
	}

	locked, release, reason := s.acquireMonthlySeriesLock(task, time.Now())
	if !locked {
		return false, reason, 0
	}
	success, message, execTime := performExchange(account, prizeID, s.tokenMgr)
	if !success {
		release()
	}
	return success, message, execTime
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
		_ = s.exchangeTaskRepo.Update(task)
		log.Printf("【抢兑调度器】任务 %d 已自动修正商品ID为 %s，避免使用历史 memo 导致 404", task.ID, product.PrizeID)
	}
	return product.PrizeID
}

// shouldStopExchange 判断是否应该停止抢兑
func (s *ExchangeScheduler) shouldStopExchange(message string) bool {
	// 以下商品级库存/上下架状态应该停止当前商品后续账号抢兑。
	// 账号级结果（如当前账号已兑换、云朵不足）不停止其他账号。
	stopPatterns := []string{
		"无库存",
		"库存不足",
		"已兑完",
		"已耗尽",
		"已下架",
		"奖品单日已耗尽",
		"奖品已兑完",
	}

	for _, pattern := range stopPatterns {
		if contains(message, pattern) {
			return true
		}
	}

	return false
}

// recordResult 记录抢兑结果，并与手动执行路径保持一致地更新尝试次数和最后结果。
func (s *ExchangeScheduler) recordResult(task *models.ExchangeTask, success bool, message string, execTime int) {
	status := "failed"
	if success {
		status = "success"
	}

	record := &models.ExchangeRecord{
		UserID:            task.UserID,
		ExchangeAccountID: task.ExchangeAccountID,
		ExchangeTaskID:    &task.ID,
		ProductID:         task.ProductID,
		PrizeID:           task.PrizeID,
		PrizeName:         task.PrizeName,
		Status:            status,
		Message:           message,
		ExecutionTimeMs:   execTime,
	}

	if err := s.exchangeRecordRepo.Create(record); err != nil {
		log.Printf("【抢兑调度器】记录抢兑结果失败: %v", err)
	}
	s.exchangeTaskRepo.UpdateAttempt(task.ID, success, message)
	createExchangeSystemLog(
		s.taskLogRepo,
		task.UserID,
		task.ExchangeAccount.AccountID,
		task.PrizeName,
		exchangeAccountName(&task.ExchangeAccount),
		success,
		message,
		execTime,
	)

	// 发送WebSocket通知
	s.hub.SendToUser(task.UserID, ws.Message{
		Type: "exchange_result",
		Data: map[string]interface{}{
			"task_id":      task.ID,
			"prize_name":   task.PrizeName,
			"success":      success,
			"message":      message,
			"execution_ms": execTime,
		},
	})
}

func (s *ExchangeScheduler) finalizeTaskResult(task *models.ExchangeTask, success bool, message string, execTime int) {
	s.recordResult(task, success, message, execTime)

	if success {
		if isSingleRunExchangeTask(task.TaskType) {
			if task.AttemptedCount+1 >= task.MaxAttempts {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
			} else {
				s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
			}
			return
		}

		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
		return
	}

	if s.shouldStopExchange(message) || strings.Contains(message, "商品已下架或不存在") || strings.Contains(message, "商品ID不是可兑换 prizeId") {
		s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskCompleted))
		return
	}

	s.exchangeTaskRepo.UpdateStatus(task.ID, string(models.ExchangeTaskPending))
}
