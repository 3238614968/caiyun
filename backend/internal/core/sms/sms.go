package sms

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultSMSAPIBaseURL = "https://smscaiyun.779776.xyz"
	smsAPITimeout        = 30 * time.Second
)

// SmsApiResponse 表示短信服务接口响应结构。
type SmsApiResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

// newHTTPClient 创建短信接口客户端。
// 默认启用证书校验，仅在显式配置环境变量时才允许跳过。
func newHTTPClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: isSMSTLSSkipVerifyEnabled(),
	}

	if transport.TLSClientConfig.InsecureSkipVerify {
		log.Println("[SMS] 警告: 已启用 CAIYUN_SMS_INSECURE_SKIP_VERIFY=true，TLS 证书校验被关闭")
	}

	return &http.Client{
		Timeout:   smsAPITimeout,
		Transport: transport,
	}
}

// SendCode 发送短信验证码，返回 task_id。
func SendCode(phone string) (string, error) {
	apiResp, err := postJSON("/api/sms/send", map[string]string{"phone": phone}, phone)
	if err != nil {
		return "", err
	}

	var taskID string
	if apiResp.Data != nil {
		if tid, ok := apiResp.Data["task_id"].(string); ok {
			taskID = tid
		}
	}

	return taskID, nil
}

// VerifyCode 校验短信验证码，返回 authorization。
func VerifyCode(phone, smsCode string) (string, error) {
	apiResp, err := postJSON("/api/sms/verify", map[string]string{
		"phone":    phone,
		"sms_code": smsCode,
	}, phone)
	if err != nil {
		return "", err
	}

	if apiResp.Data == nil {
		return "", fmt.Errorf("响应数据为空")
	}

	authorization, ok := apiResp.Data["authorization"].(string)
	if !ok || authorization == "" {
		return "", fmt.Errorf("未获取到 authorization")
	}

	return authorization, nil
}

// GetCodeStatus 查询验证码发送状态。
func GetCodeStatus(phone string) (string, error) {
	client := newHTTPClient()
	requestURL := buildSMSAPIURL("/api/sms/status/" + url.PathEscape(phone))

	resp, err := client.Get(requestURL)
	if err != nil {
		log.Printf("[SMS] 查询状态请求失败 phone=%s url=%s err=%v", phone, requestURL, err)
		return "", fmt.Errorf("请求失败: %s", err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %s", err.Error())
	}

	log.Printf("[SMS] 查询状态响应 phone=%s status=%d body=%s", phone, resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}

	var apiResp SmsApiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return "", fmt.Errorf("解析响应失败: %s", err.Error())
	}
	if apiResp.Code != 0 {
		return "", fmt.Errorf("%s", apiResp.Message)
	}
	if apiResp.Data == nil {
		return "", fmt.Errorf("响应数据为空")
	}

	status, ok := apiResp.Data["status"].(string)
	if !ok || status == "" {
		return "", fmt.Errorf("未获取到状态")
	}

	return status, nil
}

// postJSON 调用短信服务 JSON POST 接口。
func postJSON(path string, payload map[string]string, phone string) (*SmsApiResponse, error) {
	requestURL := buildSMSAPIURL(path)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	client := newHTTPClient()
	resp, err := client.Post(requestURL, "application/json", strings.NewReader(string(body)))
	if err != nil {
		log.Printf("[SMS] 请求失败 phone=%s url=%s err=%v", phone, requestURL, err)
		return nil, fmt.Errorf("请求失败: %s", err.Error())
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %s", err.Error())
	}

	log.Printf("[SMS] 响应 phone=%s path=%s status=%d body=%s", phone, path, resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}

	var apiResp SmsApiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %s", err.Error())
	}
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s", apiResp.Message)
	}

	return &apiResp, nil
}

// buildSMSAPIURL 拼接短信服务接口地址。
func buildSMSAPIURL(path string) string {
	baseURL := strings.TrimRight(getSMSAPIBaseURL(), "/")
	return baseURL + path
}

// getSMSAPIBaseURL 读取短信服务基础地址。
func getSMSAPIBaseURL() string {
	if value := strings.TrimSpace(os.Getenv("CAIYUN_SMS_API_BASE_URL")); value != "" {
		return value
	}
	return defaultSMSAPIBaseURL
}

// isSMSTLSSkipVerifyEnabled 判断是否允许跳过 TLS 证书校验。
func isSMSTLSSkipVerifyEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY")))
	return value == "1" || value == "true" || value == "yes"
}
