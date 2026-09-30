package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
)

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
		stopHeartbeat := tm.keepRefreshLockAlive(accountID, lockValue)
		defer stopHeartbeat()
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
		// 到期后的刷新失败才计入停用阈值；单纯过期不会停用账号。
		failed := newTokenInfoFromSession(session, now)
		failed.JWTToken = ""
		tm.updateAccountJWTHealth(account, failed)
		if account.IsActive {
			failed.ErrorMsg = fmt.Sprintf("authorization 刷新失败: %v", err)
		}
		tm.tokenCache.Store(accountID, failed)
		return failed, err
	}
	if session.jwtToken == "" {
		if previous := tm.usablePreviousJWT(account, now); previous != nil {
			if previous.SSOToken == "" {
				if sso, err := session.authForJWT.QuerySpecTokenForJWT(account.Phone); err == nil {
					previous.SSOToken = sso
				}
			}
			previous.Auth = session.authStr
			previous.LastRefresh = now
			previous.HealthStatus = "healthy"
			previous.ErrorMsg = ""
			if account.JWTErrorCount != 0 {
				if err := tm.accountRepo.ResetJWTErrorCount(account.ID); err != nil {
					log.Printf("[TokenManager] 重置账号 %d JWT 错误计数失败: %v", account.ID, err)
				} else {
					account.JWTErrorCount = 0
				}
			}
			tm.tokenCache.Store(accountID, previous)
			return previous, nil
		}
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

func (tm *TokenManager) usablePreviousJWT(account *models.Account, now time.Time) *TokenInfo {
	var token string
	var sso string
	if cached, ok := tm.tokenCache.Load(account.ID); ok {
		if info, ok := cached.(*TokenInfo); ok && info != nil {
			token, sso = info.JWTToken, info.SSOToken
		}
	}
	if token == "" {
		token = account.JWTToken
	}
	jwtExpiry := jwtExpiresAt(token)
	if jwtExpiry.IsZero() || !jwtExpiry.After(now.Add(30*time.Second)) {
		return nil
	}
	validUntil := jwtExpiry
	if accountExpiry := accountAuthorizationExpireAt(account); accountExpiry > now.UnixMilli() {
		if expiresAt := time.UnixMilli(accountExpiry); expiresAt.Before(validUntil) {
			validUntil = expiresAt
		}
	}
	// A failed pre-refresh gets a short grace window, never beyond JWT expiry.
	if grace := now.Add(5 * time.Minute); validUntil.After(grace) {
		validUntil = grace
	}
	if !validUntil.After(now.Add(30 * time.Second)) {
		return nil
	}
	return &TokenInfo{JWTToken: token, SSOToken: sso, ExpiresAt: validUntil}
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
		if accountAuthorizationExpireAt(account) <= now.UnixMilli() {
			return fmt.Errorf("authorization 已到期且刷新失败: %w", err)
		}
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
	info := &TokenInfo{
		JWTToken:     session.jwtToken,
		SSOToken:     session.ssoToken,
		Auth:         session.authStr,
		ExpiresAt:    now.Add(15 * time.Minute),
		LastRefresh:  now,
		HealthStatus: "healthy",
	}
	if expiresAt := jwtExpiresAt(session.jwtToken); !expiresAt.IsZero() && expiresAt.Before(info.ExpiresAt) {
		info.ExpiresAt = expiresAt
	}
	return info
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
	var account *models.Account
	if tm.accountRepo != nil {
		var err error
		account, err = tm.accountRepo.GetByID(accountID)
		if err != nil {
			return nil, fmt.Errorf("核对账号缓存状态失败: %w", err), true
		}
		if !account.IsActive {
			tm.tokenCache.CompareAndDelete(accountID, tokenInfo)
			return nil, fmt.Errorf("账号已停用，请更新凭据或手动启用"), true
		}
		if !tokenCacheMatchesAccount(tokenInfo, account) {
			tm.tokenCache.CompareAndDelete(accountID, tokenInfo)
			return nil, nil, false
		}
	}

	now := time.Now()
	if tokenInfo.JWTToken != "" && tokenInfo.HealthStatus == "healthy" && now.Add(tokenRefreshHealthySkew).Before(tokenInfo.ExpiresAt) {
		return tokenInfo, nil, true
	}
	if tokenInfo.HealthStatus == "error" && now.Before(tokenInfo.ExpiresAt) {
		// Manual re-enable or updated authorization resets the persisted failure
		// count. Other processes must notice that immediately, even with an old
		// negative cache entry.
		if account != nil && account.JWTErrorCount == 0 {
			tm.tokenCache.CompareAndDelete(accountID, tokenInfo)
			return nil, nil, false
		}
		if tokenInfo.ErrorMsg == "" {
			return tokenInfo, fmt.Errorf("Token 暂时不可用"), true
		}
		return tokenInfo, fmt.Errorf("%s", tokenInfo.ErrorMsg), true
	}

	return nil, nil, false
}

func tokenCacheMatchesAccount(info *TokenInfo, account *models.Account) bool {
	return info != nil && account != nil && sanitizeAuthValue(account.Auth) == info.Auth
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

// keepRefreshLockAlive prevents a slow upstream authorization refresh from
// outliving its 30-second distributed lease.  Renewal is ownership-checked in
// Redis, so a process that has lost the lease never extends a successor's key.
func (tm *TokenManager) keepRefreshLockAlive(accountID uint, value string) func() {
	if tm == nil || tm.lockCache == nil || value == "" {
		return func() {}
	}
	stopped := make(chan struct{})
	var once sync.Once
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(tokenRefreshLockRenewInterval(tokenRefreshLockTTL))
		defer ticker.Stop()
		for {
			select {
			case <-tm.ctx.Done():
				return
			case <-stopped:
				return
			case <-ticker.C:
				renewed, err := tm.lockCache.ExtendIfValue(tokenRefreshLockKey(accountID), value, tokenRefreshLockTTL)
				if err != nil {
					log.Printf("[TokenManager] 续租账号 %d 刷新锁失败: %v", accountID, err)
					return
				}
				if !renewed {
					log.Printf("[TokenManager] 账号 %d 刷新锁已失去所有权", accountID)
					return
				}
			}
		}
	}()
	return func() {
		once.Do(func() { close(stopped) })
		wg.Wait()
	}
}

func tokenRefreshLockRenewInterval(ttl time.Duration) time.Duration {
	interval := ttl / 3
	if interval < time.Second {
		return time.Second
	}
	return interval
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
			ssoCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if sso, err := newTokenRefreshSession(account).authForJWT.QuerySpecTokenForJWTContext(ssoCtx, account.Phone); err == nil {
				tokenInfo.SSOToken = sso
			}
			cancel()
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
