package services

import (
	"log"
	"strings"
	"time"
	"unicode"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/envutil"
)

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
	maxScan := tokenPreRefreshMaxScanFromEnv()
	readyBefore := time.Now().Add(tokenPreRefreshSkew)
	queued := 0

	tm.tokenCache.Range(func(key, value interface{}) bool {
		if maxScan > 0 && queued >= maxScan {
			return false
		}

		accountID := key.(uint)
		tokenInfo := value.(*TokenInfo)
		if tokenInfo == nil || tokenInfo.JWTToken == "" || tokenInfo.HealthStatus == "error" {
			return true
		}

		// Token 即将过期时提前刷新。
		if readyBefore.After(tokenInfo.ExpiresAt) {
			select {
			case tm.preRefreshChan <- accountID:
				queued++
			default:
				// 通道已满时提前结束本轮扫描，避免无意义遍历。
				return false
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

func tokenPreRefreshMaxScanFromEnv() int {
	value := envutil.Int("TOKEN_PREREFRESH_MAX_SCAN", defaultTokenPreRefreshMaxScan)
	if value <= 0 {
		return defaultTokenPreRefreshMaxScan
	}
	return value
}

// sanitizeAuthValue 清理认证值中的控制字符，保留合法的 Unicode / Base64 / JWT 内容。
func sanitizeAuthValue(authValue string) string {
	var b strings.Builder
	b.Grow(len(authValue))
	for _, c := range authValue {
		if unicode.IsControl(c) {
			continue
		}
		b.WriteRune(c)
	}
	return strings.TrimSpace(b.String())
}
