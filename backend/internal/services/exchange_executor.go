package services

import (
	coreapi "caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	exchangeRequestTimeout   = time.Minute
	exchangeClientVersion    = coreapi.MarketClientVersion
	exchangeAppVersion       = exchangeClientVersion + ".0"
	exchangeActivityID       = "sign_in_3"
	exchangeSourceID         = "1097"
	exchangeTargetSourceID   = "001005"
	exchangeSlideMaxAttempt  = 3
	exchangeFallbackDeviceID = "BXe6dG5DL447+uIMwsoyfnkg68InzFABuAHx7JkXFgEUJGuHGaU5iU4p7MF5JLgXpxZesH/8QKfck3ViH4MpJEw=="
)

type exchangeAuthContext struct {
	jwtToken string
	ssoToken string
}

type exchangePreparedSession struct {
	auth       *exchangeAuthContext
	http       *exchangeHTTPSession
	sourceAuth string
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
	return performExchangePreparedContext(ctx, account, prizeID, tokenMgr, &exchangePreparedSession{})
}

func performExchangePreparedContext(ctx context.Context, account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager, prepared *exchangePreparedSession) (bool, string, int) {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	if err := ctx.Err(); err != nil {
		return false, err.Error(), int(time.Since(startTime).Milliseconds())
	}

	if prepared == nil {
		prepared = &exchangePreparedSession{}
	}
	authCtx := prepared.auth
	if authCtx != nil {
		expiry := jwtExpiresAt(authCtx.jwtToken)
		if !expiry.IsZero() && !expiry.After(time.Now().Add(15*time.Second)) {
			authCtx = nil
			prepared.auth = nil
			prepared.http = nil
		}
	}
	if authCtx == nil {
		var err error
		authCtx, err = prepareExchangeAuth(account, tokenMgr)
		if err != nil {
			return false, err.Error(), int(time.Since(startTime).Milliseconds())
		}
		prepared.auth = authCtx
	}
	if authCtx.jwtToken == "" {
		return false, "JWT token 为空", int(time.Since(startTime).Milliseconds())
	}

	session := prepared.http
	if session == nil {
		session = newExchangeHTTPSessionContext(ctx, account, authCtx)
		prepared.http = session
	}
	result := executeExchangeOnceContext(ctx, prizeID, authCtx, session)
	return result.success, result.message, result.execTime
}

func prepareExchangeAuth(account *models.ExchangeAccount, tokenMgr *TokenManager) (*exchangeAuthContext, error) {
	if tokenMgr == nil {
		return prepareExchangeAuthWithProvider(account, nil)
	}
	return prepareExchangeAuthWithProvider(account, tokenMgr)
}

type exchangeTokenProvider interface {
	GetToken(accountID uint) (*TokenInfo, error)
}

func prepareExchangeAuthWithProvider(account *models.ExchangeAccount, tokenMgr exchangeTokenProvider) (*exchangeAuthContext, error) {
	if account == nil {
		return nil, fmt.Errorf("抢兑账号为空")
	}
	authStr := sanitizeAuthValue(account.Auth)
	jwtToken := strings.TrimSpace(account.JWTToken)
	ssoToken := ""

	if tokenMgr != nil && account.AccountID > 0 {
		tokenInfo, err := tokenMgr.GetToken(account.AccountID)
		if err != nil {
			return nil, fmt.Errorf("刷新抢兑账号 Token 失败: %w", err)
		}
		if tokenInfo == nil || tokenInfo.JWTToken == "" {
			return nil, fmt.Errorf("抢兑账号刷新后没有可用 JWT")
		}
		jwtToken = tokenInfo.JWTToken
		ssoToken = tokenInfo.SSOToken
		if tokenInfo.Auth != "" {
			authStr = tokenInfo.Auth
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
	if _, err := strconv.ParseUint(strings.TrimSpace(prizeID), 10, 64); err != nil {
		return exchangeAttemptResult{message: "商品 prizeId 格式无效", stop: true}
	}

	offset, solveInfo, err := obtainExchangeSlideOffsetContext(ctx, session, authCtx)
	if err != nil {
		return exchangeAttemptResult{
			success:  false,
			message:  fmt.Sprintf("滑块验证码识别失败：%v", err),
			execTime: int(time.Since(startTime).Milliseconds()),
		}
	}
	// The solver returns the original image coordinate. Altering it afterward
	// makes a correct match fail the upstream challenge.
	req, err := buildExchangeV3Request(ctx, prizeID, offset, session.deviceID)
	if err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("创建请求失败：%v", err), execTime: int(time.Since(startTime).Milliseconds()), stop: true}
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, map[string]string{"isDeviceId": "true", "Origin": "https://m.mcloud.139.com"}) {
		req.Header.Set(key, value)
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
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("解析响应失败：%v | http_status=%d | body=%s", err, statusCode, summarizeExchangeBody(body)), execTime: execTime}
	}

	msg := firstResponseValue(response, "msg", "message")
	if msg == "" {
		return exchangeAttemptResult{success: false, message: fmt.Sprintf("响应格式错误 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime: execTime}
	}
	if !strings.EqualFold(strings.TrimSpace(msg), "success") {
		message := buildExchangeFailureMessage(statusCode, response, body)
		if solveInfo != "" {
			message += " | " + solveInfo
		}
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

	if firstResponseValue(response, "code") == "" || firstNestedResponseValue(response, []string{"result"}, "oid", "oId") == "" {
		return exchangeAttemptResult{message: "响应格式错误：兑换结果待确认，成功回包缺少业务码或奖品记录，请先查询领奖专区", execTime: execTime, stop: true}
	}
	if returnedID := firstNestedResponseValue(response, []string{"result"}, "prizeId"); returnedID != "" && returnedID != strings.TrimSpace(prizeID) {
		return exchangeAttemptResult{message: "响应格式错误：兑换结果待确认，回包商品与请求不一致，请先查询领奖专区", execTime: execTime, stop: true}
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

func buildExchangeV3Request(ctx context.Context, prizeID string, puzzleOffset int, deviceID string) (*http.Request, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("设备标识为空")
	}
	body, err := json.Marshal(map[string]interface{}{
		"prizeId": json.Number(strings.TrimSpace(prizeID)), "client": "app",
		"clientVersion": exchangeClientVersion, "puzzleOffset": puzzleOffset,
		"smsCode": "", "deviceId": deviceID,
	})
	if err != nil {
		return nil, err
	}
	return http.NewRequestWithContext(ctx, http.MethodPost,
		"https://m.mcloud.139.com/ycloud/signin/page/exchangeV3", strings.NewReader(string(body)))
}

func isExchangeTerminalMessage(message string) bool {
	terminalPatterns := []string{
		"兑换结果待确认",
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
