// Exchange upstream session owns HTTP client, device identity, cookies and request headers.
// It is kept separate from exchange orchestration so upstream transport changes
// do not affect scheduling or state transition logic.
package services

import (
	coreapi "caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/shumei"
	"caiyun/internal/models"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

type exchangeHTTPSession struct {
	client       *http.Client
	deviceID     string
	userAgent    string
	referer      string
	captchaTID   string
	deviceErr    error
	bootstrapErr error
	loggedIn     bool
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

	userAgent := coreapi.MarketUserAgent
	// Cached JWT lifetimes do not imply that the SSO used to obtain them is
	// still valid. Get a fresh page token before registering the page device.
	if account != nil && authCtx != nil && authCtx.authorization != "" {
		c := corehttp.NewClient()
		c.SetAuth(authCtx.authorization)
		phone := account.Phone
		if parsed, err := auth.ParseToken(authCtx.authorization); err == nil {
			phone = parsed.Phone
		}
		ssoCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
		fresh, err := auth.NewAuth(c).QuerySpecTokenForJWTContext(ssoCtx, phone)
		cancel()
		if err != nil || fresh == "" {
			return &exchangeHTTPSession{client: client, bootstrapErr: fmt.Errorf("获取活动页面 SSO 失败，未提交兑换")}
		}
		authCtx.ssoToken = fresh
	}
	referer := buildExchangeReferer(authCtx)
	deviceID := ""
	var deviceErr error
	deviceCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	if fetchedDeviceID, err := shumei.FetchDeviceIDForPage(deviceCtx, client, "", userAgent, referer); err == nil && strings.TrimSpace(fetchedDeviceID) != "" {
		deviceID = fetchedDeviceID
	} else {
		if err != nil {
			deviceErr = fmt.Errorf("设备认证失败，未提交兑换: %w", err)
		} else {
			deviceErr = fmt.Errorf("设备认证失败，设备标识为空，未提交兑换")
		}
	}
	cancel()

	session := &exchangeHTTPSession{
		client:    client,
		deviceID:  shumei.NormalizeDeviceID(deviceID),
		deviceErr: deviceErr,
		userAgent: userAgent,
		referer:   referer,
	}
	seedExchangeAuthCookies(session, authCtx)
	return session
}

func ensureExchangeSessionLogin(ctx context.Context, session *exchangeHTTPSession, credentials *exchangeAuthContext) error {
	if session.bootstrapErr != nil {
		return session.bootstrapErr
	}
	if session.deviceErr != nil {
		return session.deviceErr
	}
	if session.loggedIn {
		return nil
	}
	if credentials == nil {
		return fmt.Errorf("缺少活动登录凭据，未提交兑换")
	}
	warmExchangePortal(ctx, session, credentials)
	loginCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	token, err := auth.LoginH5Session(loginCtx, session.client, credentials.ssoToken, session.userAgent, session.referer)
	if err != nil {
		session.bootstrapErr = fmt.Errorf("活动会话初始化失败，未提交兑换: %w", err)
		return session.bootstrapErr
	}
	if before, after := exchangeUserDomainID(credentials.jwtToken), exchangeUserDomainID(token); before != "" && before != after {
		session.bootstrapErr = fmt.Errorf("活动会话账号不匹配，未提交兑换")
		return session.bootstrapErr
	}
	credentials.jwtToken = token
	seedExchangeAuthCookies(session, credentials)
	session.loggedIn = true
	return nil
}

// HAR HTTP/2 cookie fields are repeated header entries, not a single map value.
// Retain current account cookies alongside the SDK and server-issued cookies.
func seedExchangeAuthCookies(session *exchangeHTTPSession, authCtx *exchangeAuthContext) {
	if session == nil || session.client == nil || session.client.Jar == nil || authCtx == nil {
		return
	}
	u, _ := url.Parse("https://m.mcloud.139.com/")
	var cookies []*http.Cookie
	if authCtx.jwtToken != "" {
		cookies = append(cookies, &http.Cookie{Name: "jwtToken", Value: authCtx.jwtToken, Path: "/", Secure: true})
		if domain := exchangeUserDomainID(authCtx.jwtToken); domain != "" {
			cookies = append(cookies, &http.Cookie{Name: "userDomainId", Value: domain, Path: "/", Secure: true})
		}
	}
	payload := strings.TrimSpace(authCtx.authorization)
	if len(payload) >= 6 && strings.EqualFold(payload[:6], "Basic ") {
		payload = strings.TrimSpace(payload[6:])
	}
	if decoded, err := base64.StdEncoding.DecodeString(payload); err == nil && strings.Count(string(decoded), ":") >= 2 {
		cookies = append(cookies, &http.Cookie{Name: "cookieTokenKey", Value: url.QueryEscape(payload), Path: "/", Secure: true})
	}
	session.client.Jar.SetCookies(u, cookies)
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

func buildExchangeHeaders(authCtx *exchangeAuthContext, session *exchangeHTTPSession, extra map[string]string) map[string]string {
	userAgent := coreapi.MarketUserAgent
	referer := buildExchangeReferer(authCtx)
	if session != nil {
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
		"Origin":           "https://m.mcloud.139.com",
		"Accept-Language":  "zh,zh-CN;q=0.9,en-US;q=0.8,en;q=0.7",
		"Cache-Control":    "no-cache",
		"ShowLoading":      "true",
		"Content-Type":     "application/json;charset=UTF-8",
		"appversion":       exchangeAppVersion,
		"User-Agent":       userAgent,
		"activityid":       exchangeActivityID,
		"x-requested-with": "com.chinamobile.mcloud",
		"Referer":          referer,
	}
	if authCtx != nil && authCtx.jwtToken != "" {
		headers["jwttoken"] = authCtx.jwtToken
	}
	// A response trace header is not a challenge token. The successful HAR's
	// exchange request does not echo x-yun-tid from an earlier response.
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
