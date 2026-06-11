package services

import (
	"caiyun/internal/models"
	"fmt"
	"log"
	"sync"
	"time"
)

func (s *ExchangeScheduler) logQueuedTasks(slot string, tasks []*models.ExchangeTask) {
	for _, task := range tasks {
		if task == nil {
			continue
		}

		accountName := exchangeAccountName(&task.ExchangeAccount)
		if accountName == "" {
			accountName = fmt.Sprintf("exchange-account-%d", task.ExchangeAccountID)
		}

		log.Printf(
			"【抢兑调度器】%s 队列任务: task=%d, 账号=%s, exchange_account=%d, account=%d, 商品=%s(%s), 抢兑时间=%s/%s",
			slot,
			task.ID,
			accountName,
			task.ExchangeAccountID,
			task.ExchangeAccount.AccountID,
			task.PrizeName,
			task.PrizeID,
			task.ExchangeAccount.ExchangeTime1,
			task.ExchangeAccount.ExchangeTime2,
		)
	}
}

func (s *ExchangeScheduler) preheatAccountsForTasks(slot string, tasks []*models.ExchangeTask) map[uint]bool {
	if s.tokenMgr == nil || len(tasks) == 0 {
		return nil
	}

	uniqueAccounts := make(map[uint]models.ExchangeAccount)
	for _, task := range tasks {
		if task == nil {
			continue
		}
		if task.ExchangeAccount.AccountID == 0 {
			log.Printf("【抢兑调度器】%s 预热跳过: task=%d, exchange_account=%d 缺少云盘账号 ID", slot, task.ID, task.ExchangeAccountID)
			continue
		}
		if task.ExchangeAccount.Account.ID > 0 {
			if !task.ExchangeAccount.Account.IsActive {
				log.Printf("【抢兑调度器】%s 预热跳过: task=%d, account=%d 云盘账号已失效", slot, task.ID, task.ExchangeAccount.AccountID)
				continue
			}
			if task.ExchangeAccount.Account.Auth == "" {
				log.Printf("【抢兑调度器】%s 预热跳过: task=%d, account=%d 云盘账号认证为空", slot, task.ID, task.ExchangeAccount.AccountID)
				continue
			}
		}
		if _, exists := uniqueAccounts[task.ExchangeAccount.AccountID]; exists {
			continue
		}
		uniqueAccounts[task.ExchangeAccount.AccountID] = task.ExchangeAccount
	}

	if len(uniqueAccounts) == 0 {
		return map[uint]bool{}
	}

	limit := s.getConfiguredConcurrency()
	if limit <= 0 {
		limit = 1
	}
	if limit > 10 {
		limit = 10
	}

	log.Printf("【抢兑调度器】%s 开始预热 JWT，共 %d 个云盘账号，预热并发 %d", slot, len(uniqueAccounts), limit)

	limiter := make(chan struct{}, limit)
	var wg sync.WaitGroup
	var resultMu sync.Mutex
	successCount := 0
	failureCount := 0
	readyAccounts := make(map[uint]bool)

	for accountID, exchangeAccount := range uniqueAccounts {
		accountID := accountID
		exchangeAccount := exchangeAccount

		wg.Add(1)
		go func() {
			defer wg.Done()

			accountName := exchangeAccountName(&exchangeAccount)
			if accountName == "" {
				accountName = fmt.Sprintf("account-%d", accountID)
			}

			limiter <- struct{}{}
			start := time.Now()
			tokenInfo, err := s.tokenMgr.GetToken(accountID)
			elapsed := time.Since(start).Milliseconds()
			<-limiter

			if err != nil {
				log.Printf("【抢兑调度器】%s JWT 预热失败: 账号=%s, account=%d, 原因=%v, 耗时=%dms", slot, accountName, accountID, err, elapsed)
				resultMu.Lock()
				failureCount++
				resultMu.Unlock()
				return
			}

			if tokenInfo == nil || tokenInfo.JWTToken == "" {
				reason := "JWT 为空"
				if tokenInfo != nil && tokenInfo.ErrorMsg != "" {
					reason = tokenInfo.ErrorMsg
				}
				log.Printf("【抢兑调度器】%s JWT 预热失败: 账号=%s, account=%d, 原因=%s, 耗时=%dms", slot, accountName, accountID, reason, elapsed)
				resultMu.Lock()
				failureCount++
				resultMu.Unlock()
				return
			}

			log.Printf(
				"【抢兑调度器】%s JWT 预热完成: 账号=%s, account=%d, 状态=%s, 过期时间=%s, 耗时=%dms",
				slot,
				accountName,
				accountID,
				tokenInfo.HealthStatus,
				formatExchangeWarmupExpiry(tokenInfo.ExpiresAt),
				elapsed,
			)
			resultMu.Lock()
			successCount++
			readyAccounts[accountID] = true
			resultMu.Unlock()
		}()
	}

	wg.Wait()
	log.Printf("【抢兑调度器】%s JWT 预热完成，成功 %d/%d 个云盘账号，失败 %d 个", slot, successCount, len(uniqueAccounts), failureCount)
	return readyAccounts
}

func filterTasksByReadyAccounts(slot string, tasks []*models.ExchangeTask, readyAccounts map[uint]bool) []*models.ExchangeTask {
	if readyAccounts == nil {
		return tasks
	}

	filtered := make([]*models.ExchangeTask, 0, len(tasks))
	skipped := 0
	for _, task := range tasks {
		if task == nil {
			continue
		}
		accountID := task.ExchangeAccount.AccountID
		if accountID == 0 || !readyAccounts[accountID] {
			skipped++
			log.Printf("【抢兑调度器】%s 跳过任务 %d：账号预热失败或无有效 JWT，不参与本次抢兑", slot, task.ID)
			continue
		}
		filtered = append(filtered, task)
	}
	if skipped > 0 {
		log.Printf("【抢兑调度器】%s 已剔除 %d 个预热失败账号关联任务，剩余 %d 个任务进入抢兑队列", slot, skipped, len(filtered))
	}
	return filtered
}

func formatExchangeWarmupExpiry(expiresAt time.Time) string {
	if expiresAt.IsZero() {
		return "-"
	}
	return expiresAt.Format("2006-01-02 15:04:05")
}
