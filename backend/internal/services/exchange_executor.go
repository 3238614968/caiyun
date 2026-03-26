package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// performExchange wraps the exchange HTTP request for both manual and scheduled flows.
func performExchange(account *models.ExchangeAccount, prizeID string, tokenMgr *TokenManager) (bool, string, int) {
	startTime := time.Now()

	client := corehttp.NewClient()
	authStr := sanitizeAuthValue(account.Auth)
	if authStr != "" {
		client.SetAuth(authStr)
	}

	jwtToken := account.JWTToken
	if tokenMgr != nil {
		if tokenInfo, err := tokenMgr.GetToken(account.AccountID); err == nil && tokenInfo.JWTToken != "" {
			jwtToken = tokenInfo.JWTToken
		}
	}

	if jwtToken == "" && authStr != "" {
		authClient := corehttp.NewClient()
		authClient.SetAuth(authStr)
		authForJWT := auth.NewAuth(authClient)
		if token, err := authForJWT.GetJWTToken(account.Phone); err == nil && token != "" {
			jwtToken = token
		}
	}
	if jwtToken != "" {
		client.SetJWTToken(jwtToken)
	}

	url := utils.BuildExchangeURL(prizeID)
	resp, err := client.Get(url, nil)
	if err != nil {
		return false, fmt.Sprintf("请求失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	body, err := client.ReadResponseBody(resp)
	if err != nil {
		return false, fmt.Sprintf("读取响应失败：%v", err), int(time.Since(startTime).Milliseconds())
	}

	execTime := int(time.Since(startTime).Milliseconds())
	statusCode := 0
	if resp != nil {
		statusCode = resp.StatusCode
	}

	if statusCode >= 400 {
		return false, fmt.Sprintf("请求返回异常 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime
	}

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return false, fmt.Sprintf("解析响应失败：%v | http_status=%d | body=%s", err, statusCode, summarizeExchangeBody(body)), execTime
	}

	msg := firstResponseValue(response, "msg", "message")
	if msg == "" {
		return false, fmt.Sprintf("响应格式错误 | http_status=%d | body=%s", statusCode, summarizeExchangeBody(body)), execTime
	}
	if msg != "success" {
		return false, buildExchangeFailureMessage(statusCode, response, body), execTime
	}

	return true, "兑换成功", execTime
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
