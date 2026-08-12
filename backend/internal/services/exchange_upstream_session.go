// Exchange upstream session owns HTTP client, device identity, cookies and request headers.
// It is kept separate from exchange orchestration so upstream transport changes
// do not affect scheduling or state transition logic.
package services

import (
	"caiyun/internal/core/shumei"
	"caiyun/internal/models"
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type exchangeHTTPSession struct {
	client    *http.Client
	deviceID  string
	userAgent string
	referer   string
}

func newExchangeHTTPSession(account *models.ExchangeAccount, authCtx *exchangeAuthContext) *exchangeHTTPSession {
	return newExchangeHTTPSessionContext(context.Background(), account, authCtx)
}

func newExchangeHTTPSessionContext(ctx context.Context, account *models.ExchangeAccount, authCtx *exchangeAuthContext) *exchangeHTTPSession {
	if ctx == nil {
		ctx = context.Background()
	}
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: exchangeRequestTimeout,
		Jar:     jar,
	}

	userAgent := shumei.RandomMarketUserAgent()
	deviceID := exchangeFallbackDeviceID
	deviceCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	if fetchedDeviceID, err := shumei.FetchDeviceID(deviceCtx, client, ""); err == nil && strings.TrimSpace(fetchedDeviceID) != "" {
		deviceID = fetchedDeviceID
	}
	cancel()

	referer := buildExchangeReferer(authCtx)
	session := &exchangeHTTPSession{
		client:    client,
		deviceID:  shumei.NormalizeDeviceID(deviceID),
		userAgent: userAgent,
		referer:   referer,
	}
	seedExchangeCookies(session, account, authCtx)
	return session
}

func buildExchangeReferer(authCtx *exchangeAuthContext) string {
	values := url.Values{}
	values.Set("path", "newsignin")
	values.Set("sourceid", exchangeSourceID)
	values.Set("enableShare", "1")
	if authCtx != nil && authCtx.ssoToken != "" {
		values.Set("token", authCtx.ssoToken)
	}
	values.Set("targetSourceId", exchangeTargetSourceID)
	return "https://m.mcloud.139.com/portal/mobilecloud/index.html?" + values.Encode()
}

func seedExchangeCookies(session *exchangeHTTPSession, account *models.ExchangeAccount, authCtx *exchangeAuthContext) {
	if session == nil || session.client == nil || session.client.Jar == nil {
		return
	}

	u, _ := url.Parse("https://m.mcloud.139.com")
	cookies := []*http.Cookie{
		{Name: "sensors_stay_time", Value: strconv.FormatInt(time.Now().UnixMilli(), 10), Path: "/"},
	}
	if authCtx != nil && authCtx.jwtToken != "" {
		cookies = append(cookies, &http.Cookie{Name: "jwtToken", Value: authCtx.jwtToken, Path: "/"})
		if userDomainID := exchangeUserDomainID(authCtx.jwtToken); userDomainID != "" {
			cookies = append(cookies, &http.Cookie{Name: "userDomainId", Value: userDomainID, Path: "/"})
		}
	}
	if cookieDeviceID := shumei.CookieDeviceValue(session.deviceID); cookieDeviceID != "" {
		cookies = append(cookies, &http.Cookie{Name: ".thumbcache_caiyun", Value: cookieDeviceID, Path: "/"})
		if account != nil && strings.TrimSpace(account.Phone) != "" {
			cookies = append(cookies, &http.Cookie{Name: ".thumbcache_" + sanitizeExchangeCookieName(account.Phone), Value: cookieDeviceID, Path: "/"})
		}
	}
	session.client.Jar.SetCookies(u, cookies)
}

func sanitizeExchangeCookieName(value string) string {
	var builder strings.Builder
	for _, r := range strings.TrimSpace(value) {
		switch {
		case r >= '0' && r <= '9':
			builder.WriteRune(r)
		case r >= 'a' && r <= 'z':
			builder.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			builder.WriteRune(r)
		case r == '_' || r == '-':
			builder.WriteRune(r)
		default:
			builder.WriteByte('_')
		}
	}
	if builder.Len() == 0 {
		return "caiyun"
	}
	return builder.String()
}

func buildExchangeHeaders(authCtx *exchangeAuthContext, session *exchangeHTTPSession, extra map[string]string) map[string]string {
	deviceID := exchangeFallbackDeviceID
	userAgent := shumei.RandomMarketUserAgent()
	referer := buildExchangeReferer(authCtx)
	if session != nil {
		if session.deviceID != "" {
			deviceID = session.deviceID
		}
		if session.userAgent != "" {
			userAgent = session.userAgent
		}
		if session.referer != "" {
			referer = session.referer
		}
	}

	headers := map[string]string{
		"Host":             "m.mcloud.139.com",
		"Accept":           "*/*",
		"Accept-Language":  "zh,zh-CN;q=0.9,en-US;q=0.8,en;q=0.7",
		"Cache-Control":    "no-cache",
		"ShowLoading":      "true",
		"Content-Type":     "application/json;charset=UTF-8",
		"deviceid":         deviceID,
		"deviceId":         deviceID,
		"appversion":       exchangeAppVersion,
		"appVersion":       exchangeAppVersion,
		"User-Agent":       userAgent,
		"user-agent":       userAgent,
		"activityid":       exchangeActivityID,
		"ActivityId":       exchangeActivityID,
		"x-requested-with": "com.chinamobile.mcloud",
		"X-Requested-With": "com.chinamobile.mcloud",
		"Referer":          referer,
		"referer":          referer,
	}
	if authCtx != nil && authCtx.jwtToken != "" {
		headers["jwttoken"] = authCtx.jwtToken
		headers["jwtToken"] = authCtx.jwtToken
	}
	for key, value := range extra {
		if strings.TrimSpace(value) != "" {
			headers[key] = value
		}
	}
	return headers
}

func exchangeUserDomainID(token string) string {
	parts := strings.Split(strings.TrimSpace(token), ".")
	if len(parts) < 2 {
		return ""
	}
	data, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var body struct {
		Sub interface{} `json:"sub"`
	}
	if err := json.Unmarshal(data, &body); err != nil {
		return ""
	}
	switch sub := body.Sub.(type) {
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
