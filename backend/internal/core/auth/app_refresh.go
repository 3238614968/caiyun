package auth

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	corehttp "caiyun/internal/core/http"
	"github.com/google/uuid"
)

const (
	defaultAppRefreshAESKey = "2olBaQGYnEKoStYomsd1n7ax"
	appRefreshURL           = "https://user-njs.yun.139.com/user/auth/refreshToken"
	appRefreshDeviceID      = "1E58F2CE422EB2234BB8795E316BD44B"
	authWebViewUA           = "Mozilla/5.0 (Linux; Android 13; 23049RAD8C Build/TKQ1.221114.001; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/108.0.5359.128 Mobile Safari/537.36 MCloudApp/12.5.4 AppLanguage/zh-CN"
	authOkHTTPUA            = "okhttp/4.12.0"
	authTargetSourceID      = "001005"
	authQuerySpecTokenURL   = "https://orches.yun.139.com/orchestration/auth-rebuild/token/v1.0/querySpecToken"
	tyrzLoginURL            = "https://m.mcloud.139.com/ycloud/auth-service/auth/tyrzLogin"
	tyrzTimestampURL        = "https://m.mcloud.139.com/ycloud/api/timestamp"
	tyrzLoginSigningKey     = "sekaMdYYLIZfbCfm"
	tyrzTimestampTimeout    = 5 * time.Second
	tyrzLoginTimeout        = 10 * time.Second
)

func appRefreshAESKey() []byte {
	if key := strings.TrimSpace(os.Getenv("CAIYUN_APP_REFRESH_AES_KEY")); key != "" {
		return []byte(key)
	}
	// 该默认值来自移动云盘上游 APP 协议，非项目自有密钥；允许为空时按协议默认值兼容。
	return []byte(defaultAppRefreshAESKey)
}

var appRefreshDeviceInfo = fmt.Sprintf(
	"1|127.0.0.1|1|12.5.4|Xiaomi|23049RAD8C|%s|02-00-00-00-00-00|android 13",
	appRefreshDeviceID,
)

// AuthorizationRefreshResult 描述新版 APP authorization 刷新后的完整结果。
type AuthorizationRefreshResult struct {
	Auth       string
	AuthBase64 string
	Token      string
	Phone      string
	Platform   string
	ExpireAt   int64
	SSOToken   string
}

// RefreshAuthorization 使用新版 APP refreshToken 接口刷新 authorization。
//
// 只有 refreshToken 解密成功、拿到新 authToken 且 querySpecToken 验证通过时才返回结果；
// 调用方应仅在本方法返回 nil error 后更新数据库。
func (a *Auth) RefreshAuthorization(currentAuthorization, phone, userDomainID string) (*AuthorizationRefreshResult, error) {
	return a.RefreshAuthorizationContext(context.Background(), currentAuthorization, phone, userDomainID)
}

// RefreshAuthorizationContext refreshes authorization with cancellation
// propagated through validation retries and every upstream HTTP request.
func (a *Auth) RefreshAuthorizationContext(ctx context.Context, currentAuthorization, phone, userDomainID string) (*AuthorizationRefreshResult, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	currentAuthorization = ensureBasicAuth(currentAuthorization)
	if currentAuthorization == "" {
		return nil, fmt.Errorf("authorization 为空")
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return nil, fmt.Errorf("手机号为空")
	}

	encBody, err := appRefreshEncrypt(`{"clientType":"414"}`)
	if err != nil {
		return nil, fmt.Errorf("加密刷新请求失败: %w", err)
	}

	headers := map[string]string{
		"Authorization":     currentAuthorization,
		"Content-Type":      "application/json; charset=UTF-8",
		"User-Agent":        authOkHTTPUA,
		"x-nettype":         "1",
		"x-deviceinfo":      appRefreshDeviceInfo,
		"x-yun-client-info": appRefreshDeviceInfo,
		"x-svctype":         "1",
		"x-yun-uni":         strings.TrimSpace(userDomainID),
		"hcy-cool-flag":     "1",
	}

	resp, err := a.client.PostWithContext(ctx, appRefreshURL, headers, encBody)
	if err != nil {
		return nil, fmt.Errorf("刷新 authorization 请求失败: %w", err)
	}
	body, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取刷新响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("刷新 authorization HTTP 状态异常: %d", resp.StatusCode)
	}

	rawB64 := normalizeJSONString(body)
	if rawB64 == "" {
		return nil, fmt.Errorf("刷新 authorization 响应为空")
	}

	decrypted, err := appRefreshDecrypt(rawB64)
	if err != nil {
		return nil, fmt.Errorf("解密刷新响应失败: %w", err)
	}

	authToken, err := parseRefreshAuthToken(decrypted)
	if err != nil {
		return nil, err
	}

	newAuth := GenerateAuth(authToken, phone, "mobile")
	ssoToken, err := a.QuerySpecTokenWithAuthorizationContext(ctx, newAuth, phone)
	if err != nil {
		return nil, fmt.Errorf("刷新 authorization 验证失败: %w", err)
	}
	if ssoToken == "" {
		return nil, fmt.Errorf("刷新 authorization 验证失败: ssoToken 为空")
	}

	info, err := ParseToken(newAuth)
	if err != nil {
		return nil, fmt.Errorf("解析刷新后的 authorization 失败: %w", err)
	}

	return &AuthorizationRefreshResult{
		Auth:       info.AuthFull,
		AuthBase64: info.Auth,
		Token:      info.Token,
		Phone:      info.Phone,
		Platform:   info.Platform,
		ExpireAt:   info.Expire,
		SSOToken:   ssoToken,
	}, nil
}

// QuerySpecTokenWithAuthorization 使用指定 authorization 获取 SSO token。
func (a *Auth) QuerySpecTokenWithAuthorization(authorization, phone string) (string, error) {
	return a.QuerySpecTokenWithAuthorizationContext(context.Background(), authorization, phone)
}

func (a *Auth) QuerySpecTokenWithAuthorizationContext(ctx context.Context, authorization, phone string) (string, error) {
	authorization = ensureBasicAuth(authorization)
	if authorization == "" {
		return "", fmt.Errorf("authorization 为空")
	}
	return a.querySpecTokenWithRetryContext(ctx, authorization, phone)
}

func (a *Auth) querySpecTokenWithRetry(authorization, phone string) (string, error) {
	return a.querySpecTokenWithRetryContext(context.Background(), authorization, phone)
}

func (a *Auth) querySpecTokenWithRetryContext(ctx context.Context, authorization, phone string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", fmt.Errorf("手机号为空")
	}

	reqBody := map[string]interface{}{
		"account":    phone,
		"toSourceId": authTargetSourceID,
	}

	headers := map[string]string{
		"Content-Type": "application/json",
		"User-Agent":   authWebViewUA,
		"Accept":       "*/*",
		"Host":         "orches.yun.139.com",
	}
	if strings.TrimSpace(authorization) != "" {
		headers["Authorization"] = ensureBasicAuth(authorization)
	}

	var lastErr error
	for i := 1; i <= 3; i++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		resp, err := a.client.PostWithContext(ctx, authQuerySpecTokenURL, headers, reqBody)
		if err != nil {
			lastErr = err
		} else {
			body, readErr := a.client.ReadResponseBody(resp)
			if readErr != nil {
				lastErr = readErr
			} else if resp.StatusCode < 200 || resp.StatusCode >= 300 {
				lastErr = fmt.Errorf("HTTP %d", resp.StatusCode)
			} else {
				var result SpecTokenResp
				if err := json.Unmarshal([]byte(body), &result); err != nil {
					lastErr = fmt.Errorf("解析响应失败: %w", err)
				} else if result.Success && result.Data.Token != "" {
					return result.Data.Token, nil
				} else {
					if result.Message != "" || result.Code != "" {
						lastErr = fmt.Errorf("code=%s, message=%s", result.Code, result.Message)
					} else {
						lastErr = fmt.Errorf("ssoToken 为空")
					}
				}
			}
		}
		if err := sleepAuthContext(ctx, time.Duration(500*i)*time.Millisecond); err != nil {
			return "", err
		}
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("所有重试均失败")
	}
	return "", lastErr
}

func (a *Auth) tyrzLoginWithCandidates(ssoToken string) (string, error) {
	return a.tyrzLoginWithCandidatesContext(context.Background(), ssoToken)
}

func (a *Auth) tyrzLoginWithCandidatesContext(ctx context.Context, ssoToken string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	ssoToken = strings.TrimSpace(ssoToken)
	if ssoToken == "" {
		return "", fmt.Errorf("ssoToken 为空")
	}

	timestamp := a.tyrzTimestampContext(ctx)
	headers := tyrzLoginSignHeaders(timestamp)
	headers["Content-Type"] = "application/json;charset=UTF-8"
	body := struct {
		Token       string `json:"token"`
		OpenAccount int    `json:"openAccount"`
		MarketName  string `json:"marketName"`
		SourceID    string `json:"sourceId"`
	}{
		Token:       ssoToken,
		OpenAccount: 0,
		MarketName:  "sign_in_3",
		SourceID:    "1097",
	}

	requestCtx, cancel := context.WithTimeout(ctx, tyrzLoginTimeout)
	defer cancel()
	resp, err := a.client.RequestWithContextOptions(
		requestCtx,
		http.MethodPost,
		tyrzLoginURL,
		headers,
		body,
		corehttp.WithoutAccountCredentials(),
	)
	if err != nil {
		return "", fmt.Errorf("请求新版 tyrzLogin 失败: %w", err)
	}
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		_, _ = a.client.ReadResponseBody(resp)
		return "", fmt.Errorf("新版 tyrzLogin HTTP 状态异常: %d", resp.StatusCode)
	}

	responseJWT := strings.TrimSpace(resp.Header.Get("jwttoken"))
	responseCookies := resp.Cookies()
	responseBody, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取新版 tyrzLogin 响应失败: %w", err)
	}
	var result TyrzLoginResp
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return "", fmt.Errorf("解析新版 tyrzLogin 响应失败: %w", err)
	}
	if result.Code != 0 {
		return "", fmt.Errorf("新版 tyrzLogin 业务失败: code=%d", result.Code)
	}
	if responseJWT == "" {
		for _, cookie := range responseCookies {
			if cookie.Name == "jwtToken" {
				responseJWT = strings.TrimSpace(cookie.Value)
				break
			}
		}
	}
	if responseJWT == "" {
		return "", fmt.Errorf("新版 tyrzLogin 响应未返回 JWT")
	}
	return responseJWT, nil
}

type tyrzTimestampResponse struct {
	Result json.RawMessage `json:"result"`
}

func (a *Auth) tyrzTimestampContext(ctx context.Context) string {
	fallback := strconv.FormatInt(time.Now().UnixMilli(), 10)
	requestCtx, cancel := context.WithTimeout(ctx, tyrzTimestampTimeout)
	defer cancel()
	resp, err := a.client.RequestWithContextOptions(
		requestCtx,
		http.MethodGet,
		tyrzTimestampURL,
		nil,
		nil,
		corehttp.WithoutAccountCredentials(),
	)
	if err != nil || resp == nil || resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		if resp != nil {
			_, _ = a.client.ReadResponseBody(resp)
		}
		return fallback
	}
	body, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return fallback
	}
	var result tyrzTimestampResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil || len(result.Result) == 0 {
		return fallback
	}
	var timestampString string
	if err := json.Unmarshal(result.Result, &timestampString); err == nil {
		if timestamp, err := strconv.ParseInt(strings.TrimSpace(timestampString), 10, 64); err == nil && timestamp > 0 {
			return strconv.FormatInt(timestamp, 10)
		}
	}
	var timestamp int64
	if err := json.Unmarshal(result.Result, &timestamp); err == nil && timestamp > 0 {
		return strconv.FormatInt(timestamp, 10)
	}
	return fallback
}

func tyrzLoginSignHeaders(timestamp string) map[string]string {
	requestID := uuid.NewString()
	nonce := uuid.NewString()
	signatureInput := tyrzLoginSigningKey + requestID + timestamp + nonce + tyrzLoginSigningKey
	signature := md5.Sum([]byte(signatureInput))
	return map[string]string{
		"x-request-id": requestID,
		"x-timestamp":  timestamp,
		"x-nonce":      nonce,
		"x-signature":  hex.EncodeToString(signature[:]),
	}
}

func sleepAuthContext(ctx context.Context, delay time.Duration) error {
	if delay <= 0 {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func ensureBasicAuth(authValue string) string {
	authValue = strings.TrimSpace(authValue)
	authValue = strings.Trim(authValue, `"`)
	if authValue == "" {
		return ""
	}
	if len(authValue) >= 6 && strings.EqualFold(authValue[:6], "Basic ") {
		return "Basic " + strings.TrimSpace(authValue[6:])
	}
	return "Basic " + authValue
}

func normalizeJSONString(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	var decoded string
	if err := json.Unmarshal([]byte(value), &decoded); err == nil {
		return strings.TrimSpace(decoded)
	}
	return strings.Trim(strings.TrimSpace(value), `"`)
}

func parseRefreshAuthToken(decrypted string) (string, error) {
	decrypted = strings.TrimSpace(decrypted)
	if decrypted == "" {
		return "", fmt.Errorf("刷新响应为空")
	}

	var payload struct {
		Success bool   `json:"success"`
		Code    string `json:"code"`
		Message string `json:"message"`
		Data    struct {
			Token     string `json:"token"`
			AuthToken string `json:"authToken"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(decrypted), &payload); err == nil {
		if !payload.Success {
			if payload.Message != "" || payload.Code != "" {
				return "", fmt.Errorf("刷新 authorization 业务失败: code=%s, message=%s", payload.Code, payload.Message)
			}
			return "", fmt.Errorf("刷新 authorization 业务失败")
		}
		token := strings.TrimSpace(payload.Data.Token)
		if token == "" {
			token = strings.TrimSpace(payload.Data.AuthToken)
		}
		if token == "" {
			return "", fmt.Errorf("刷新 authorization 响应未包含 token")
		}
		return token, nil
	}

	// 兼容极少数服务端直接返回明文 authToken 的情况。
	return decrypted, nil
}

func appRefreshEncrypt(plaintext string) (string, error) {
	block, err := aes.NewCipher(appRefreshAESKey())
	if err != nil {
		return "", err
	}

	iv := make([]byte, aes.BlockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", err
	}

	padded := pkcs7Pad([]byte(plaintext), aes.BlockSize)
	ciphertext := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(ciphertext, padded)

	combined := make([]byte, 0, len(iv)+len(ciphertext))
	combined = append(combined, iv...)
	combined = append(combined, ciphertext...)
	return base64.StdEncoding.EncodeToString(combined), nil
}

func appRefreshDecrypt(cipherB64 string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(cipherB64))
	if err != nil {
		return "", err
	}
	if len(raw) < aes.BlockSize*2 {
		return "", fmt.Errorf("密文长度不足")
	}

	iv := raw[:aes.BlockSize]
	data := raw[aes.BlockSize:]
	if len(data) == 0 || len(data)%aes.BlockSize != 0 {
		return "", fmt.Errorf("密文块长度非法")
	}

	block, err := aes.NewCipher(appRefreshAESKey())
	if err != nil {
		return "", err
	}

	plain := make([]byte, len(data))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, data)
	plain, err = pkcs7Unpad(plain, aes.BlockSize)
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(plain), "\x00"), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	out := make([]byte, len(data)+padding)
	copy(out, data)
	for i := len(data); i < len(out); i++ {
		out[i] = byte(padding)
	}
	return out
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("PKCS7 数据长度非法")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("PKCS7 padding 非法")
	}
	for i := len(data) - padding; i < len(data); i++ {
		if int(data[i]) != padding {
			return nil, fmt.Errorf("PKCS7 padding 不一致")
		}
	}
	return data[:len(data)-padding], nil
}
