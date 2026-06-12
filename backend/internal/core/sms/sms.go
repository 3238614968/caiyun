package sms

import (
	"caiyun/internal/utils"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

const (
	defaultSMSAPIBaseURL = "https://ydyp.apisky.cn"
	smsAPITimeout        = 30 * time.Second
	maxSMSResponseBytes  = 1 << 20 // 1 MiB
)

// SmsApiResponse 表示短信服务接口响应结构。
type SmsApiResponse struct {
	Code    int                    `json:"code"`
	Message string                 `json:"message"`
	Data    map[string]interface{} `json:"data"`
}

// SlideSolveResult 表示滑块识别服务返回的结果。
type SlideSolveResult struct {
	Offset     int
	Confidence float64
	Method     string
	Attempt    int
}

// CodeStatus 表示验证码任务状态。
type CodeStatus struct {
	Phone     string
	TaskID    string
	Status    string
	CreatedAt string
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
func VerifyCode(phone, smsCode, taskID string) (string, error) {
	payload := map[string]string{
		"phone": phone,
		"code":  smsCode,
	}
	if strings.TrimSpace(taskID) != "" {
		payload["task_id"] = taskID
	}

	apiResp, err := postJSON("/api/sms/verify", payload, phone)
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
func GetCodeStatus(phone string) (*CodeStatus, error) {
	client := newHTTPClient()
	requestURL := buildSMSAPIURL("/api/sms/status/" + url.PathEscape(phone))

	req, err := http.NewRequest(http.MethodGet, requestURL, nil)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %s", err.Error())
	}
	applyCommonHeaders(req)

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[SMS] 查询状态请求失败 phone=%s url=%s err=%v", phone, requestURL, err)
		return nil, fmt.Errorf("请求失败: %s", err.Error())
	}
	defer resp.Body.Close()

	respBody, err := utils.ReadLimitedBody(resp.Body, maxSMSResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[SMS] 查询状态响应异常 phone=%s status=%d body_bytes=%d", maskPhone(phone), resp.StatusCode, len(respBody))
		return nil, fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}

	var apiResp SmsApiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %s", err.Error())
	}
	log.Printf("[SMS] 查询状态响应 phone=%s status=%d code=%d message=%s data_keys=%s",
		maskPhone(phone), resp.StatusCode, apiResp.Code, apiResp.Message, responseDataKeys(apiResp.Data))
	if apiResp.Code != 0 {
		return nil, fmt.Errorf("%s", apiResp.Message)
	}
	if apiResp.Data == nil {
		return nil, fmt.Errorf("响应数据为空")
	}

	status, ok := apiResp.Data["status"].(string)
	if !ok || status == "" {
		return nil, fmt.Errorf("未获取到状态")
	}

	result := &CodeStatus{
		Phone:  phone,
		Status: status,
	}
	if taskID, ok := apiResp.Data["task_id"].(string); ok {
		result.TaskID = taskID
	}
	if createdAt, ok := apiResp.Data["created_at"].(string); ok {
		result.CreatedAt = createdAt
	}

	return result, nil
}

// SolveSlide 调用短信登录服务的 /api/sms/solve 接口识别滑块偏移量。
// 仅依赖远端接口返回的 offset，项目内不做本地图像识别。
func SolveSlide(puzzle, picture string) (*SlideSolveResult, error) {
	puzzle = strings.TrimSpace(puzzle)
	picture = strings.TrimSpace(picture)
	if puzzle == "" || picture == "" {
		return nil, fmt.Errorf("缺少滑块图片数据")
	}

	apiResp, err := postJSON("/api/sms/solve", map[string]string{
		"puzzle":  puzzle,
		"picture": picture,
	}, "")
	if err != nil {
		return nil, err
	}
	if apiResp.Data == nil {
		return nil, fmt.Errorf("滑块识别响应数据为空")
	}

	offset, ok := numberFromSMSData(apiResp.Data["offset"])
	if !ok {
		return nil, fmt.Errorf("滑块识别响应缺少 offset")
	}
	result := &SlideSolveResult{Offset: int(offset)}
	if confidence, ok := numberFromSMSData(apiResp.Data["confidence"]); ok {
		result.Confidence = confidence
	}
	if attempt, ok := numberFromSMSData(apiResp.Data["attempt"]); ok {
		result.Attempt = int(attempt)
	}
	if method, ok := apiResp.Data["method"].(string); ok {
		result.Method = strings.TrimSpace(method)
	}
	return result, nil
}

// postJSON 调用短信服务 JSON POST 接口。
func postJSON(path string, payload map[string]string, phone string) (*SmsApiResponse, error) {
	requestURL := buildSMSAPIURL(path)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("序列化请求失败: %w", err)
	}

	client := newHTTPClient()
	req, err := http.NewRequest(http.MethodPost, requestURL, strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %s", err.Error())
	}
	applyCommonHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		log.Printf("[SMS] 请求失败 phone=%s url=%s err=%v", phone, requestURL, err)
		return nil, fmt.Errorf("请求失败: %s", err.Error())
	}
	defer resp.Body.Close()

	respBody, err := utils.ReadLimitedBody(resp.Body, maxSMSResponseBytes)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %s", err.Error())
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("[SMS] 响应异常 phone=%s path=%s status=%d body_bytes=%d", maskPhone(phone), path, resp.StatusCode, len(respBody))
		return nil, fmt.Errorf("HTTP 状态码异常: %d", resp.StatusCode)
	}

	var apiResp SmsApiResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("解析响应失败: %s", err.Error())
	}
	log.Printf("[SMS] 响应 phone=%s path=%s status=%d code=%d message=%s data_keys=%s",
		maskPhone(phone), path, resp.StatusCode, apiResp.Code, apiResp.Message, responseDataKeys(apiResp.Data))
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

func getSMSAPIToken() string {
	if value := strings.TrimSpace(os.Getenv("CAIYUN_SMS_API_TOKEN")); value != "" {
		return value
	}
	if value := strings.TrimSpace(os.Getenv("DEFAULT_API_TOKEN")); value != "" {
		return value
	}
	return ""
}

func applyCommonHeaders(req *http.Request) {
	req.Header.Set("Accept", "application/json")
	if token := getSMSAPIToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
}

// isSMSTLSSkipVerifyEnabled 判断是否允许跳过 TLS 证书校验。
func isSMSTLSSkipVerifyEnabled() bool {
	value := strings.TrimSpace(strings.ToLower(os.Getenv("CAIYUN_SMS_INSECURE_SKIP_VERIFY")))
	return value == "1" || value == "true" || value == "yes"
}

func maskPhone(phone string) string {
	phone = strings.TrimSpace(phone)
	if len(phone) < 7 {
		return "***"
	}
	return phone[:3] + "****" + phone[len(phone)-4:]
}

func responseDataKeys(data map[string]interface{}) string {
	if len(data) == 0 {
		return ""
	}
	keys := make([]string, 0, len(data))
	for key := range data {
		keys = append(keys, key)
	}
	return strings.Join(keys, ",")
}

func numberFromSMSData(value interface{}) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case json.Number:
		n, err := v.Float64()
		return n, err == nil
	case string:
		v = strings.TrimSpace(v)
		if v == "" {
			return 0, false
		}
		var n json.Number = json.Number(v)
		parsed, err := n.Float64()
		return parsed, err == nil
	default:
		return 0, false
	}
}
