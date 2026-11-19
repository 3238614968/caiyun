package services

import (
	coreapi "caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"caiyun/internal/utils"
	"caiyun/internal/version"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	exchangeRequestTimeout  = time.Minute
	exchangeClientVersion   = coreapi.MarketClientVersion
	exchangeAppVersion      = exchangeClientVersion + ".0"
	exchangeActivityID      = "sign_in_3"
	exchangeSourceID        = "1097"
	exchangeTargetSourceID  = "001005"
	exchangeSlideMaxAttempt = 3
)

type exchangeAuthContext struct {
	jwtToken      string
	ssoToken      string
	authorization string
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
	outcome  models.ExchangeOutcome
	guardErr error
}

type exchangeSubmissionGuard func(context.Context) error

type exchangeTaskExecutionResult struct {
	exchangeAttemptResult
	executionToken string
	attemptsUsed   int
	skipped        bool
	persistenceErr error
}

func newExchangeAttemptResult(success bool, message string, execTime int, outcome models.ExchangeOutcome) exchangeAttemptResult {
	return exchangeAttemptResult{success: success, message: message, execTime: execTime, outcome: outcome, stop: outcome == models.ExchangeOutcomeUnknown}
}

func preflightExchangeResult(message string, stop bool) exchangeAttemptResult {
	return exchangeAttemptResult{message: message, stop: stop, outcome: models.ExchangeOutcomeNotSubmitted}
}

func unknownExchangeResult(message string, execTime int) exchangeAttemptResult {
	if !strings.Contains(message, "兑换结果待确认") {
		message += " | 兑换结果待确认，请核对领奖专区及兑换记录，暂勿重复兑换"
	}
	return newExchangeAttemptResult(false, message, execTime, models.ExchangeOutcomeUnknown)
}

func acceptedExchangeResult(message string, execTime int) exchangeAttemptResult {
	return newExchangeAttemptResult(true, message, execTime, models.ExchangeOutcomeAccepted)
}

func rejectedExchangeResult(message string, execTime int, stop bool) exchangeAttemptResult {
	result := newExchangeAttemptResult(false, message, execTime, models.ExchangeOutcomeRejected)
	result.stop = stop
	return result
}

func (r exchangeAttemptResult) retryable() bool {
	if r.guardErr != nil || (r.outcome != "" && r.outcome != models.ExchangeOutcomeNotSubmitted) {
		return false
	}
	return isExchangeRetryableFailure(r.message)
}

func (r exchangeAttemptResult) settlement(status models.ExchangeTaskStatus) repository.ExchangeSettlement {
	outcome := r.outcome
	if outcome == "" {
		outcome = models.ExchangeOutcomeNotSubmitted
	}
	return repository.ExchangeSettlement{Outcome: outcome, Message: r.message, Status: status, ExecutionTimeMs: r.execTime}
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

// performExchange wraps the legacy no-task request used by monthly exchange.
// Task execution uses performExchangeWithControlsContext with a durable guard.
func performExchange(account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager) (bool, string, int) {
	result := performExchangePreparedContext(context.Background(), account, prizeID, tokenMgr, &exchangePreparedSession{})
	return result.success, result.message, result.execTime
}

func performExchangePreparedContext(ctx context.Context, account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager, prepared *exchangePreparedSession, guards ...exchangeSubmissionGuard) (result exchangeAttemptResult) {
	defer func() { result.message = version.Annotate(result.message) }()
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	defer func() {
		if result.execTime == 0 {
			result.execTime = int(time.Since(startTime).Milliseconds())
		}
	}()
	if err := ctx.Err(); err != nil {
		return preflightExchangeResult(err.Error(), false)
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
			return preflightExchangeResult(err.Error(), false)
		}
		prepared.auth = authCtx
	}
	if authCtx.jwtToken == "" {
		return preflightExchangeResult("JWT token 为空", false)
	}

	session := prepared.http
	if session == nil {
		session = newExchangeHTTPSessionContext(ctx, account, authCtx)
		prepared.http = session
	}
	if err := ensureExchangeSessionLogin(ctx, session, authCtx); err != nil {
		return preflightExchangeResult(err.Error(), false)
	}
	return executeExchangeOnceContext(ctx, prizeID, authCtx, session, guards...)
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
	return prepareExchangeAuthWithRefresh(account, tokenMgr, func(authValue, phone string) (string, string, error) {
		client := corehttp.NewClient()
		client.SetAuth(authValue)
		return auth.NewAuth(client).GetJWTTokenWithSSOToken(phone)
	})
}

func prepareExchangeAuthWithRefresh(account *models.ExchangeAccount, tokenMgr exchangeTokenProvider, refresh func(string, string) (string, string, error)) (*exchangeAuthContext, error) {
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
		token, matchedSSOToken, err := refresh(authStr, account.Phone)
		if err != nil {
			return nil, fmt.Errorf("刷新抢兑会话失败，未提交兑换: %w", err)
		}
		jwtToken, ssoToken = strings.TrimSpace(token), strings.TrimSpace(matchedSSOToken)
	}
	if jwtToken == "" || ssoToken == "" {
		return nil, fmt.Errorf("抢兑登录会话缺少匹配的 JWT/SSO，未提交兑换")
	}

	return &exchangeAuthContext{jwtToken: jwtToken, ssoToken: ssoToken, authorization: authStr}, nil
}

func executeExchangeOnce(prizeID string, authCtx *exchangeAuthContext, session *exchangeHTTPSession, guards ...exchangeSubmissionGuard) exchangeAttemptResult {
	return executeExchangeOnceContext(context.Background(), prizeID, authCtx, session, guards...)
}

func executeExchangeOnceContext(ctx context.Context, prizeID string, authCtx *exchangeAuthContext, session *exchangeHTTPSession, guards ...exchangeSubmissionGuard) (result exchangeAttemptResult) {
	if ctx == nil {
		ctx = context.Background()
	}
	startTime := time.Now()
	if session == nil {
		session = newExchangeHTTPSessionContext(ctx, nil, authCtx)
	}
	result = preflightExchangeResult("", false)
	defer func() { result.execTime = int(time.Since(startTime).Milliseconds()) }()
	if _, err := strconv.ParseUint(strings.TrimSpace(prizeID), 10, 64); err != nil {
		result.message, result.stop = "商品 prizeId 格式无效", true
		return result
	}
	if session.deviceErr != nil {
		result.message, result.stop = session.deviceErr.Error(), true
		return result
	}
	if strings.TrimSpace(session.deviceID) == "" {
		result.message, result.stop = "设备认证失败，设备标识为空，未提交兑换", true
		return result
	}

	// H5 checks prize eligibility before opening the captcha. Keep known account
	// restrictions separate from the upstream's generic 610/GK rejection.
	eligibility, eligibilityErr := fetchExchangeEligibilityContext(ctx, session, authCtx, prizeID)
	if eligibilityErr != nil {
		result.message, result.stop = fmt.Sprintf("兑换资格查询失败，未提交兑换: %v", eligibilityErr), true
		return result
	} else {
		if reason := exchangeEligibilityBlockReason(eligibility); reason != "" {
			result.message, result.stop = reason, true
			return result
		}
	}

	visitErr := prepareExchangeVisit(ctx, session, authCtx)
	offset, solveInfo, err := obtainExchangeSlideOffsetContext(ctx, session, authCtx)
	if err != nil {
		result.message = fmt.Sprintf("滑块验证码识别失败：%v", err)
		return result
	}
	if note := exchangeEligibilityNote(eligibility); note != "" {
		solveInfo = strings.TrimSpace(solveInfo + " " + note)
	}
	if visitErr != nil {
		solveInfo += " 活动访问记录未完成"
	}
	// The solver returns the original image coordinate. Altering it afterward
	// makes a correct match fail the upstream challenge.
	req, err := buildExchangeV3Request(ctx, prizeID, offset, session.deviceID)
	if err != nil {
		result.message, result.stop = fmt.Sprintf("创建请求失败：%v", err), true
		return result
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, map[string]string{"isDeviceId": "true", "Origin": "https://m.mcloud.139.com"}) {
		req.Header.Set(key, value)
	}

	// Mark only after every preflight step and immediately before the POST. The
	// callback is fencing-owned: no durable boundary means no upstream request.
	if len(guards) > 0 && guards[0] != nil {
		if err := guards[0](ctx); err != nil {
			result.guardErr = err
			result.message = fmt.Sprintf("提交边界持久化失败，未提交兑换: %v", err)
			return result
		}
	}
	resp, err := session.client.Do(req)
	if err != nil {
		return unknownExchangeResult(fmt.Sprintf("请求失败：%v", err), result.execTime)
	}
	defer resp.Body.Close()

	bodyBytes, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return unknownExchangeResult(fmt.Sprintf("读取响应失败：%v", err), result.execTime)
	}
	body := string(bodyBytes)

	execTime := result.execTime
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	if statusCode >= 400 {
		return unknownExchangeResult(fmt.Sprintf("请求返回异常 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime)
	}

	var response map[string]interface{}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return unknownExchangeResult(fmt.Sprintf("解析响应失败：%v | http_status=%d | body=%s", err, statusCode, summarizeExchangeBody(body)), execTime)
	}

	msg := firstResponseValue(response, "msg", "message")
	if msg == "" {
		return unknownExchangeResult(fmt.Sprintf("响应格式错误 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime)
	}
	if !strings.EqualFold(strings.TrimSpace(msg), "success") {
		message := buildExchangeFailureMessage(statusCode, response, body)
		if solveInfo != "" {
			message += " | " + solveInfo
		}
		return rejectedExchangeResult(message, execTime, isExchangeTerminalMessage(message))
	}
	if businessFailure := exchangeResponseBusinessFailure(response); businessFailure != "" {
		normalizedResponse := make(map[string]interface{}, len(response)+1)
		for key, value := range response {
			normalizedResponse[key] = value
		}
		normalizedResponse["msg"] = businessFailure
		message := buildExchangeFailureMessage(statusCode, normalizedResponse, body)
		return rejectedExchangeResult(message, execTime, isExchangeTerminalMessage(message))
	}

	if firstResponseValue(response, "code") == "" || firstNestedResponseValue(response, []string{"result"}, "oid", "oId") == "" {
		return unknownExchangeResult("响应格式错误：兑换结果待确认，成功回包缺少业务码或奖品记录，请先查询领奖专区", execTime)
	}
	if returnedID := firstNestedResponseValue(response, []string{"result"}, "prizeId"); returnedID != "" && returnedID != strings.TrimSpace(prizeID) {
		return unknownExchangeResult("响应格式错误：兑换结果待确认，回包商品与请求不一致，请先查询领奖专区", execTime)
	}

	prizeName := firstNestedResponseValue(response, []string{"result"}, "prizeName", "name")
	if solveInfo != "" {
		solveInfo = "，" + solveInfo
	}
	if prizeName != "" {
		return acceptedExchangeResult("兑换成功："+prizeName+solveInfo, execTime)
	}
	return acceptedExchangeResult("兑换成功"+solveInfo, execTime)
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
		"兑换资格校验未通过",
		"非目标用户",
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
