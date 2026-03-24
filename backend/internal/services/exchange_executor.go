package services

import (
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/utils"
	"encoding/json"
	"fmt"
	"time"
)

// performExchange 统一封装抢兑请求，避免 API 手动执行与定时调度走两套 HTTP 逻辑。
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

	var response map[string]interface{}
	if err := json.Unmarshal([]byte(body), &response); err != nil {
		return false, fmt.Sprintf("解析响应失败：%v", err), execTime
	}

	msg, ok := response["msg"].(string)
	if !ok {
		return false, "响应格式错误", execTime
	}
	if msg != "success" {
		return false, msg, execTime
	}

	return true, "兑换成功", execTime
}
