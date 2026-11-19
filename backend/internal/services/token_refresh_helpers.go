package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
	"strings"
	"time"
)

func jwtExpiresAt(token string) time.Time {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) != 3 {
		return time.Time{}
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return time.Time{}
	}
	var claims struct {
		ExpiresAt int64 `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil || claims.ExpiresAt <= 0 {
		return time.Time{}
	}
	if claims.ExpiresAt > 1_000_000_000_000 {
		return time.UnixMilli(claims.ExpiresAt)
	}
	return time.Unix(claims.ExpiresAt, 0)
}

func accountAuthorizationExpireAt(account *models.Account) int64 {
	if account == nil {
		return 0
	}
	if account.ExpireAt > 0 {
		return account.ExpireAt
	}
	if info, err := auth.ParseToken(account.Auth); err == nil && info != nil {
		return info.Expire
	}
	return 0
}

// authorizationRefreshLead 提前续期窗口：凭据在到期前该时长内即触发轮换。
//
// 背景（2026-11 实测）：Authorization 有效期约 30 天；硬过期后 querySpecToken 回
// 1010010015（非 05050006），且 refreshToken 拿过期凭据会被服务端拒绝，只能走
// 重新登录。旧策略「到期才刷」导致每月凭据到期日（各账号均为 20 几号）连续 3 次
// JWT 失败后账号被自动停用。提前在有效期内轮换可无限续链。
// CAIYUN_AUTH_REFRESH_LEAD_HOURS=0 关闭提前续期（回到旧的到期后行为）。
func authorizationRefreshLead() time.Duration {
	raw := strings.TrimSpace(os.Getenv("CAIYUN_AUTH_REFRESH_LEAD_HOURS"))
	if raw == "" {
		return 72 * time.Hour
	}
	hours, err := strconv.ParseFloat(raw, 64)
	if err != nil || hours <= 0 {
		return 0
	}
	return time.Duration(hours * float64(time.Hour))
}

func authorizationShouldRefresh(expireAt int64, now time.Time) bool {
	if expireAt <= 0 {
		return true
	}
	if expireAt <= now.UnixMilli() {
		return true
	}
	return expireAt <= now.Add(authorizationRefreshLead()).UnixMilli()
}

// authorizationShouldRotateEarly 判定「JWT 兑换成功时」是否仍应主动轮换：
// 只有到期时间落在未来 lead 窗口内才轮换；已过去或为 0 的到期时间属于
// 陈旧/缺失元数据（凭据实际可用），不作为主动轮换依据。
func authorizationShouldRotateEarly(expireAt int64, now time.Time) bool {
	if expireAt <= now.UnixMilli() {
		return false
	}
	return expireAt <= now.Add(authorizationRefreshLead()).UnixMilli()
}

func applyAuthorizationRefreshToAccount(account *models.Account, refreshed *auth.AuthorizationRefreshResult, jwtToken string) {
	if account == nil || refreshed == nil {
		return
	}
	account.Auth = refreshed.Auth
	account.Token = refreshed.Token
	account.ExpireAt = refreshed.ExpireAt
	if refreshed.Platform != "" {
		account.Platform = refreshed.Platform
	}
	if jwtToken != "" {
		account.JWTToken = jwtToken
	}
	account.JWTErrorCount = 0
}

func jwtUserDomainID(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return ""
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}

	var payload struct {
		Sub interface{} `json:"sub"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return ""
	}

	switch sub := payload.Sub.(type) {
	case map[string]interface{}:
		if value, ok := sub["userDomainId"].(string); ok {
			return strings.TrimSpace(value)
		}
	case string:
		var nested map[string]interface{}
		if err := json.Unmarshal([]byte(sub), &nested); err == nil {
			if value, ok := nested["userDomainId"].(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}
