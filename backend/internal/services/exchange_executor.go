package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
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
	return performExchangeContext(context.Background(), account, prizeID, tokenMgr)
}

func performExchangeContext(ctx context.Context, account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager) (bool, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	if err := ctx.Err(); err != nil {
		return false, err.Error(), int(time.Since(startTime).Milliseconds())
	}

	authCtx, err := prepareExchangeAuth(account, tokenMgr)
	if err != nil {
		return false, err.Error(), int(time.Since(startTime).Milliseconds())
	}
	if authCtx.jwtToken == "" {
		return false, "JWT token 为空", int(time.Since(startTime).Milliseconds())
	}

	session := newExchangeHTTPSessionContext(ctx, account, authCtx)
	result := executeExchangeOnceContext(ctx, prizeID, authCtx, session)
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
	return executeExchangeOnceContext(context.Background(), prizeID, authCtx, session)
}

func executeExchangeOnceContext(ctx context.Context, prizeID string, authCtx *exchangeAuthContext, session *exchangeHTTPSession) exchangeAttemptResult {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	if session == nil {
		session = newExchangeHTTPSessionContext(ctx, nil, authCtx)
	}

	offset, solveInfo, err := obtainExchangeSlideOffsetContext(ctx, session, authCtx)
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

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, exchangeURL, nil)
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
	if !strings.EqualFold(strings.TrimSpace(msg), "success") {
		message := buildExchangeFailureMessage(statusCode, response, body)
		return exchangeAttemptResult{success: false, message: message, execTime: execTime, stop: isExchangeTerminalMessage(message)}
	}
	if businessFailure := exchangeResponseBusinessFailure(response); businessFailure != "" {
		normalizedResponse := make(map[string]interface{}, len(response)+1)
		for key, value := range response {
			normalizedResponse[key] = value
		}
		normalizedResponse["msg"] = businessFailure
		message := buildExchangeFailureMessage(statusCode, normalizedResponse, body)
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

func buildExchangeURLWithPuzzle(prizeID string, puzzleOffset int) string {
	values := url.Values{}
	values.Set("prizeId", prizeID)
	values.Set("client", "app")
	values.Set("clientVersion", exchangeClientVersion)
	values.Set("puzzleOffset", strconv.Itoa(puzzleOffset))
	values.Set("smsCode", "")
	return "https://m.mcloud.139.com/ycloud/signin/page/exchangeV2?" + values.Encode()
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
		"账号失效",
		"账号已失效",
		"登录失效",
		"重新登录",
		"认证为空",
		"JWT token 为空",
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

func exchangeResponseBusinessFailure(response map[string]interface{}) string {
	if len(response) == 0 {
		return ""
	}

	if candidate := firstResponseValue(response, "desc", "resultMsg", "subMsg", "sub_msg", "error", "errorMsg"); isExchangeFailureText(candidate) {
		return candidate
	}

	if result, ok := response["result"].(map[string]interface{}); ok {
		candidate := firstResponseValue(result, "msg", "message", "desc", "resultMsg", "subMsg", "sub_msg", "error", "errorMsg")
		if isExchangeFailureText(candidate) {
			return candidate
		}
		if code := firstResponseValue(result, "code", "resultCode", "result_code"); !isExchangeSuccessCode(code) {
			if candidate != "" && !strings.EqualFold(candidate, "success") {
				return candidate
			}
			return "兑换失败（业务码 " + code + "）"
		}
	} else if resultText := stringifyExchangeValue(response["result"]); isExchangeFailureText(resultText) {
		return resultText
	}

	if code := firstResponseValue(response, "code", "resultCode", "result_code"); !isExchangeSuccessCode(code) {
		return "兑换失败（业务码 " + code + "）"
	}
	return ""
}

func isExchangeSuccessCode(code string) bool {
	switch strings.ToLower(strings.TrimSpace(code)) {
	case "", "0", "0000", "000000", "200", "success", "ok", "true":
		return true
	default:
		return false
	}
}

func isExchangeFailureText(message string) bool {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" || text == "success" || text == "ok" {
		return false
	}
	patterns := []string{
		"失败", "错误", "异常", "失效", "未登录", "重新登录", "认证为空",
		"token 无效", "jwt token 为空", "不足", "已兑完", "已耗尽", "已兑换",
		"已领取", "不在线", "已下架", "未开始", "频繁", "繁忙", "限流", "风控",
		"failed", "error", "invalid", "expired", "unauthorized",
	}
	for _, pattern := range patterns {
		if strings.Contains(text, pattern) {
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
