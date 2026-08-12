package services

import (
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
)

func (s *ExchangeScheduler) executeExchangeWithAutoSwitch(tasks []*models.ExchangeTask, period string) {
	ctx := s.executionContext()
	if err := ctx.Err(); err != nil {
		log.Printf("【抢兑调度器】%s时段抢兑已取消: %v", period, err)
		return
	}
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
			defer func() {
				if r := recover(); r != nil {
					log.Printf("【抢兑调度器】商品组 %s 执行 panic: %v\n%s", prizeID, r, debug.Stack())
				}
			}()
			s.executeProductGroup(ctx, prizeID, groupTasks, limiter)
		}(prizeID, groupTasks)
	}

	wg.Wait()

	log.Printf("【抢兑调度器】%s时段抢兑执行完成", period)

	// 发送完成通知
	s.broadcast(ws.Message{
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
func (s *ExchangeScheduler) executeProductGroup(ctx context.Context, prizeID string, tasks []*models.ExchangeTask, limiter chan struct{}) {
	if ctx == nil {
		ctx = context.Background()
	}
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
		if err := ctx.Err(); err != nil {
			log.Printf("【抢兑调度器】商品 %s 执行已取消: %v", prizeID, err)
			break
		}
		if task == nil {
			continue
		}
		task := task
		wg.Add(1)
		go func() {
			defer wg.Done()
			limiterAcquired := false
			executionToken := ""
			defer func() {
				if limiterAcquired {
					<-limiter
				}
				if r := recover(); r != nil {
					message := fmt.Sprintf("任务执行异常，已自动标记失败: %v", r)
					log.Printf("【抢兑调度器】任务 %d 执行 panic: %v\n%s", task.ID, r, debug.Stack())
					s.finalizeTaskResult(task, executionToken, false, message, 0)
				}
			}()

			accountName := exchangeAccountName(&task.ExchangeAccount)
			if accountName == "" {
				accountName = fmt.Sprintf("exchange-account-%d", task.ExchangeAccountID)
			}
			maskedAccountName := maskExchangeAccountName(accountName)

			if stopped, reason := getStopReason(); stopped {
				if reason == "" {
					reason = "商品已无库存，跳过抢兑"
				}
				log.Printf("【抢兑调度器】任务 %d 跳过执行，商品 %s 已停止抢兑，原因: %s", task.ID, prizeID, reason)
				recordFailureReason(reason)
				s.finalizeTaskResult(task, executionToken, false, reason, 0)
				return
			}

			successMutex.Lock()
			if successMap[task.ID] {
				successMutex.Unlock()
				message := "任务已在同一批次完成，跳过重复执行"
				log.Printf("【抢兑调度器】任务 %d 检测到重复入队，跳过重复执行", task.ID)
				s.reportSkippedTask(task, message)
				return
			}
			successMutex.Unlock()

			if task.Product.ID > 0 {
				log.Printf(
					"【抢兑调度器】任务 %d 开始抢兑: 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s), 本地快照 active=%t, stock=%s, remain=%d",
					task.ID,
					maskedAccountName,
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
					maskedAccountName,
					task.ExchangeAccountID,
					task.ExchangeAccount.AccountID,
					task.PrizeName,
					task.PrizeID,
				)
			}

			select {
			case limiter <- struct{}{}:
				limiterAcquired = true
			case <-ctx.Done():
				log.Printf("【抢兑调度器】任务 %d 等待执行槽时已取消", task.ID)
				return
			}
			if stopped, reason := getStopReason(); stopped {
				if reason == "" {
					reason = "商品已无库存，跳过抢兑"
				}
				log.Printf("【抢兑调度器】任务 %d 获取执行槽后跳过，商品 %s 已停止抢兑，原因: %s", task.ID, prizeID, reason)
				recordFailureReason(reason)
				s.finalizeTaskResult(task, executionToken, false, reason, 0)
				return
			}

			success, message, execTime, executionToken := s.executeTask(ctx, task)
			<-limiter
			limiterAcquired = false
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
					maskedAccountName,
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
					maskedAccountName,
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

			s.finalizeTaskResult(task, executionToken, success, message, execTime)
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

func (s *ExchangeScheduler) executionContext() context.Context {
	if s == nil || s.executionCtx == nil {
		return context.Background()
	}
	return s.executionCtx
}
