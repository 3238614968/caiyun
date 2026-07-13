package services

import (
	"caiyun/internal/envutil"
	"caiyun/internal/models"
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	exchangeSubmitLockPrefix         = "exchange:submit:"
	defaultExchangeRequestsPerSecond = 6
	defaultExchangeRequestBurst      = 2
	defaultExchangeSubmitLockTTL     = 5 * time.Second
	defaultExchangeSubmitLockJanitor = 30 * time.Second
)

type exchangeFailureKind string

const (
	exchangeFailureUnknown            exchangeFailureKind = "unknown"
	exchangeFailureSoldOut            exchangeFailureKind = "sold_out"
	exchangeFailureInsufficientCloud  exchangeFailureKind = "insufficient_cloud"
	exchangeFailureAuth               exchangeFailureKind = "auth"
	exchangeFailureTimeout            exchangeFailureKind = "timeout"
	exchangeFailureRateLimited        exchangeFailureKind = "rate_limited"
	exchangeFailureAlreadyClaimed     exchangeFailureKind = "already_claimed"
	exchangeFailureDuplicateWindow    exchangeFailureKind = "duplicate_window"
	exchangeFailureNetwork            exchangeFailureKind = "network"
	exchangeFailureProductUnavailable exchangeFailureKind = "product_unavailable"
	exchangeFailureInvalidResponse    exchangeFailureKind = "invalid_response"
	exchangeFailureOther              exchangeFailureKind = "other"
)

type exchangeRequestController struct {
	enabled  bool
	tokens   chan struct{}
	ticker   *time.Ticker
	stopCh   chan struct{}
	stopOnce sync.Once
	wg       sync.WaitGroup
}

type localExchangeSubmitLocks struct {
	mu        sync.Mutex
	expires   map[string]time.Time
	stopCh    chan struct{}
	stopOnce  sync.Once
	janitorWG sync.WaitGroup
}

var (
	exchangeControllerOnce sync.Once
	exchangeController     *exchangeRequestController
	exchangeSubmitLocks    = newLocalExchangeSubmitLocks()
)

func performExchangeWithControls(account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager, lockStore exchangeLockStore) (bool, string, int) {
	return performExchangeWithControlsContext(context.Background(), account, prizeID, tokenMgr, lockStore)
}

func performExchangeWithControlsContext(ctx context.Context, account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager, lockStore exchangeLockStore) (bool, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return false, err.Error(), 0
	}
	if locked, reason := acquireExchangeSubmitProtection(lockStore, account, prizeID); !locked {
		return false, reason, 0
	}
	if err := getExchangeRequestController().WaitContext(ctx); err != nil {
		return false, err.Error(), 0
	}
	return performExchangeContext(ctx, account, prizeID, tokenMgr)
}

func getExchangeRequestController() *exchangeRequestController {
	exchangeControllerOnce.Do(func() {
		exchangeController = newExchangeRequestController()
	})
	return exchangeController
}

func newExchangeRequestController() *exchangeRequestController {
	enabled := envutil.Bool("EXCHANGE_REQUEST_PACING_ENABLED", true)
	requestsPerSecond := envutil.Int("EXCHANGE_REQUESTS_PER_SECOND", defaultExchangeRequestsPerSecond)
	burst := envutil.Int("EXCHANGE_REQUEST_BURST", defaultExchangeRequestBurst)

	if !enabled || requestsPerSecond <= 0 {
		return &exchangeRequestController{}
	}
	if burst <= 0 {
		burst = 1
	}

	tokens := make(chan struct{}, burst)
	for i := 0; i < burst; i++ {
		tokens <- struct{}{}
	}

	interval := time.Second / time.Duration(requestsPerSecond)
	if interval <= 0 {
		interval = time.Millisecond
	}

	controller := &exchangeRequestController{
		enabled: true,
		tokens:  tokens,
		ticker:  time.NewTicker(interval),
		stopCh:  make(chan struct{}),
	}
	controller.wg.Add(1)
	go func() {
		defer controller.wg.Done()
		for {
			select {
			case <-controller.stopCh:
				return
			case <-controller.ticker.C:
				select {
				case tokens <- struct{}{}:
				default:
				}
			}
		}
	}()

	return controller
}

func (c *exchangeRequestController) Wait() {
	_ = c.WaitContext(context.Background())
}

func (c *exchangeRequestController) WaitContext(ctx context.Context) error {
	if c == nil || !c.enabled || c.tokens == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-c.stopCh:
		return context.Canceled
	case <-c.tokens:
		return nil
	}
}

func (c *exchangeRequestController) Stop() {
	if c == nil || !c.enabled {
		return
	}
	c.stopOnce.Do(func() {
		if c.ticker != nil {
			c.ticker.Stop()
		}
		if c.stopCh != nil {
			close(c.stopCh)
		}
	})
	c.wg.Wait()
}

func newLocalExchangeSubmitLocks() *localExchangeSubmitLocks {
	locks := &localExchangeSubmitLocks{
		expires: make(map[string]time.Time),
		stopCh:  make(chan struct{}),
	}
	locks.startJanitor(envutil.Duration("EXCHANGE_SUBMIT_LOCK_CLEANUP_INTERVAL", defaultExchangeSubmitLockJanitor))
	return locks
}

func (l *localExchangeSubmitLocks) Acquire(key string, ttl time.Duration) bool {
	if l == nil {
		return true
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	if expiry, ok := l.expires[key]; ok && expiry.After(now) {
		return false
	}
	l.expires[key] = now.Add(ttl)
	return true
}

func (l *localExchangeSubmitLocks) startJanitor(interval time.Duration) {
	if l == nil {
		return
	}
	if interval <= 0 {
		interval = defaultExchangeSubmitLockJanitor
	}
	l.janitorWG.Add(1)
	go func() {
		defer l.janitorWG.Done()
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-l.stopCh:
				return
			case <-ticker.C:
				l.cleanupExpired(time.Now())
			}
		}
	}()
}

func (l *localExchangeSubmitLocks) cleanupExpired(now time.Time) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for existingKey, expiry := range l.expires {
		if !expiry.After(now) {
			delete(l.expires, existingKey)
		}
	}
}

func (l *localExchangeSubmitLocks) Stop() {
	if l == nil {
		return
	}
	l.stopOnce.Do(func() {
		if l.stopCh != nil {
			close(l.stopCh)
		}
	})
	l.janitorWG.Wait()
}

func acquireExchangeSubmitProtection(lockStore exchangeLockStore, account *models.ExchangeAccount, prizeID string) (bool, string) {
	if account == nil || strings.TrimSpace(prizeID) == "" {
		return true, ""
	}

	ttl := envutil.Duration("EXCHANGE_SUBMIT_LOCK_TTL", defaultExchangeSubmitLockTTL)
	if ttl <= 0 {
		ttl = defaultExchangeSubmitLockTTL
	}
	key := exchangeSubmitLockKey(account, prizeID)

	if lockStore == nil {
		if !exchangeSubmitLocks.Acquire(key, ttl) {
			return false, exchangeDuplicateSubmitMessage(ttl)
		}
		return true, ""
	}

	locked, err := lockStore.SetNX(key, strconv.FormatInt(time.Now().UnixNano(), 10), ttl)
	if err != nil {
		log.Printf("【抢兑保护】获取短时幂等锁失败，降级为继续执行: key=%s err=%v", key, err)
		return true, ""
	}
	if !locked {
		return false, exchangeDuplicateSubmitMessage(ttl)
	}
	return true, ""
}

func exchangeSubmitLockKey(account *models.ExchangeAccount, prizeID string) string {
	actorID := account.AccountID
	if actorID == 0 {
		actorID = account.ID
	}
	return fmt.Sprintf("%s%d:%s", exchangeSubmitLockPrefix, actorID, strings.TrimSpace(prizeID))
}

func exchangeDuplicateSubmitMessage(ttl time.Duration) string {
	seconds := int(ttl / time.Second)
	if ttl%time.Second != 0 {
		seconds++
	}
	if seconds <= 0 {
		seconds = 1
	}
	return fmt.Sprintf("幂等保护：%d 秒内已有相同账号的同商品请求正在处理，请稍后重试", seconds)
}

func classifyExchangeFailure(message string) exchangeFailureKind {
	lower := strings.ToLower(strings.TrimSpace(message))
	switch {
	case lower == "":
		return exchangeFailureUnknown
	case strings.Contains(lower, "幂等保护") || strings.Contains(lower, "重复提交"):
		return exchangeFailureDuplicateWindow
	case strings.Contains(lower, "已兑完") || strings.Contains(lower, "已耗尽") || strings.Contains(lower, "售罄") || strings.Contains(lower, "库存") || strings.Contains(lower, "sold"):
		return exchangeFailureSoldOut
	case strings.Contains(lower, "云朵") || strings.Contains(lower, "余额") || strings.Contains(lower, "不足"):
		return exchangeFailureInsufficientCloud
	case strings.Contains(lower, "auth") || strings.Contains(lower, "token") || strings.Contains(lower, "jwt") || strings.Contains(lower, "登录") || strings.Contains(lower, "鉴权"):
		return exchangeFailureAuth
	case strings.Contains(lower, "timeout") || strings.Contains(lower, "超时"):
		return exchangeFailureTimeout
	case strings.Contains(lower, "429") || strings.Contains(lower, "频繁") || strings.Contains(lower, "限流"):
		return exchangeFailureRateLimited
	case strings.Contains(lower, "本月") || strings.Contains(lower, "今日已兑换") || strings.Contains(lower, "已兑换") || strings.Contains(lower, "已领"):
		return exchangeFailureAlreadyClaimed
	case strings.Contains(lower, "下架") || strings.Contains(lower, "不存在") || strings.Contains(lower, "prizeid") || strings.Contains(lower, "更新商品列表"):
		return exchangeFailureProductUnavailable
	case strings.Contains(lower, "解析响应失败") || strings.Contains(lower, "响应格式错误") || strings.Contains(lower, "body="):
		return exchangeFailureInvalidResponse
	case strings.Contains(lower, "请求失败") || strings.Contains(lower, "connection") || strings.Contains(lower, "refused") || strings.Contains(lower, "reset by peer"):
		return exchangeFailureNetwork
	default:
		return exchangeFailureOther
	}
}

func exchangeFailureReasonLabel(message string) string {
	return string(classifyExchangeFailure(message))
}

func isExchangeRetryableFailure(message string) bool {
	switch classifyExchangeFailure(message) {
	case exchangeFailureSoldOut,
		exchangeFailureInsufficientCloud,
		exchangeFailureAuth,
		exchangeFailureAlreadyClaimed,
		exchangeFailureProductUnavailable:
		return false
	default:
		return true
	}
}

func StopExchangeRequestControls() {
	if exchangeController != nil {
		exchangeController.Stop()
	}
	if exchangeSubmitLocks != nil {
		exchangeSubmitLocks.Stop()
	}
}

func resetExchangeRequestControlsForTests() {
	StopExchangeRequestControls()
	exchangeControllerOnce = sync.Once{}
	exchangeController = nil
	exchangeSubmitLocks = newLocalExchangeSubmitLocks()
}
