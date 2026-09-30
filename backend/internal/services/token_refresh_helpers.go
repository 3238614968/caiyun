package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"encoding/base64"
	"encoding/json"
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

const authorizationPreRefreshSkew = 5 * 24 * time.Hour

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

func authorizationShouldRefresh(expireAt int64, now time.Time) bool {
	if expireAt <= 0 {
		return true
	}
	return expireAt <= now.Add(authorizationPreRefreshSkew).UnixMilli()
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
