package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

// ExecuteMonthlyExchange 执行月卡兑换任务（所有账号）
func (s *ExchangeService) ExecuteMonthlyExchange() {
	_ = s.ExecuteMonthlyExchangeContext(context.Background())
}

// ExecuteMonthlyExchangeContext executes the monthly fan-out under a caller
// context so Worker shutdown cancels semaphore waits and upstream HTTP calls.
func (s *ExchangeService) ExecuteMonthlyExchangeContext(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	locked, releaseRunLock := s.acquireMonthlyExchangeRunLock()
	if !locked {
		return nil
	}
	keepRunLock := false
	defer func() {
		if !keepRunLock {
			releaseRunLock()
		}
	}()

	// 获取所有兑换账号
	accounts, err := s.exchangeAccountRepo.WithContext(ctx).GetAllActive()
	if err != nil {
		// 记录错误日志并发送通知
		log.Printf("【月卡兑换】获取兑换账号列表失败: %v", err)
		s.hub.Broadcast(ws.Message{
			Type: "exchange_error",
			Data: map[string]interface{}{
				"task":    "monthly_exchange",
				"error":   "获取兑换账号列表失败",
				"details": err.Error(),
			},
		})
		return err
	}

	if len(accounts) == 0 {
		log.Println("【月卡兑换】没有活跃的兑换账号，跳过执行")
		return nil
	}

	log.Printf("【月卡兑换】开始执行，共 %d 个账号", len(accounts))

	// 从系统配置获取月卡商品ID，使用常量作为默认值
	monthlyCardPrizeID := constants.DefaultMonthlyCardPrizeID
	if config, err := s.GetSystemConfig("exchange_monthly_prize_id"); err == nil && config.KeyValue != "" {
		monthlyCardPrizeID = config.KeyValue
	}

	// 并发执行兑换
	concurrency := 10
	if config, err := s.GetExchangeConcurrency(); err == nil && config > 0 {
		concurrency = config
	}

	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup

	for _, account := range accounts {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(acc *models.ExchangeAccount) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					accountID := uint(0)
					if acc != nil {
						accountID = acc.ID
					}
					log.Printf("【月卡兑换】账号 %d 执行 panic: %v", accountID, r)
				}
			}()

			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				return
			}
			defer func() { <-semaphore }()

			// 执行月卡兑换
			s.executeMonthlyExchangeForAccountContext(ctx, acc, monthlyCardPrizeID)
		}(account)
	}

	wg.Wait()
	if err := ctx.Err(); err != nil {
		return err
	}
	keepRunLock = true
	return nil
}

// executeMonthlyExchangeForAccount 为单个账号执行月卡兑换
func (s *ExchangeService) executeMonthlyExchangeForAccount(account *models.ExchangeAccount, prizeID string) {
	s.executeMonthlyExchangeForAccountContext(context.Background(), account, prizeID)
}

func (s *ExchangeService) executeMonthlyExchangeForAccountContext(ctx context.Context, account *models.ExchangeAccount, prizeID string) {
	if ctx == nil {
		ctx = context.Background()
	}
	if account == nil {
		return
	}
	if ctx.Err() != nil {
		return
	}
	locked, releaseAccountLock := s.acquireMonthlyExchangeAccountLock(account.ID)
	if !locked {
		return
	}
	keepAccountLock := false
	defer func() {
		if r := recover(); r != nil {
			log.Printf("【月卡兑换】账号 %d 执行 panic: %v", account.ID, r)
		}
		if !keepAccountLock {
			releaseAccountLock()
		}
	}()

	startTime := time.Now()
	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}
	maskedAccountName := maskExchangeAccountName(accountName)

	log.Printf("【月卡兑换】开始为账号 %s 执行兑换", maskedAccountName)

	// 执行兑换
	success, message, execTime := s.doExchangeContext(ctx, account, prizeID)
	if ctx.Err() != nil {
		return
	}
	keepAccountLock = true

	// 查找或创建月卡商品记录
	product, err := s.productRepo.WithContext(ctx).GetByPrizeID(prizeID)
	if err != nil {
		// 如果商品不存在，创建一个临时记录
		product = &models.Product{
			PrizeID:   prizeID,
			PrizeName: "移动云盘月卡",
			POrder:    0,
			Category:  "会员权益",
		}
		log.Printf("【月卡兑换】商品 %s 不存在，使用临时记录", prizeID)
	}

	// 记录兑换结果
	record := &models.ExchangeRecord{
		UserID:            account.UserID,
		ExchangeAccountID: account.ID,
		ProductID:         product.ID,
		PrizeID:           prizeID,
		PrizeName:         product.PrizeName,
		Status:            string(models.ExchangeRecordSuccess),
		Message:           message,
		ExecutionTimeMs:   execTime,
	}

	if !success {
		record.Status = string(models.ExchangeRecordFailed)
		log.Printf("【月卡兑换】账号 %s 兑换失败: %s", maskedAccountName, message)
	} else {
		log.Printf("【月卡兑换】账号 %s 兑换成功，耗时 %d ms", maskedAccountName, execTime)
	}

	if err := s.exchangeRecordRepo.WithContext(ctx).Create(record); err != nil {
		log.Printf("【月卡兑换】记录兑换结果失败: %v", err)
	}

	// 推送 WebSocket 通知
	s.hub.SendToUser(account.UserID, ws.Message{
		Type: "monthly_exchange_complete",
		Data: map[string]interface{}{
			"account_name": accountName,
			"product_name": product.PrizeName,
			"success":      success,
			"message":      message,
			"exec_time":    time.Since(startTime).Milliseconds(),
		},
	})
}

func (s *ExchangeService) acquireMonthlyExchangeRunLock() (bool, func()) {
	if s.lockStore == nil {
		log.Println("【月卡兑换】未配置分布式锁，继续执行（仅建议单实例本地环境）")
		return true, func() {}
	}
	key := monthlyExchangeRunLockKey(time.Now())
	locked, err := s.lockStore.SetNX(key, "1", 24*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】获取全局日级锁失败: %v", err)
		return false, func() {}
	}
	if !locked {
		log.Println("【月卡兑换】今日已执行过，跳过")
		return false, func() {}
	}
	return true, func() {
		if err := s.lockStore.Del(key); err != nil {
			log.Printf("【月卡兑换】释放全局日级锁失败: %v", err)
		}
	}
}

func (s *ExchangeService) acquireMonthlyExchangeAccountLock(accountID uint) (bool, func()) {
	if s.lockStore == nil {
		return true, func() {}
	}
	key := monthlyExchangeAccountLockKey(accountID, time.Now())
	locked, err := s.lockStore.SetNX(key, "1", 25*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】账号 %d 获取日级锁失败: %v", accountID, err)
		return false, func() {}
	}
	if !locked {
		log.Printf("【月卡兑换】账号 %d 今日已处理，跳过", accountID)
		return false, func() {}
	}
	return true, func() {
		if err := s.lockStore.Del(key); err != nil {
			log.Printf("【月卡兑换】账号 %d 释放日级锁失败: %v", accountID, err)
		}
	}
}

func monthlyExchangeRunLockKey(now time.Time) string {
	return "exchange:monthly:run:" + now.Format("2006-01-02")
}

func monthlyExchangeAccountLockKey(accountID uint, now time.Time) string {
	return fmt.Sprintf("exchange:monthly:acc:%d:%s", accountID, now.Format("2006-01-02"))
}
