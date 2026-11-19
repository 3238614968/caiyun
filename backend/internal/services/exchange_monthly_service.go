package services

import (
	"caiyun/internal/constants"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/ws"
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// ExecuteMonthlyExchangeContext executes the monthly fan-out under a caller
// context so Worker shutdown cancels semaphore waits and upstream HTTP calls.
func (s *ExchangeService) ExecuteMonthlyExchangeContext(ctx context.Context, scheduled bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if exchangeOperationContextID(ctx) == "" {
		return fmt.Errorf("%w: 月卡兑换必须关联持久化操作", ErrExchangeInvalidInput)
	}
	guard := s.monthlySubmissionGuard(scheduled)
	if err := guard(ctx); err != nil {
		return err
	}
	locked, releaseRunLock := s.acquireMonthlyExchangeRunLock()
	if !locked {
		return fmt.Errorf("%w: 月卡兑换已有运行或今日已完成，请先核对记录", ErrExchangeTaskConflict)
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
		s.broadcast(ws.Message{
			Type: "exchange_error",
			Data: map[string]interface{}{
				"task":  "monthly_exchange",
				"error": "获取兑换账号列表失败",
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
	results := make(chan monthlyExchangeResult, len(accounts))
	var wg sync.WaitGroup

	for _, account := range accounts {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		go func(acc *models.ExchangeAccount) {
			defer wg.Done()
			defer func() {
				if recovered := recover(); recovered != nil {
					results <- monthlyExchangeResult{accountID: acc.ID, outcome: models.ExchangeOutcomeUnknown, message: fmt.Sprintf("月卡账号执行异常: %v", recovered)}
				}
			}()
			select {
			case semaphore <- struct{}{}:
			case <-ctx.Done():
				results <- monthlyExchangeResult{accountID: acc.ID, outcome: models.ExchangeOutcomeNotSubmitted, message: ctx.Err().Error()}
				return
			}
			defer func() { <-semaphore }()
			results <- s.executeMonthlyExchangeForAccountContext(ctx, acc, monthlyCardPrizeID, guard)
		}(account)
	}
	wg.Wait()
	close(results)
	var failures []string
	for result := range results {
		if result.outcome != models.ExchangeOutcomeAccepted {
			failures = append(failures, fmt.Sprintf("账号%d：%s", result.accountID, result.message))
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("%w: 月卡兑换未全部完成：%s", ErrExchangeBatchPartialFailure, strings.Join(failures, "；"))
	}

	if err := ctx.Err(); err != nil {
		return err
	}
	keepRunLock = true
	return nil
}

func (s *ExchangeService) monthlySubmissionGuard(scheduled bool) exchangeSubmissionGuard {
	return func(ctx context.Context) error {
		if s.configRepo == nil {
			return fmt.Errorf("兑换开关存储不可用")
		}
		enabled, err := s.ExchangeEnabled(ctx)
		if err != nil {
			return fmt.Errorf("读取抢兑开关失败: %w", err)
		}
		if !enabled {
			return fmt.Errorf("%w: 抢兑功能已关闭", ErrExchangeTaskConflict)
		}
		if scheduled {
			enabled, err = s.MonthlyExchangeEnabled(ctx)
			if err != nil {
				return fmt.Errorf("读取月卡自动兑换开关失败: %w", err)
			}
			if !enabled {
				return fmt.Errorf("%w: 月卡自动兑换已关闭", ErrExchangeTaskConflict)
			}
		}
		return nil
	}
}

type monthlyExchangeResult struct {
	accountID uint
	outcome   models.ExchangeOutcome
	message   string
	execTime  int
	skipped   bool
}

func (s *ExchangeService) monthlyReceipt(ctx context.Context, account *models.ExchangeAccount, product *models.Product) (*repository.ExchangeExecutionReceipt, error) {
	if s.exchangeReceiptRepo == nil || account == nil {
		return nil, fmt.Errorf("月卡执行回执存储不可用")
	}
	operationID := exchangeOperationContextID(ctx)
	if operationID == "" {
		return nil, fmt.Errorf("%w: 月卡回执缺少持久化操作 ID", ErrExchangeInvalidInput)
	}
	receipt, err := s.exchangeReceiptRepo.WithContext(ctx).FindByOperationResource(operationID, exchangeAccountReceiptKind, account.ID)
	if err != nil {
		return nil, err
	}
	if receipt == nil {
		receipt = &repository.ExchangeExecutionReceipt{OperationID: operationID, ResourceID: account.ID, ResourceKind: exchangeAccountReceiptKind, UserID: account.UserID, ProductID: product.ID}
	}
	return receipt, nil
}

func (s *ExchangeService) executeMonthlyExchangeForAccountContext(ctx context.Context, account *models.ExchangeAccount, prizeID string, guard exchangeSubmissionGuard) monthlyExchangeResult {
	result := monthlyExchangeResult{outcome: models.ExchangeOutcomeNotSubmitted}
	if account == nil || account.ID == 0 {
		result.message = "月卡账号为空"
		return result
	}
	result.accountID = account.ID
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		result.message = err.Error()
		return result
	}
	locked, releaseAccountLock := s.acquireMonthlyExchangeAccountLock(account.ID)
	if !locked {
		result.skipped = true
		result.message = "账号今日已有月卡执行或已完成，跳过"
		return result
	}
	keepAccountLock := false
	defer func() {
		if !keepAccountLock {
			releaseAccountLock()
		}
	}()
	startTime := time.Now()
	product, err := s.productRepo.WithContext(ctx).GetByPrizeID(prizeID)
	if err != nil {
		product = &models.Product{PrizeID: prizeID, PrizeName: "移动云盘月卡", POrder: 0, Category: "会员权益"}
	}
	receipt, err := s.monthlyReceipt(ctx, account, product)
	if err != nil {
		result.message = err.Error()
		return result
	}
	if receipt.CompletedAt != nil && receipt.Outcome != string(models.ExchangeOutcomeUnknown) {
		result.outcome = models.ExchangeOutcome(receipt.Outcome)
		result.message = receipt.Message
		return result
	}
	if receipt.Outcome == string(models.ExchangeOutcomeUnknown) {
		result.outcome = models.ExchangeOutcomeUnknown
		result.message = "兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
		return result
	}
	if err := s.exchangeReceiptRepo.WithContext(ctx).StartSubmission(receipt); err != nil {
		result.message = fmt.Sprintf("提交边界持久化失败，未提交兑换: %v", err)
		return result
	}
	keepAccountLock = true
	upstream := performExchangeWithControlsContext(ctx, account, prizeID, s.tokenMgr, s.lockStore, &exchangePreparedSession{}, guard)
	result = s.finalizeMonthlyExchange(ctx, account, product, receipt, upstream, int(time.Since(startTime).Milliseconds()))
	if upstream.guardErr != nil && result.outcome == models.ExchangeOutcomeNotSubmitted {
		keepAccountLock = false
	}
	return result
}

func (s *ExchangeService) finalizeMonthlyExchange(ctx context.Context, account *models.ExchangeAccount, product *models.Product, receipt *repository.ExchangeExecutionReceipt, upstream exchangeAttemptResult, execTime int) monthlyExchangeResult {
	result := monthlyExchangeResult{accountID: account.ID, outcome: upstream.outcome, message: upstream.message, execTime: execTime}
	record := &models.ExchangeRecord{
		UserID: account.UserID, ExchangeAccountID: account.ID, ProductID: product.ID,
		PrizeID: product.PrizeID, PrizeName: product.PrizeName, Status: string(models.ExchangeRecordFailed),
		Outcome: string(upstream.outcome), Message: upstream.message, ExecutionTimeMs: result.execTime,
	}
	if upstream.success {
		record.Status = string(models.ExchangeRecordSuccess)
	}
	receipt.Outcome = string(upstream.outcome)
	receipt.Message = upstream.message
	receipt.ExecutionTimeMs = result.execTime
	now := time.Now()
	receipt.CompletedAt = &now
	if err := s.exchangeReceiptRepo.WithContext(ctx).Finalize(receipt, record); err != nil {
		// The upstream may already have accepted this request. Keep the account
		// protection even when its local result could not be persisted.
		result.outcome = models.ExchangeOutcomeUnknown
		result.message = fmt.Sprintf("月卡结果保存失败，需核对兑换记录，暂勿重复兑换: %v", err)
		return result
	}
	accountName := account.Remark
	if accountName == "" {
		accountName = account.Phone
	}
	s.sendToUser(account.UserID, ws.Message{Type: "monthly_exchange_complete", Data: map[string]interface{}{
		"account_name": accountName, "product_name": product.PrizeName, "success": upstream.success,
		"outcome": string(upstream.outcome), "message": upstream.message, "exec_time": result.execTime,
	}})
	return result
}

func (s *ExchangeService) acquireMonthlyExchangeRunLock() (bool, func()) {
	if s.lockStore == nil {
		log.Println("【月卡兑换】未配置分布式锁，继续执行（仅建议单实例本地环境）")
		return true, func() {}
	}
	key := monthlyExchangeRunLockKey(time.Now())
	owner := randomLockValue(0)
	locked, err := s.lockStore.SetNX(key, owner, 24*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】获取全局日级锁失败: %v", err)
		return false, func() {}
	}
	if !locked {
		log.Println("【月卡兑换】今日已执行过，跳过")
		return false, func() {}
	}
	return true, func() {
		if _, err := s.lockStore.DelIfValue(key, owner); err != nil {
			log.Printf("【月卡兑换】释放全局日级锁失败: %v", err)
		}
	}
}

func (s *ExchangeService) acquireMonthlyExchangeAccountLock(accountID uint) (bool, func()) {
	if s.lockStore == nil {
		return true, func() {}
	}
	key := monthlyExchangeAccountLockKey(accountID, time.Now())
	owner := randomLockValue(0)
	locked, err := s.lockStore.SetNX(key, owner, 25*time.Hour)
	if err != nil {
		log.Printf("【月卡兑换】账号 %d 获取日级锁失败: %v", accountID, err)
		return false, func() {}
	}
	if !locked {
		log.Printf("【月卡兑换】账号 %d 今日已处理，跳过", accountID)
		return false, func() {}
	}
	return true, func() {
		if _, err := s.lockStore.DelIfValue(key, owner); err != nil {
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
