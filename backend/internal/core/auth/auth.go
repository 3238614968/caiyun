package auth

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/http"
	"caiyun/internal/core/utils"
)

// Auth 认证管理器
type Auth struct {
	client *http.Client
}

type TokenRefreshError struct {
	Code     string
	Desc     string
	Response string
}

func (e *TokenRefreshError) Error() string {
	if e == nil {
		return ""
	}
	if e.Desc != "" {
		return fmt.Sprintf("Token刷新失败: code=%s desc=%s", e.Code, e.Desc)
	}
	return fmt.Sprintf("Token刷新失败: code=%s", e.Code)
}

// NewAuth 创建认证管理器
func NewAuth(client *http.Client) *Auth {
	return &Auth{client: client}
}

// UserInfo 用户信息
type UserInfo struct {
	Phone    string
	Token    string
	Platform string
	Expire   int64
	Auth     string // 只包含 base64 部分，不包含 "Basic " 前缀
	AuthFull string // 完整的 "Basic xxx" 格式
}

// ParseToken 解析 Token 字符串
// 支持两种 Auth 格式：
// 1) Basic base64(platform:phone:token)  冒号分隔
// 2) Basic base64(platform|phone|token|expire|...)  竖线分隔（青龙/脚本常见）
func ParseToken(authString string) (*UserInfo, error) {
	authBase64 := strings.TrimPrefix(strings.TrimSpace(authString), "Basic ")
	if authBase64 == "" {
		return nil, fmt.Errorf("Auth 为空")
	}

	normalizedAuthFull := "Basic " + authBase64

	// 优先按冒号分隔解析：platform:phone:token（token 本身通常包含竖线分隔字段）
	platform, phone, token, err := utils.ParseAuthString(normalizedAuthFull)
	if err != nil {
		decoded, decodeErr := base64.StdEncoding.DecodeString(authBase64)
		if decodeErr != nil {
			return nil, fmt.Errorf("解析 Auth 失败: %w", err)
		}

		decodedStr := string(decoded)
		parts := strings.Split(decodedStr, "|")
		if len(parts) < 3 {
			return nil, fmt.Errorf("解析 Auth 失败: %w", err)
		}

		platform = parts[0]
		phone = parts[1]
		token = strings.Join(parts[2:], "|")
	}

	expire := time.Now().Add(30 * 24 * time.Hour).UnixMilli()
	parts := strings.Split(token, "|")
	if len(parts) > 3 {
		if t, e := strconv.ParseInt(parts[3], 10, 64); e == nil && t > 0 {
			if t < 1e12 {
				expire = t * 1000
			} else {
				expire = t
			}
		}
	}

	return &UserInfo{
		Phone:    phone,
		Token:    token,
		Platform: platform,
		Expire:   expire,
		Auth:     authBase64,
		AuthFull: normalizedAuthFull,
	}, nil
}

// GenerateAuth 生成 Auth 字符串
func GenerateAuth(token, phone, platform string) string {
	return utils.GenerateAuthString(platform, phone, token)
}

// RefreshToken 刷新 Token，返回新的 token 字符串
func (a *Auth) RefreshToken(token, phone string) (string, error) {
	// 构建 XML 请求体
	xmlBody := fmt.Sprintf(
		`<?xml version="1.0" encoding="utf-8"?><root><token>%s</token><account>%s</account><clienttype>656</clienttype></root>`,
		token,
		phone,
	)

	// 设置请求头
	headers := map[string]string{
		"Accept":       "*/*",
		"Content-Type": "application/json; charset=utf-8",
	}

	// 调用正确的 authTokenRefresh API
	resp, err := a.client.Post(
		"https://aas.caiyun.feixin.10086.cn/tellin/authTokenRefresh.do",
		headers,
		xmlBody,
	)

	if err != nil {
		return "", fmt.Errorf("刷新 Token 请求失败: %w", err)
	}

	responseBody, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取响应失败: %w", err)
	}

	if code := utils.ExtractXMLTag(responseBody, "return"); code != "" && code != "0" {
		desc := utils.ExtractXMLTag(responseBody, "desc")
		return "", &TokenRefreshError{
			Code:     code,
			Desc:     desc,
			Response: responseBody,
		}
	}

	// 从 XML 响应中提取 token
	newToken := utils.ExtractXMLTag(responseBody, "token")
	if newToken == "" {
		return "", fmt.Errorf("从响应中提取 token 失败，响应内容: %s", responseBody)
	}

	return newToken, nil
}

// SpecTokenResp SpecToken 响应
type SpecTokenResp struct {
	Success bool   `json:"success"`
	Code    string `json:"code"` // 注意：code 是字符串类型
	Message string `json:"message"`
	Data    struct {
		Token string `json:"token"`
	} `json:"data"`
}

// QuerySpecToken 获取 SpecToken
func (a *Auth) QuerySpecToken(phone string) (*SpecTokenResp, error) {
	// 构建 SpecToken 请求 URL
	url := fmt.Sprintf("https://caiyun.feixin.10086.cn/portal/auth/querySpecToken.action?phone=%s", phone)

	resp, err := a.client.Get(url, nil)
	if err != nil {
		return nil, fmt.Errorf("获取 SpecToken 请求失败: %w", err)
	}

	body, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result SpecTokenResp
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return &result, nil
}

// LoginMailResp 邮箱登录响应
type LoginMailResp struct {
	Code    string `xml:"code"`
	Summary string `xml:"summary"`
	Var     struct {
		Sid   string `xml:"sid"`
		Rmkey string `xml:"rmkey"`
	} `xml:"var"`
}

// LoginMail 邮箱登录
func (a *Auth) LoginMail(ssoToken string) (*LoginMailResp, error) {
	// 构建 XML 请求体
	xmlBody := fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
      <object>
       <string name="clientId">10804</string> 
       <string name="version">9</string>
       <string name="loginType">7</string> 
       <string name="token">%s</string> 
      </object>`, ssoToken)

	headers := map[string]string{
		"Content-Type": "application/xml",
	}

	resp, err := a.client.Post(
		"https://mail.10086.cn/login/inlogin.action",
		headers,
		xmlBody,
	)

	if err != nil {
		return nil, fmt.Errorf("邮箱登录请求失败: %w", err)
	}

	body, err := a.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}

	var result LoginMailResp

	// 尝试 JSON 解析（mail.10086.cn 可能返回 JSON）
	var jsonResult struct {
		Code    string `json:"code"`
		Summary string `json:"summary"`
		Var     struct {
			Sid   string `json:"sid"`
			Rmkey string `json:"rmkey"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(body), &jsonResult); err == nil {
		result.Code = jsonResult.Code
		result.Summary = jsonResult.Summary
		result.Var.Sid = jsonResult.Var.Sid
		result.Var.Rmkey = jsonResult.Var.Rmkey
		return &result, nil
	}

	// 降级为 XML 解析
	result.Code = utils.ExtractXMLTag(body, "code")
	if result.Code == "" {
		return nil, fmt.Errorf("解析响应失败，未找到 code 字段: %s", body)
	}

	result.Summary = utils.ExtractXMLTag(body, "summary")

	// 提取 var 中的 sid 和 rmkey
	varContent := utils.ExtractXMLTag(body, "var")
	if varContent != "" {
		parts := strings.Split(varContent, "&")
		for _, part := range parts {
			kv := strings.Split(part, "=")
			if len(kv) == 2 {
				if kv[0] == "sid" {
					result.Var.Sid = kv[1]
				} else if kv[0] == "rmkey" {
					result.Var.Rmkey = kv[1]
				}
			}
		}
	}

	return &result, nil
}

// ParseLoginVar 解析邮箱登录的 var 字段
func ParseLoginVar(varStr string) (sid, rmkey string, err error) {
	parts := strings.Split(varStr, "&")
	for _, part := range parts {
		kv := strings.Split(part, "=")
		if len(kv) == 2 {
			if kv[0] == "sid" {
				sid = kv[1]
			} else if kv[0] == "rmkey" {
				rmkey = kv[1]
			}
		}
	}

	if sid == "" || rmkey == "" {
		err = fmt.Errorf("解析 var 字段失败")
		return
	}

	return
}

// QuerySpecTokenForJWT 获取 ssoToken（用于 JWT）
func (a *Auth) QuerySpecTokenForJWT(phone string) (string, error) {
	return a.querySpecTokenWithRetry("", phone)
}

// TyrzLoginResp tyrzLogin 响应
type TyrzLoginResp struct {
	Code   int `json:"code"`
	Result struct {
		Token string `json:"token"`
	} `json:"result"`
}

// TyrzLogin 使用 ssoToken 获取 JWT token
func (a *Auth) TyrzLogin(ssoToken string) (string, error) {
	return a.tyrzLoginWithCandidates(ssoToken)
}

// GetJWTTokenWithSSOToken 获取 JWT Token 和对应的 ssoToken（单次 sso 查询）
func (a *Auth) GetJWTTokenWithSSOToken(phone string) (string, string, error) {
	// 1. 获取 ssoToken
	ssoToken, err := a.QuerySpecTokenForJWT(phone)
	if err != nil {
		return "", "", fmt.Errorf("获取 ssoToken 失败: %w", err)
	}

	if ssoToken == "" {
		return "", "", fmt.Errorf("ssoToken 为空")
	}

	// 2. 使用 ssoToken 获取 JWT token
	jwtToken, err := a.TyrzLogin(ssoToken)
	if err != nil {
		return "", ssoToken, fmt.Errorf("获取 JWT token 失败: %w", err)
	}

	return jwtToken, ssoToken, nil
}

// GetJWTToken 获取 JWT Token（完整流程）
func (a *Auth) GetJWTToken(phone string) (string, error) {
	jwtToken, _, err := a.GetJWTTokenWithSSOToken(phone)
	if err != nil {
		return "", err
	}
	return jwtToken, nil
}
