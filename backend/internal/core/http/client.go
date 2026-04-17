package http

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// Client HTTP 客户端
type Client struct {
	client     *http.Client
	userAgent  string
	auth       string // Basic Auth
	jwtToken   string
	clientInfo string
	deviceInfo string
	netType    string
	channelSrc string
	cookieJar  *cookiejar.Jar
}

// NewClient 创建 HTTP 客户端
func NewClient() *Client {
	jar, _ := cookiejar.New(nil)

	// 使用 Android 客户端信息（与 mjs 源码一致）
	androidClientInfo := "1|127.0.0.1|1|12.0.1|Xiaomi|22041216C||02-00-00-00-00-00|android 14|1080X2360|zh||||032|0|"

	return &Client{
		client: &http.Client{
			Timeout: 30 * time.Second,
			Jar:     jar,
		},
		userAgent:  "Mozilla/5.0 (Linux; Android 14; 22041216C Build/TP1A.220624.014; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/128.0.6613.88 Mobile Safari/537.36",
		clientInfo: androidClientInfo,
		deviceInfo: androidClientInfo,
		netType:    "1",
		channelSrc: "10000034",
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

	for _, domain := range []string{"m.mcloud.139.com", "mrp.mcloud.139.com", "caiyun.feixin.10086.cn"} {
		c.SetCookie("jwtToken", token, domain)
	}
	c.SetCookie("sensors_stay_time", fmt.Sprintf("%d", time.Now().UnixMilli()), "m.mcloud.139.com")
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
			time.Sleep(retryDelay)
		}

		// 每次重新创建 body reader
		var reqBody io.Reader
		if bodyBytes != nil {
			reqBody = bytes.NewReader(bodyBytes)
		}

		req, err := http.NewRequest(method, reqURL, reqBody)
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

// Post POST 请求
func (c *Client) Post(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("POST", url, headers, body)
}

// Put PUT 请求
func (c *Client) Put(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("PUT", url, headers, body)
}

// Delete DELETE 请求
func (c *Client) Delete(url string, headers map[string]string, body interface{}) (*http.Response, error) {
	return c.Request("DELETE", url, headers, body)
}

// ParseJSONResponse 解析 JSON 响应
func (c *Client) ParseJSONResponse(resp *http.Response, result interface{}) error {
	if resp == nil {
		return fmt.Errorf("响应为空")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("读取响应体失败: %w", err)
	}

	if err := json.Unmarshal(body, result); err != nil {
		return fmt.Errorf("解析 JSON 失败: %w, body: %s", err, string(body))
	}

	return nil
}

// ReadResponseBody 读取响应体（返回字符串）
func (c *Client) ReadResponseBody(resp *http.Response) (string, error) {
	if resp == nil {
		return "", fmt.Errorf("响应为空")
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("读取响应体失败: %w", err)
	}

	return string(body), nil
}

// Sleep 休眠（毫秒）
func (c *Client) Sleep(ms int) {
	time.Sleep(time.Duration(ms) * time.Millisecond)
}
