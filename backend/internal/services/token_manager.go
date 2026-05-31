package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/repository"
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
	preRefreshChan chan uint
	mu             sync.RWMutex
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

const (
	tokenRefreshErrorTTL    = 2 * time.Minute
	maxJWTRefreshFailures   = 3
	tokenRefreshHealthySkew = 2 * time.Minute
)

// NewTokenManager 创建 Token 管理器，并启动后台维护协程。
func NewTokenManager(
	accountRepo *repository.AccountRepository,
	exchangeRepo *repository.ExchangeAccountRepository,
	authMgr *auth.Auth,
) *TokenManager {
	tm := &TokenManager{
		accountRepo:    accountRepo,
		exchangeRepo:   exchangeRepo,
		authMgr:        authMgr,
		preRefreshChan: make(chan uint, 100),
	}

	// 启动预刷新协程（包含扫描与消费预刷新队列）。
	go tm.preRefreshLoop()
	// 启动健康检查协程。
	go tm.healthCheckLoop()

	return tm
}

// GetToken 获取有效 Token（优先走缓存）。
func (tm *TokenManager) GetToken(accountID uint) (*TokenInfo, error) {
	if info, ok := tm.tokenCache.Load(accountID); ok {
		tokenInfo := info.(*TokenInfo)
		// 提前 2 分钟视为即将过期，需要刷新（JWT Token 有效期只有 20-30 分钟）
		if tokenInfo.JWTToken != "" && tokenInfo.HealthStatus == "healthy" && time.Now().Add(tokenRefreshHealthySkew).Before(tokenInfo.ExpiresAt) {
			return tokenInfo, nil
		}
	}

	return tm.refreshToken(accountID)
}

// refreshToken 刷新指定账号 Token，并更新缓存与数据库。
func (tm *TokenManager) refreshToken(accountID uint) (*TokenInfo, error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	// 双重检查，避免并发重复刷新。
	if info, ok := tm.tokenCache.Load(accountID); ok {
		tokenInfo := info.(*TokenInfo)
		if tokenInfo.JWTToken != "" && tokenInfo.HealthStatus == "healthy" && time.Now().Add(tokenRefreshHealthySkew).Before(tokenInfo.ExpiresAt) {
			return tokenInfo, nil
		}
	}

	account, err := tm.accountRepo.GetByID(accountID)
	if err != nil {
		return nil, fmt.Errorf("获取账号失败: %w", err)
	}
	if !account.IsActive {
		return nil, fmt.Errorf("账号已失效，请重新登录后再启用任务")
	}

	// 使用账号 Auth 创建客户端。
	client := corehttp.NewClient()
	authStr := sanitizeAuthValue(account.Auth)
	if authStr != "" {
		client.SetAuth(authStr)
	}

	// 使用账号鉴权信息获取 JWT。
	jwtToken := ""
	authClient := corehttp.NewClient()
	if authStr != "" {
		authClient.SetAuth(authStr)
	}
	authForJWT := auth.NewAuth(authClient)
	ssoToken := ""
	if token, matchedSSOToken, err := authForJWT.GetJWTTokenWithSSOToken(account.Phone); err == nil && token != "" {
		jwtToken = token
		ssoToken = matchedSSOToken
	}

	now := time.Now()
	// JWT Token 缓存时间改为 15 分钟，因为 JWT 本身的有效期只有 20-30 分钟
	tokenInfo := &TokenInfo{
		JWTToken:     jwtToken,
		SSOToken:     ssoToken,
		Auth:         authStr,
		ExpiresAt:    now.Add(15 * time.Minute),
		LastRefresh:  now,
		HealthStatus: "healthy",
	}
	if jwtToken == "" {
		tokenInfo.HealthStatus = "error"
		tokenInfo.ErrorMsg = "无法获取 JWT Token"
		tokenInfo.ExpiresAt = now.Add(tokenRefreshErrorTTL)
		account.JWTErrorCount++
		if account.JWTErrorCount >= maxJWTRefreshFailures {
			account.IsActive = false
			tokenInfo.ErrorMsg = "连续无法获取 JWT Token，账号已暂停，请重新登录后再启用任务"
		}
		_ = tm.accountRepo.Update(account)
	} else if account.JWTErrorCount != 0 {
		account.JWTErrorCount = 0
		_ = tm.accountRepo.Update(account)
	}

	tm.tokenCache.Store(accountID, tokenInfo)

	if jwtToken != "" && account.JWTToken != jwtToken {
		tm.accountRepo.UpdateJWTToken(accountID, jwtToken)
	}

	return tokenInfo, nil
}

// preRefreshLoop 定时扫描即将过期 Token，并消费预刷新队列执行刷新。
func (tm *TokenManager) preRefreshLoop() {
	ticker := time.NewTicker(30 * time.Minute)
	defer ticker.Stop()

	for {
		select {
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

		// Token 在 1 小时内过期时提前刷新。
		if time.Now().Add(1 * time.Hour).After(tokenInfo.ExpiresAt) {
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

	for range ticker.C {
		tm.checkAllTokensHealth()
	}
}

// checkAllTokensHealth 检查所有缓存 Token 的健康状态。
func (tm *TokenManager) checkAllTokensHealth() {
	tm.tokenCache.Range(func(key, value interface{}) bool {
		_ = key.(uint)
		tokenInfo := value.(*TokenInfo)

		if tokenInfo.JWTToken == "" {
			tokenInfo.HealthStatus = "error"
			tokenInfo.ErrorMsg = "Token 为空"
		} else if time.Now().After(tokenInfo.ExpiresAt) {
			tokenInfo.HealthStatus = "error"
			tokenInfo.ErrorMsg = "Token 已过期"
		} else if time.Now().Add(1 * time.Hour).After(tokenInfo.ExpiresAt) {
			tokenInfo.HealthStatus = "warning"
			tokenInfo.ErrorMsg = "Token 即将过期"
		} else {
			tokenInfo.HealthStatus = "healthy"
			tokenInfo.ErrorMsg = ""
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
