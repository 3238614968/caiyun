package services

import (
	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"
)

// TokenManager 统一管理账号 JWT Token 的缓存、刷新与健康状态。
type TokenManager struct {
	accountRepo  *repository.AccountRepository
	exchangeRepo *repository.ExchangeAccountRepository
	authMgr      *auth.Auth

	tokenCache     sync.Map // map[uint]*TokenInfo
	accountLocks   sync.Map // map[uint]*sync.Mutex
	preRefreshChan chan uint
	lockCache      *cache.RedisCache
	ctx            context.Context
	cancel         context.CancelFunc
}

// TokenInfo 描述账号当前 Token 状态。
type TokenInfo struct {
	JWTToken     string
	SSOToken     string
	Auth         string
	ExpiresAt    time.Time
	LastRefresh  time.Time
	HealthStatus string // healthy, warning, error
	ErrorMsg     string
}

type tokenRefreshSession struct {
	authStr    string
	authForJWT *auth.Auth
	jwtToken   string
	ssoToken   string
}

const (
	tokenRefreshErrorTTL    = 2 * time.Minute
	maxJWTRefreshFailures   = 3
	tokenRefreshHealthySkew = 2 * time.Minute
	tokenPreRefreshSkew     = 5 * time.Minute
	tokenRefreshLockTTL     = 30 * time.Second
	tokenRefreshLockWait    = 10 * time.Second
	tokenRefreshPollDelay   = 500 * time.Millisecond
)

// NewTokenManager 创建 Token 管理器，并启动后台维护协程。
func NewTokenManager(
	accountRepo *repository.AccountRepository,
	exchangeRepo *repository.ExchangeAccountRepository,
	authMgr *auth.Auth,
) *TokenManager {
	ctx, cancel := context.WithCancel(context.Background())
	tm := &TokenManager{
		accountRepo:    accountRepo,
		exchangeRepo:   exchangeRepo,
		authMgr:        authMgr,
		preRefreshChan: make(chan uint, 100),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 启动预刷新协程（包含扫描与消费预刷新队列）。
	go tm.preRefreshLoop()
	// 启动健康检查协程。
	go tm.healthCheckLoop()

	return tm
}

// SetDistributedLockCache 启用 Redis 分布式刷新锁，避免多副本同时刷新同一账号 Token。
func (tm *TokenManager) SetDistributedLockCache(redisCache *cache.RedisCache) {
	if tm == nil {
		return
	}
	tm.lockCache = redisCache
}

// Stop 停止 TokenManager 后台维护协程。
func (tm *TokenManager) Stop() {
	if tm == nil || tm.cancel == nil {
		return
	}
	tm.cancel()
}

// GetToken 获取有效 Token（优先走缓存）。
func (tm *TokenManager) GetToken(accountID uint) (*TokenInfo, error) {
	if tokenInfo, err, ok := tm.cachedToken(accountID); ok {
		return tokenInfo, err
	}

	return tm.refreshToken(accountID)
}

// refreshToken 刷新指定账号 Token，并更新缓存与数据库。
func (tm *TokenManager) refreshToken(accountID uint) (*TokenInfo, error) {
	lock := tm.accountLock(accountID)
	lock.Lock()
	defer lock.Unlock()

	// 双重检查，避免并发重复刷新。
	if tokenInfo, err, ok := tm.cachedToken(accountID); ok {
		return tokenInfo, err
	}

	lockValue, locked, err := tm.acquireOrWaitRefreshLock(accountID, time.Now())
	if err != nil {
		return nil, err
	}
	if !locked {
		if tokenInfo, err, ok := tm.cachedToken(accountID); ok {
			return tokenInfo, err
		}
	}
	if locked {
		defer tm.releaseRefreshLock(accountID, lockValue)
	}

	return tm.refreshAccountToken(accountID)
}

func (tm *TokenManager) acquireOrWaitRefreshLock(accountID uint, refreshStartedAt time.Time) (string, bool, error) {
	lockValue, locked, lockErr := tm.acquireRefreshLock(accountID)
	if lockErr != nil {
		log.Printf("[TokenManager] 获取账号 %d 分布式刷新锁失败，降级为进程内锁: %v", accountID, lockErr)
		return lockValue, locked, nil
	}
	if tm.lockCache == nil || locked {
		return lockValue, locked, nil
	}

	if tokenInfo, err := tm.waitForExternalRefresh(accountID, refreshStartedAt); err == nil {
		tm.tokenCache.Store(accountID, tokenInfo)
		return "", false, nil
	} else {
		log.Printf("[TokenManager] 等待账号 %d 外部刷新失败，尝试重新抢锁: %v", accountID, err)
	}

	lockValue, locked, lockErr = tm.acquireRefreshLock(accountID)
	if lockErr != nil {
		log.Printf("[TokenManager] 重新获取账号 %d 分布式刷新锁失败，降级为进程内锁: %v", accountID, lockErr)
		return lockValue, locked, nil
	}
	if !locked {
		return "", false, fmt.Errorf("账号 %d Token 正在其他实例刷新，请稍后重试", accountID)
	}
	return lockValue, locked, nil
}

func (tm *TokenManager) refreshAccountToken(accountID uint) (*TokenInfo, error) {
	account, err := tm.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("获取账号失败: %w", err)
	}
	if !account.IsActive {
		return nil, fmt.Errorf("账号已失效，请重新登录后再启用任务")
	}

	session := newTokenRefreshSession(account)
	session.refreshJWT(account.Phone)

	now := time.Now()
	if err := tm.refreshAuthorizationIfNeeded(account, session, now); err != nil {
		return nil, err
	}

	tokenInfo := newTokenInfoFromSession(session, now)
	tm.updateAccountJWTHealth(account, tokenInfo)

	if session.jwtToken != "" && account.JWTToken != session.jwtToken {
		if err := tm.accountRepo.UpdateJWTToken(accountID, session.jwtToken); err != nil {
			tm.tokenCache.Delete(accountID)
			return nil, fmt.Errorf("更新账号 JWT Token 失败: %w", err)
		}
	}

	tm.tokenCache.Store(accountID, tokenInfo)
	return tokenInfo, nil
}

func newTokenRefreshSession(account *models.Account) *tokenRefreshSession {
	authStr := sanitizeAuthValue(account.Auth)
	authClient := corehttp.NewClient()
	if authStr != "" {
		authClient.SetAuth(authStr)
	}
	return &tokenRefreshSession{
		authStr:    authStr,
		authForJWT: auth.NewAuth(authClient),
	}
}

func (s *tokenRefreshSession) refreshJWT(phone string) {
	if s == nil || s.authForJWT == nil {
		return
	}
	if token, matchedSSOToken, err := s.authForJWT.GetJWTTokenWithSSOToken(phone); err == nil && token != "" {
		s.jwtToken = token
		s.ssoToken = matchedSSOToken
	}
}

func (tm *TokenManager) refreshAuthorizationIfNeeded(account *models.Account, session *tokenRefreshSession, now time.Time) error {
	if !authorizationShouldRefresh(accountAuthorizationExpireAt(account), now) {
		return nil
	}

	userDomainID := jwtUserDomainID(session.jwtToken)
	refreshed, err := session.authForJWT.RefreshAuthorization(account.Auth, account.Phone, userDomainID)
	if err != nil {
		log.Printf("[TokenManager] 账号 %d authorization 刷新失败，保留原数据库记录: %v", account.ID, err)
		return nil
	}

	if refreshed.SSOToken != "" {
		if token, jwtErr := session.authForJWT.TyrzLogin(refreshed.SSOToken); jwtErr == nil && token != "" {
			session.jwtToken = token
			session.ssoToken = refreshed.SSOToken
		} else if jwtErr != nil {
			log.Printf("[TokenManager] 账号 %d authorization 刷新成功但 JWT 重取失败，将保留已有 JWT: %v", account.ID, jwtErr)
		}
	}

	applyAuthorizationRefreshToAccount(account, refreshed, session.jwtToken)
	if err := tm.accountRepo.UpdateAuthorizationFields(account.ID, account.Auth, account.Token, account.JWTToken, account.Platform, account.ExpireAt); err != nil {
		return fmt.Errorf("更新刷新后的 authorization 失败: %w", err)
	}
	if tm.exchangeRepo != nil {
		if err := tm.exchangeRepo.UpdateAuthByAccountID(account.ID, account.Auth, account.Token, account.JWTToken); err != nil {
			log.Printf("[TokenManager] 同步刷新后的抢兑账号鉴权失败 account_id=%d: %v", account.ID, err)
		}
	}
	session.authStr = sanitizeAuthValue(account.Auth)
	log.Printf("[TokenManager] 账号 %d authorization 已刷新并写入数据库", account.ID)
	return nil
}

func newTokenInfoFromSession(session *tokenRefreshSession, now time.Time) *TokenInfo {
	return &TokenInfo{
		JWTToken:     session.jwtToken,
		SSOToken:     session.ssoToken,
		Auth:         session.authStr,
		ExpiresAt:    now.Add(15 * time.Minute),
		LastRefresh:  now,
		HealthStatus: "healthy",
	}
}

func (tm *TokenManager) updateAccountJWTHealth(account *models.Account, tokenInfo *TokenInfo) {
	if tokenInfo.JWTToken == "" {
		tokenInfo.HealthStatus = "error"
		tokenInfo.ErrorMsg = "无法获取 JWT Token"
		tokenInfo.ExpiresAt = time.Now().Add(tokenRefreshErrorTTL)

		newCount, err := tm.accountRepo.IncrementJWTErrorCount(account.ID)
		if err != nil {
			log.Printf("[TokenManager] 账号 %d JWT 错误计数自增失败: %v", account.ID, err)
			newCount = account.JWTErrorCount + 1
		}
		account.JWTErrorCount = newCount
		if newCount >= maxJWTRefreshFailures {
			account.IsActive = false
			tokenInfo.ErrorMsg = "连续无法获取 JWT Token，账号已暂停，请重新登录后再启用任务"
			if err := tm.accountRepo.SetActiveStatus(account.ID, false); err != nil {
				log.Printf("[TokenManager] 账号 %d 自动暂停失败: %v", account.ID, err)
			}
		}
		return
	}
	if account.JWTErrorCount != 0 {
		account.JWTErrorCount = 0
		if err := tm.accountRepo.ResetJWTErrorCount(account.ID); err != nil {
			log.Printf("[TokenManager] 账号 %d JWT 错误计数重置失败: %v", account.ID, err)
		}
	}
}

func (tm *TokenManager) cachedToken(accountID uint) (*TokenInfo, error, bool) {
	info, ok := tm.tokenCache.Load(accountID)
	if !ok {
		return nil, nil, false
	}
	tokenInfo, ok := info.(*TokenInfo)
	if !ok || tokenInfo == nil {
		tm.tokenCache.Delete(accountID)
		return nil, nil, false
	}

	now := time.Now()
	if tokenInfo.JWTToken != "" && tokenInfo.HealthStatus == "healthy" && now.Add(tokenRefreshHealthySkew).Before(tokenInfo.ExpiresAt) {
		return tokenInfo, nil, true
	}
	if tokenInfo.HealthStatus == "error" && now.Before(tokenInfo.ExpiresAt) {
		if tokenInfo.ErrorMsg == "" {
			tokenInfo.ErrorMsg = "Token 暂时不可用"
		}
		return tokenInfo, fmt.Errorf("%s", tokenInfo.ErrorMsg), true
	}

	return nil, nil, false
}

func (tm *TokenManager) accountLock(accountID uint) *sync.Mutex {
	lock, _ := tm.accountLocks.LoadOrStore(accountID, &sync.Mutex{})
	return lock.(*sync.Mutex)
}

func (tm *TokenManager) acquireRefreshLock(accountID uint) (string, bool, error) {
	if tm == nil || tm.lockCache == nil {
		return "", false, nil
	}

	value := randomLockValue(accountID)
	ok, err := tm.lockCache.SetNX(tokenRefreshLockKey(accountID), value, tokenRefreshLockTTL)
	if err != nil {
		return "", false, err
	}
	return value, ok, nil
}

func (tm *TokenManager) releaseRefreshLock(accountID uint, value string) {
	if tm == nil || tm.lockCache == nil || value == "" {
		return
	}
	if _, err := tm.lockCache.DelIfValue(tokenRefreshLockKey(accountID), value); err != nil {
		log.Printf("[TokenManager] 释放账号 %d 分布式刷新锁失败: %v", accountID, err)
	}
}

func (tm *TokenManager) waitForExternalRefresh(accountID uint, since time.Time) (*TokenInfo, error) {
	if tm == nil {
		return nil, fmt.Errorf("TokenManager 为空")
	}

	deadline := time.NewTimer(tokenRefreshLockWait)
	defer deadline.Stop()
	ticker := time.NewTicker(tokenRefreshPollDelay)
	defer ticker.Stop()

	for {
		select {
		case <-tm.ctx.Done():
			return nil, fmt.Errorf("TokenManager 已停止")
		case <-deadline.C:
			return nil, fmt.Errorf("等待刷新超时")
		case <-ticker.C:
			if tokenInfo, err, ok := tm.cachedToken(accountID); ok && err == nil && tokenInfo.JWTToken != "" {
				return tokenInfo, nil
			}

			account, err := tm.accountRepo.GetByID(accountID)
			if err != nil {
				continue
			}
			if account.JWTToken == "" || account.UpdatedAt.Before(since.Add(-1*time.Second)) {
				continue
			}

			tokenInfo := &TokenInfo{
				JWTToken:     account.JWTToken,
				Auth:         sanitizeAuthValue(account.Auth),
				ExpiresAt:    time.Now().Add(15 * time.Minute),
				LastRefresh:  account.UpdatedAt,
				HealthStatus: "healthy",
			}
			tm.tokenCache.Store(accountID, tokenInfo)
			return tokenInfo, nil
		}
	}
}

func tokenRefreshLockKey(accountID uint) string {
	return fmt.Sprintf("token:refresh:lock:%d", accountID)
}

func randomLockValue(accountID uint) string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("%d:%d", accountID, time.Now().UnixNano())
	}
	return fmt.Sprintf("%d:%s", accountID, hex.EncodeToString(buf[:]))
}

// preRefreshLoop 定时扫描即将过期 Token，并消费预刷新队列执行刷新。
func (tm *TokenManager) preRefreshLoop() {
	ticker := time.NewTicker(2 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-tm.ctx.Done():
			return
		case <-ticker.C:
			tm.preRefreshExpiredTokens()
		case accountID := <-tm.preRefreshChan:
			if _, err := tm.refreshToken(accountID); err != nil {
				log.Printf("[TokenManager] 预刷新账号 %d 失败: %v", accountID, err)
			}
		}
	}
}

// preRefreshExpiredTokens 将即将过期的 Token 放入预刷新队列。
func (tm *TokenManager) preRefreshExpiredTokens() {
	tm.tokenCache.Range(func(key, value interface{}) bool {
		accountID := key.(uint)
		tokenInfo := value.(*TokenInfo)

		if tokenInfo == nil || tokenInfo.JWTToken == "" || tokenInfo.HealthStatus == "error" {
			return true
		}

		// Token 即将过期时提前刷新。
		if time.Now().Add(tokenPreRefreshSkew).After(tokenInfo.ExpiresAt) {
			select {
			case tm.preRefreshChan <- accountID:
			default:
				// 通道已满时跳过，避免阻塞扫描。
			}
		}
		return true
	})
}

// healthCheckLoop 定时更新缓存中 Token 的健康状态。
func (tm *TokenManager) healthCheckLoop() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-tm.ctx.Done():
			return
		case <-ticker.C:
			tm.checkAllTokensHealth()
		}
	}
}

// checkAllTokensHealth 检查所有缓存 Token 的健康状态，并清理长期过期/错误的条目。
func (tm *TokenManager) checkAllTokensHealth() {
	tm.tokenCache.Range(func(key, value interface{}) bool {
		accountID, _ := key.(uint)
		tokenInfo, ok := value.(*TokenInfo)
		if !ok || tokenInfo == nil {
			tm.tokenCache.Delete(accountID)
			return true
		}

		if tokenInfo.JWTToken == "" {
			tokenInfo.HealthStatus = "error"
			tokenInfo.ErrorMsg = "Token 为空"
		} else if time.Now().After(tokenInfo.ExpiresAt) {
			tokenInfo.HealthStatus = "error"
			tokenInfo.ErrorMsg = "Token 已过期"
		} else if time.Now().Add(tokenRefreshHealthySkew).After(tokenInfo.ExpiresAt) {
			tokenInfo.HealthStatus = "warning"
			tokenInfo.ErrorMsg = "Token 即将过期"
		} else {
			tokenInfo.HealthStatus = "healthy"
			tokenInfo.ErrorMsg = ""
		}

		// 清理超过 tokenRefreshErrorTTL 的 error 条目，避免 sync.Map 无限增长。
		if tokenInfo.HealthStatus == "error" && time.Since(tokenInfo.LastRefresh) > 30*time.Minute {
			tm.tokenCache.Delete(accountID)
		}

		return true
	})
}

// GetHealthyTokenCount 获取健康 Token 数量。
func (tm *TokenManager) GetHealthyTokenCount() int {
	count := 0
	tm.tokenCache.Range(func(key, value interface{}) bool {
		tokenInfo := value.(*TokenInfo)
		if tokenInfo.HealthStatus == "healthy" {
			count++
		}
		return true
	})
	return count
}

// GetTokenStats 获取 Token 统计信息。
func (tm *TokenManager) GetTokenStats() map[string]interface{} {
	stats := map[string]interface{}{
		"total":      0,
		"healthy":    0,
		"warning":    0,
		"error":      0,
		"cache_size": 0,
	}

	tm.tokenCache.Range(func(key, value interface{}) bool {
		tokenInfo := value.(*TokenInfo)
		stats["total"] = stats["total"].(int) + 1
		stats["cache_size"] = stats["total"]

		switch tokenInfo.HealthStatus {
		case "healthy":
			stats["healthy"] = stats["healthy"].(int) + 1
		case "warning":
			stats["warning"] = stats["warning"].(int) + 1
		case "error":
			stats["error"] = stats["error"].(int) + 1
		}
		return true
	})

	return stats
}

// ClearToken 清除指定账号的 Token 缓存。
func (tm *TokenManager) ClearToken(accountID uint) {
	tm.tokenCache.Delete(accountID)
}

// ForceRefresh 强制刷新指定账号 Token。
func (tm *TokenManager) ForceRefresh(accountID uint) (*TokenInfo, error) {
	tm.tokenCache.Delete(accountID)
	return tm.refreshToken(accountID)
}

// CreateAuthenticatedClient 创建带认证信息的 HTTP 客户端。
func (tm *TokenManager) CreateAuthenticatedClient(accountID uint, auth string) (*corehttp.Client, error) {
	tokenInfo, err := tm.GetToken(accountID)
	if err != nil {
		return nil, err
	}

	client := corehttp.NewClient()

	authStr := sanitizeAuthValue(auth)
	if authStr != "" {
		client.SetAuth(authStr)
	}

	if tokenInfo.JWTToken != "" {
		client.SetJWTToken(tokenInfo.JWTToken)
	}
	if tokenInfo.SSOToken != "" {
		client.SetSSOToken(tokenInfo.SSOToken)
	}

	return client, nil
}

// sanitizeAuthValue 清理认证值中的非法字符，只保留可打印 ASCII。
func sanitizeAuthValue(authValue string) string {
	var b strings.Builder
	b.Grow(len(authValue))
	for _, c := range authValue {
		if c >= 32 && c < 127 {
			b.WriteRune(c)
		}
	}
	return strings.TrimSpace(b.String())
}
