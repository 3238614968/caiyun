package http

import (
	"bytes"
	"caiyun/internal/core/shumei"
	"caiyun/internal/utils"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
)

// Client HTTP 客户端
type Client struct {
	client     *http.Client
	userAgent  string
	auth       string // Basic Auth
	jwtToken   string
	ssoToken   string
	userDomain string
	clientInfo string
	deviceInfo string
	deviceID   string
	deviceMu   sync.RWMutex
	deviceOnce sync.Once
	deviceErr  error
	deviceLive bool
	account    string
	netType    string
	channelSrc string
	cookieJar  *cookiejar.Jar
}

// NewClient 创建 HTTP 客户端
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)

	// 使用新版 Android 客户端信息（与最新版移动云盘脚本一致）
	androidClientInfo := "6|127.0.0.1|1|12.5.4|realme|RMX5060|BCFF2BBA6881DD8E4971803C63DDB5E4|02-00-00-00-00-00|android 15|1264X2592|zh||||032|0|"
	deviceID := "BCFF2BBA6881DD8E4971803C63DDB5E4"

	return &Client{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		userAgent:  shumei.RandomMarketUserAgent(),
		clientInfo: androidClientInfo,
		deviceInfo: androidClientInfo,
		deviceID:   deviceID,
		netType:    "1",
		channelSrc: "10000023",
		cookieJar:  jar,
	}
}

// SetAuth 设置认证信息（只存储base64部分，不包含"Basic "前缀）
func (c *Client) SetAuth(auth string) {
	// 移除 "Basic " 前缀（如果存在）
	c.auth = strings.TrimPrefix(auth, "Basic ")
}

// SetJWTToken 设置 JWT Token
func (c *Client) SetJWTToken(token string) {
	c.jwtToken = token
	if token == "" {
		return
	}
	c.userDomain = extractUserDomainIDFromJWT(token)

	for _, domain := range []string{"m.mcloud.139.com", "mrp.mcloud.139.com", "caiyun.feixin.10086.cn"} {
		c.SetCookie("jwtToken", token, domain)
	}
	c.SetCookie("sensors_stay_time", fmt.Sprintf("%d", time.Now().UnixMilli()), "m.mcloud.139.com")
	if c.userDomain != "" {
		c.SetCookie("userDomainId", c.userDomain, "m.mcloud.139.com")
	}
}

// SetSSOToken 设置当前 SSO Token
func (c *Client) SetSSOToken(token string) {
	c.ssoToken = strings.TrimSpace(token)
}

// SetMarketAccount 设置当前账号标识，用于写入新版签到页依赖的 .thumbcache_* 设备指纹 Cookie。
func (c *Client) SetMarketAccount(account string) {
	account = strings.TrimSpace(account)

	c.deviceMu.Lock()
	c.account = account
	deviceID := c.deviceID
	deviceLive := c.deviceLive
	c.deviceMu.Unlock()

	if deviceLive {
		c.seedMarketDeviceCookie(deviceID, account)
	}
}

// SetUserAgent 设置 User-Agent
func (c *Client) SetUserAgent(ua string) {
	c.userAgent = ua
}

// SetClientInfo 设置客户端信息
func (c *Client) SetClientInfo(info string) {
	c.clientInfo = info
	c.deviceInfo = info
}

// GetJWTToken 获取当前 JWT Token
func (c *Client) GetJWTToken() string {
	return c.jwtToken
}

// GetSSOToken 获取当前 SSO Token
func (c *Client) GetSSOToken() string {
	return c.ssoToken
}

// GetUserDomainID 获取当前 userDomainId
func (c *Client) GetUserDomainID() string {
	return c.userDomain
}

// GetDeviceID 获取当前设备标识
func (c *Client) GetDeviceID() string {
	c.deviceMu.RLock()
	defer c.deviceMu.RUnlock()
	return c.deviceID
}

// SetDeviceID 设置当前设备标识。传入不带 B 前缀的数美 deviceId 时会自动补齐。
func (c *Client) SetDeviceID(deviceID string) {
	c.setDeviceID(deviceID, true)
}

// EnsureShumeiDeviceID 懒加载数美 deviceprofile/v4 deviceId。
func (c *Client) EnsureShumeiDeviceID(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	c.deviceOnce.Do(func() {
		var err error
		if manualDeviceID := strings.TrimSpace(os.Getenv("CAIYUN_SHUMEI_DEVICE_ID")); manualDeviceID != "" {
			c.setDeviceID(manualDeviceID, true)
		} else {
			var deviceID string
			deviceID, err = shumei.FetchDeviceID(ctx, c.client, "")
			if err == nil && deviceID != "" {
				c.setDeviceID(deviceID, true)
			}
		}

		c.deviceMu.Lock()
		c.deviceErr = err
		c.deviceMu.Unlock()
	})

	c.deviceMu.RLock()
	defer c.deviceMu.RUnlock()
	if c.deviceLive && c.deviceID != "" {
		return nil
	}
	return c.deviceErr
}

// SetCookie 设置 Cookie
func (c *Client) SetCookie(name, value, domain string) {
	u, _ := url.Parse("https://" + domain)
	cookie := &http.Cookie{
		Name:   name,
		Value:  value,
		Domain: domain,
		Path:   "/",
	}
	c.cookieJar.SetCookies(u, []*http.Cookie{cookie})
}

func (c *Client) setDeviceID(deviceID string, deviceLive bool) {
	deviceID = shumei.NormalizeDeviceID(deviceID)
	if deviceID == "" {
		return
	}

	c.deviceMu.Lock()
	c.deviceID = deviceID
	if deviceLive {
		c.deviceLive = true
	}
	account := c.account
	shouldSeedCookie := c.deviceLive
	c.deviceMu.Unlock()

	if shouldSeedCookie {
		c.seedMarketDeviceCookie(deviceID, account)
	}
}

func (c *Client) seedMarketDeviceCookie(deviceID, account string) {
	cookieValue := shumei.CookieDeviceValue(deviceID)
	if cookieValue == "" {
		return
	}

	c.SetCookie(".thumbcache_caiyun", cookieValue, "m.mcloud.139.com")
	if cookieName := marketDeviceCookieName(account); cookieName != "" {
		c.SetCookie(cookieName, cookieValue, "m.mcloud.139.com")
	}
}

func marketDeviceCookieName(account string) string {
	account = strings.TrimSpace(account)
	if account == "" {
		return ""
	}

	var builder strings.Builder
	for _, r := range account {
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
		return ""
	}
	return ".thumbcache_" + builder.String()
}

// GetCookies 获取指定域名的所有 Cookie
func (c *Client) GetCookies(domain string) []*http.Cookie {
	u, _ := url.Parse("https://" + domain)
	return c.cookieJar.Cookies(u)
}

// buildHeaders 构建请求头
func (c *Client) buildHeaders(reqURL string, customHeaders map[string]string) map[string]string {
	headers := make(map[string]string)

	// 默认请求头
	headers["User-Agent"] = c.userAgent
	headers["Accept"] = "application/json, text/plain, */*"
	headers["Content-Type"] = "application/json;charset=UTF-8"
	headers["x-yun-client-info"] = c.clientInfo
	headers["x-DeviceInfo"] = c.deviceInfo
	headers["x-NetType"] = c.netType
	headers["x-requested-with"] = "com.chinamobile.mcloud"
	headers["charset"] = "utf-8"
	if deviceID := c.GetDeviceID(); deviceID != "" {
		headers["deviceId"] = deviceID
	}

	// 解析 URL
	u, _ := url.Parse(reqURL)
	hostname := u.Hostname()

	// 根据域名设置 Authorization
	// c.auth 存储的是纯 base64 字符串（不包含 "Basic " 前缀）
	// 对于 caiyun/mrp/m 域名:
	//   authorization = "Basic " + c.auth
	//   jwttoken = jwtToken
	// 对于其他域名:
	//   authorization = "Basic " + c.auth
	if strings.Contains(hostname, "caiyun.feixin.10086.cn") ||
		strings.Contains(hostname, "mrp.mcloud.139.com") ||
		strings.Contains(hostname, "m.mcloud.139.com") {
		if c.jwtToken != "" {
			headers["jwttoken"] = c.jwtToken
			headers["jwtToken"] = c.jwtToken
		}
		if c.auth != "" {
			headers["Authorization"] = "Basic " + c.auth
		}
	} else if c.auth != "" {
		headers["Authorization"] = "Basic " + c.auth
	}

	// 覆盖自定义请求头
	for key, value := range customHeaders {
		if value != "" {
			headers[key] = value
		}
	}

	return headers
}

// Request HTTP 请求方法（带重试）
// 重试策略：网络错误、超时、5xx 状态码自动重试，最多 3 次
// 4xx 状态码不重试，立即返回
func (c *Client) Request(method, reqURL string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.RequestWithContext(context.Background(), method, reqURL, headers, body)
}

// RequestWithContext HTTP 请求方法（带重试与 context 取消）。
// 重试策略：网络错误、超时、5xx 状态码自动重试，最多 3 次。
// 4xx 状态码不重试，立即返回。
func (c *Client) RequestWithContext(ctx context.Context, method, reqURL string, headers map[string]string, body interface{}) (*http.Response, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	const maxRetries = 3
	const retryDelay = 1 * time.Second

	// 预先序列化请求体为 []byte，确保重试时可以重新构建 Reader
	var bodyBytes []byte
	if body != nil {
		switch v := body.(type) {
		case string:
			bodyBytes = []byte(v)
		case []byte:
			bodyBytes = v
		default:
			jsonData, err := json.Marshal(body)
			if err != nil {
				return nil, fmt.Errorf("序列化请求体失败: %w", err)
			}
			bodyBytes = jsonData
		}
	}

	var lastResp *http.Response
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// 重试前等待 1 秒（首次请求不等待）
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryDelay):
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}

		// 每次重新创建 body reader
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequestWithContext(ctx, method, reqURL, reqBody)
		if err != nil {
			return nil, fmt.Errorf("创建请求失败: %w", err)
		}

		// 构建并设置请求头
		allHeaders := c.buildHeaders(reqURL, headers)
		for key, value := range allHeaders {
			// 直接设置header map，避免key被规范化
			// 服务器可能只识别小写的header key（如jwttoken）
			req.Header[key] = []string{value}
		}

		// 执行请求
		resp, err := c.client.Do(req)
		if err != nil {
			// 网络错误/超时 → 记录错误，继续重试
			lastErr = fmt.Errorf("请求失败: %w", err)
			lastResp = nil
			continue
		}

		// HTTP 5xx → 关闭响应体防止连接泄漏，继续重试
		if resp.StatusCode >= 500 {
			lastResp = resp
			lastErr = nil
			// 非最后一次尝试时关闭响应体，防止连接泄漏
			if attempt < maxRetries {
				resp.Body.Close()
			}
			continue
		}

		// 成功或 4xx → 立即返回，不重试
		return resp, nil
	}

	// 全部失败，返回最后一次结果
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}

// Get GET 请求
func (c *Client) Get(url string, headers map[string]string) (*http.Response, error) {
	return c.Request("GET", url, headers, nil)
}

func (c *Client) GetWithContext(ctx context.Context, url string, headers map[string]string) (*http.Response, error) {
	return c.RequestWithContext(ctx, "GET", url, headers, nil)
}

// Post POST 请求
func (c *Client) Post(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("POST", url, headers, body)
}

func (c *Client) PostWithContext(ctx context.Context, url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.RequestWithContext(ctx, "POST", url, headers, body)
}

// Put PUT 请求
func (c *Client) Put(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("PUT", url, headers, body)
}

func (c *Client) PutWithContext(ctx context.Context, url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.RequestWithContext(ctx, "PUT", url, headers, body)
}

// Delete DELETE 请求
func (c *Client) Delete(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("DELETE", url, headers, body)
}

func (c *Client) DeleteWithContext(ctx context.Context, url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.RequestWithContext(ctx, "DELETE", url, headers, body)
}

// ParseJSONResponse 解析 JSON 响应
func (c *Client) ParseJSONResponse(resp *http.Response, result interface{}) error {
	if resp == nil {
		return fmt.Errorf("响应为空")
	}
	defer resp.Body.Close()

	body, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		// 截断上游响应体片段，避免把整个 5MB 响应塞进 error 链路造成日志膨胀或泄漏。
		snippet := string(body)
		if len(snippet) > 512 {
			snippet = snippet[:512] + "...(truncated)"
		}
		return fmt.Errorf("解析 JSON 失败: %w, body: %s", err, snippet)
	}

	return nil
}

// ReadResponseBody 读取响应体（返回字符串）
func (c *Client) ReadResponseBody(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("响应为空")
	}
	defer resp.Body.Close()

	body, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}

	return string(body), nil
}

// Sleep 休眠（毫秒）
func (c *Client) Sleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}

func extractUserDomainIDFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) < 2 {
		return ""
	}

	payload := parts[1]
	data, err := base64.RawURLEncoding.DecodeString(payload)
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
