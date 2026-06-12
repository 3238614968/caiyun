package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/shumei"
	"caiyun/internal/core/sms"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	exchangeRequestTimeout   = time.Minute
	exchangeClientVersion    = "13.0.0"
	exchangeAppVersion       = exchangeClientVersion + ".0"
	exchangeActivityID       = "sign_in_3"
	exchangeSourceID         = "1097"
	exchangeTargetSourceID   = "001005"
	exchangeSlideMaxAttempt  = 3
	exchangeSlideJitter      = 3
	exchangeFallbackDeviceID = "BXe6dG5DL447+uIMwsoyfnkg68InzFABuAHx7JkXFgEUJGuHGaU5iU4p7MF5JLgXpxZesH/8QKfck3ViH4MpJEw=="
)

type exchangeAuthContext struct {
	jwtToken string
	ssoToken string
}

type exchangeAttemptResult struct {
	success  bool
	message  string
	execTime int
	stop     bool
}

type exchangeHTTPSession struct {
	client    *http.Client
	deviceID  string
	userAgent string
	referer   string
}

type exchangeSlidePayload struct {
	puzzle      string
	picture     string
	picWidth    int
	picHeight   int
	puzzleWidth int
}

func taskExchangePrizeID(task *models.ExchangeTask) string {
	if task == nil {
		return ""
	}
	if task.Product.ID > 0 && isUsableExchangePrizeID(task.Product.PrizeID) {
		return strings.TrimSpace(task.Product.PrizeID)
	}
	return strings.TrimSpace(task.PrizeID)
}

func isUsableExchangePrizeID(prizeID string) bool {
	prizeID = strings.TrimSpace(prizeID)
	if prizeID == "" {
		return false
	}
	return !strings.HasPrefix(prizeID, "{") && !strings.Contains(prizeID, "\"actId\"") && !strings.Contains(prizeID, "\"batchID\"")
}

// performExchange wraps the exchange HTTP request for both manual and scheduled flows.
func performExchange(account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager) (bool, string, int) {
	startTime := time.Now()

	authCtx, err := prepareExchangeAuth(account, tokenMgr)
	if err != nil {
		return false, err.Error(), int(time.Since(startTime).Milliseconds())
	}
	if authCtx.jwtToken == "" {
		return false, "JWT token 为空", int(time.Since(startTime).Milliseconds())
	}

	session := newExchangeHTTPSession(account, authCtx)
	result := executeExchangeOnce(prizeID, authCtx, session)
	return result.success, result.message, result.execTime
}

func prepareExchangeAuth(account *models.ExchangeAccount, tokenMgr *TokenManager) (*exchangeAuthContext, error) {
	authStr := sanitizeAuthValue(account.Auth)
	jwtToken := strings.TrimSpace(account.JWTToken)
	ssoToken := ""

	if tokenMgr != nil && account.AccountID > 0 {
		if tokenInfo, err := tokenMgr.GetToken(account.AccountID); err == nil && tokenInfo != nil {
			if tokenInfo.JWTToken != "" {
				jwtToken = tokenInfo.JWTToken
			}
			ssoToken = tokenInfo.SSOToken
		}
	}

	if (jwtToken == "" || ssoToken == "") && authStr != "" {
		authClient := corehttp.NewClient()
		authClient.SetAuth(authStr)
		authForJWT := auth.NewAuth(authClient)
		token, matchedSSOToken, err := authForJWT.GetJWTTokenWithSSOToken(account.Phone)
		if err == nil {
			if token != "" {
				jwtToken = token
			}
			if matchedSSOToken != "" {
				ssoToken = matchedSSOToken
			}
		}
	}

	return &exchangeAuthContext{jwtToken: jwtToken, ssoToken: ssoToken}, nil
}

func executeExchangeOnce(prizeID string, authCtx *exchangeAuthContext, session *exchangeHTTPSession) exchangeAttemptResult {
	startTime := time.Now()
	if session == nil {
		session = newExchangeHTTPSession(nil, authCtx)
	}

	offset, solveInfo, err := obtainExchangeSlideOffset(session, authCtx)
	if err != nil {
		return exchangeAttemptResult{
			success:  false,
			message:  fmt.Sprintf("滑块验证码识别失败：%v", err),
			execTime: int(time.Since(startTime).Milliseconds()),
		}
	}
	finalOffset := offset + rand.Intn(exchangeSlideJitter*2+1) - exchangeSlideJitter
	if finalOffset < 0 {
		finalOffset = 0
	}

	exchangeURL := buildExchangeURLWithPuzzle(prizeID, finalOffset)

	req, err := http.NewRequest(http.MethodGet, exchangeURL, nil)
	if err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("创建请求失败：%v", err), execTime: int(time.Since(startTime).Milliseconds()), stop: true}
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, nil) {
		req.Header[key] = []string{value}
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("请求失败：%v", err), execTime: int(time.Since(startTime).Milliseconds())}
	}
	defer resp.Body.Close()

	bodyBytes, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("读取响应失败：%v", err), execTime: int(time.Since(startTime).Milliseconds())}
	}
	body := string(bodyBytes)

	execTime := int(time.Since(startTime).Milliseconds())
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	if statusCode >= 400 {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("请求返回异常 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime: execTime}
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("解析响应失败：%v | http_status=%d | body=%s", err, statusCode, summarizeExchangeBody(body)), execTime: execTime}
	}

	msg := firstResponseValue(response, "msg", "message")
	if msg == "" {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("响应格式错误 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime: execTime}
	}
	if msg != "success" {
		message := buildExchangeFailureMessage(statusCode, response, body)
		return exchangeAttemptResult{success: false, message: message, execTime: execTime, stop: isExchangeTerminalMessage(message)}
	}

	prizeName := firstNestedResponseValue(response, []string{"result"}, "prizeName", "name")
	if solveInfo != "" {
		solveInfo = "，" + solveInfo
	}
	if prizeName != "" {
		return exchangeAttemptResult{success: true, message: "兑换成功：" + prizeName + solveInfo, execTime: execTime, stop: true}
	}
	return exchangeAttemptResult{success: true, message: "兑换成功" + solveInfo, execTime: execTime, stop: true}
}

func newExchangeHTTPSession(account *models.ExchangeAccount, authCtx *exchangeAuthContext) *exchangeHTTPSession {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{
		Timeout: exchangeRequestTimeout,
		Jar:     jar,
	}

	userAgent := shumei.RandomMarketUserAgent()
	deviceID := exchangeFallbackDeviceID
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	if fetchedDeviceID, err := shumei.FetchDeviceID(ctx, client, ""); err == nil && strings.TrimSpace(fetchedDeviceID) != "" {
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

func obtainExchangeSlideOffset(session *exchangeHTTPSession, authCtx *exchangeAuthContext) (int, string, error) {
	var lastErr error
	for attempt := 1; attempt <= exchangeSlideMaxAttempt; attempt++ {
		payload, err := fetchExchangeSlide(session, authCtx)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			continue
		}

		result, err := sms.SolveSlide(payload.puzzle, payload.picture)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(attempt) * 200 * time.Millisecond)
			continue
		}
		if result == nil {
			lastErr = fmt.Errorf("识别接口返回为空")
			continue
		}
		info := fmt.Sprintf("滑块识别offset=%d", result.Offset)
		if result.Confidence > 0 {
			info += fmt.Sprintf(" confidence=%.4f", result.Confidence)
		}
		if result.Method != "" {
			info += " method=" + result.Method
		}
		return result.Offset, info, nil
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("识别失败")
	}
	return 0, "", lastErr
}

func fetchExchangeSlide(session *exchangeHTTPSession, authCtx *exchangeAuthContext) (*exchangeSlidePayload, error) {
	if session == nil || session.client == nil {
		return nil, fmt.Errorf("HTTP 会话为空")
	}

	req, err := http.NewRequest(http.MethodPost, "https://m.mcloud.139.com/ycloud/auth-service/slide/getSlide", strings.NewReader(""))
	if err != nil {
		return nil, err
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, map[string]string{
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
	}) {
		req.Header[key] = []string{value}
	}

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("获取滑块验证码请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("读取滑块验证码响应失败: %w", err)
	}
	body := string(bodyBytes)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("获取滑块验证码 HTTP %d: %s", resp.StatusCode, summarizeExchangeBody(body))
	}

	payload, err := decodeExchangeSlideResponse(bodyBytes)
	if err != nil {
		return nil, fmt.Errorf("解析滑块验证码响应失败: %w | body=%s", err, summarizeExchangeBody(body))
	}
	return payload, nil
}

func decodeExchangeSlideResponse(bodyBytes []byte) (*exchangeSlidePayload, error) {
	var raw struct {
		Code    int    `json:"code"`
		Msg     string `json:"msg"`
		Message string `json:"message"`
		Result  struct {
			Puzzle      string      `json:"puzzle"`
			Picture     string      `json:"picture"`
			PicWidth    interface{} `json:"picWidth"`
			PicHeight   interface{} `json:"picHeight"`
			PuzzleWidth interface{} `json:"puzzleWidth"`
		} `json:"result"`
	}
	if err := json.Unmarshal(bodyBytes, &raw); err != nil {
		return nil, err
	}
	if raw.Code != 0 {
		msg := strings.TrimSpace(raw.Msg)
		if msg == "" {
			msg = strings.TrimSpace(raw.Message)
		}
		if msg == "" {
			msg = summarizeExchangeBody(string(bodyBytes))
		}
		return nil, fmt.Errorf("获取滑块验证码失败: code=%d msg=%s", raw.Code, msg)
	}
	if strings.TrimSpace(raw.Result.Puzzle) == "" || strings.TrimSpace(raw.Result.Picture) == "" {
		return nil, fmt.Errorf("获取滑块验证码失败: 图片数据为空")
	}
	return &exchangeSlidePayload{
		puzzle:      raw.Result.Puzzle,
		picture:     raw.Result.Picture,
		picWidth:    exchangeIntFromAny(raw.Result.PicWidth, 680),
		picHeight:   exchangeIntFromAny(raw.Result.PicHeight, 400),
		puzzleWidth: exchangeIntFromAny(raw.Result.PuzzleWidth, 96),
	}, nil
}

func exchangeIntFromAny(value interface{}, fallback int) int {
	switch v := value.(type) {
	case nil:
		return fallback
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return fallback
		}
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}

func buildExchangeURLWithPuzzle(prizeID string, puzzleOffset int) string {
	values := url.Values{}
	values.Set("prizeId", prizeID)
	values.Set("client", "app")
	values.Set("clientVersion", exchangeClientVersion)
	values.Set("puzzleOffset", strconv.Itoa(puzzleOffset))
	values.Set("smsCode", "")
	return "https://m.mcloud.139.com/ycloud/signin/page/exchangeV2?" + values.Encode()
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

func isExchangeTerminalMessage(message string) bool {
	terminalPatterns := []string{
		"已兑完",
		"已耗尽",
		"奖品单日已耗尽",
		"奖品已兑完",
		"云朵不足",
		"不足",
		"已兑换",
		"今日已兑换",
		"本月已兑换",
		"已下架",
		"账号未登录",
		"Token 无效",
		"账号被封禁",
	}
	for _, pattern := range terminalPatterns {
		if strings.Contains(message, pattern) {
			return true
		}
	}
	return false
}

func firstNestedResponseValue(response map[string]interface{}, path []string, keys ...string) string {
	var current interface{} = response
	for _, segment := range path {
		currentMap, ok := current.(map[string]interface{})
		if !ok {
			return ""
		}
		current = currentMap[segment]
	}
	currentMap, ok := current.(map[string]interface{})
	if !ok {
		return ""
	}
	return firstResponseValue(currentMap, keys...)
}

func buildExchangeFailureMessage(statusCode int, response map[string]interface{}, body string) string {
	msg := firstResponseValue(response, "msg", "message", "desc", "resultMsg")
	if msg == "" {
		msg = "兑换失败"
	}

	parts := []string{msg}
	if statusCode > 0 {
		parts = append(parts, fmt.Sprintf("http_status=%d", statusCode))
	}

	appendField := func(label string, keys ...string) {
		value := firstResponseValue(response, keys...)
		if value == "" || value == msg {
			return
		}
		parts = append(parts, fmt.Sprintf("%s=%s", label, value))
	}

	appendField("code", "code")
	appendField("result_code", "resultCode", "result_code")
	appendField("result", "result")
	appendField("desc", "desc")
	appendField("sub_msg", "subMsg", "sub_msg")
	appendField("trace_id", "traceId", "trace_id")

	if compactBody := summarizeExchangeBody(body); compactBody != "" {
		parts = append(parts, fmt.Sprintf("body=%s", compactBody))
	}

	return strings.Join(parts, " | ")
}

func firstResponseValue(response map[string]interface{}, keys ...string) string {
	for _, key := range keys {
		value, ok := response[key]
		if !ok {
			continue
		}
		text := stringifyExchangeValue(value)
		if text != "" {
			return text
		}
	}
	return ""
}

func stringifyExchangeValue(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v))
		}
		return fmt.Sprintf("%v", v)
	case bool:
		if v {
			return "true"
		}
		return "false"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

func summarizeExchangeBody(body string) string {
	compact := strings.Join(strings.Fields(body), " ")
	if compact == "" {
		return "-"
	}
	const limit = 180
	if len(compact) <= limit {
		return compact
	}
	return compact[:limit] + "..."
}
