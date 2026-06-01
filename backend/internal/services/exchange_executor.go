package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	exchangeRequestTimeout = time.Minute
	exchangeDeviceID       = "BXe6dG5DL447+uIMwsoyfnkg68InzFABuAHx7JkXFgEUJGuHGaU5iU4p7MF5JLgXpxZesH/8QKfck3ViH4MpJEw=="
	exchangeAppVersion     = "12.5.3.0"
	exchangeUserAgent      = "Mozilla/5.0 (Linux; Android 16; 22127RK46C Build/BP2A.250605.031.A3; wv) AppleWebKit/537.36 Chrome/146.0.7680.164 Mobile Safari/537.36"
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

	result := executeExchangeOnce(prizeID, authCtx)
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

func executeExchangeOnce(prizeID string, authCtx *exchangeAuthContext) exchangeAttemptResult {
	startTime := time.Now()
	url := utils.BuildExchangeURL(prizeID)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("创建请求失败：%v", err), execTime: int(time.Since(startTime).Milliseconds()), stop: true}
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx) {
		req.Header[key] = []string{value}
	}

	client := &http.Client{Timeout: exchangeRequestTimeout}
	resp, err := client.Do(req)
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
	if prizeName != "" {
		return exchangeAttemptResult{success: true, message: "兑换成功：" + prizeName, execTime: execTime, stop: true}
	}
	return exchangeAttemptResult{success: true, message: "兑换成功", execTime: execTime, stop: true}
}

func buildExchangeHeaders(authCtx *exchangeAuthContext) map[string]string {
	referer := "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin&sourceid=1097&enableShare=1&targetSourceId=001005"
	if authCtx != nil && authCtx.ssoToken != "" {
		referer = "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin&sourceid=1097&enableShare=1&token=" + url.QueryEscape(authCtx.ssoToken) + "&targetSourceId=001005"
	}

	headers := map[string]string{
		"Host":             "m.mcloud.139.com",
		"Accept":           "*/*",
		"deviceid":         exchangeDeviceID,
		"deviceId":         exchangeDeviceID,
		"appversion":       exchangeAppVersion,
		"User-Agent":       exchangeUserAgent,
		"user-agent":       exchangeUserAgent,
		"activityid":       "sign_in_3",
		"x-requested-with": "com.chinamobile.mcloud",
		"Referer":          referer,
		"referer":          referer,
	}
	if authCtx != nil && authCtx.jwtToken != "" {
		headers["jwttoken"] = authCtx.jwtToken
		headers["jwtToken"] = authCtx.jwtToken
	}
	return headers
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
