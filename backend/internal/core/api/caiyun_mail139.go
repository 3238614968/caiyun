package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

const (
	mail139SSOURL    = "https://orches.yun.139.com/orchestration/auth-rebuild/token/v1.0/querySpecToken"
	mail139LoginURL  = "https://mail.10086.cn/login/inlogin.action"
	mail139BaseURL   = "https://appmail.mail.10086.cn"
	mail139UserAgent = "Mozilla/5.0 (Linux; Android 13; wv) AppleWebKit/537.36 Chrome/108.0.5359.128 Mobile Safari/537.36"
)

// ErrMail139Rejected means the provider explicitly declined the compose request.
// A transport error is deliberately different: delivery might have happened.
var ErrMail139Rejected = errors.New("139 邮箱拒绝发信")

type Mail139Session struct {
	SID   string
	RMKey string
}

func mail139BasicAuthorization(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToLower(value), "basic ") {
		return "Basic " + strings.TrimSpace(value[6:])
	}
	return "Basic " + value
}

func mail139Escape(value string) string {
	var out strings.Builder
	_ = xml.EscapeText(&out, []byte(value))
	return out.String()
}

func mail139GUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}

// Login139Mail obtains an email-specific SSO token and creates a fresh RmWeb
// session. A cloud activity SSO token uses a different source ID and cannot be reused.
func (api *CaiyunAPI) Login139Mail(phone, authorization string) (*Mail139Session, error) {
	phone = strings.TrimSpace(phone)
	authorization = mail139BasicAuthorization(authorization)
	if phone == "" || authorization == "" {
		return nil, fmt.Errorf("139 邮箱登录缺少手机号或 authorization")
	}
	resp, err := api.client.Post(mail139SSOURL, map[string]string{
		"Authorization":      authorization,
		"Main-Authorization": authorization,
		"Content-Type":       "application/json",
		"User-Agent":         mail139UserAgent,
	}, map[string]string{"account": phone, "toSourceId": "001003"})
	if err != nil {
		return nil, fmt.Errorf("获取邮箱 SSO token 失败: %w", err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("邮箱 SSO HTTP %d", resp.StatusCode)
	}
	var sso struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
		Result struct {
			Token string `json:"token"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(body), &sso); err != nil {
		return nil, fmt.Errorf("解析邮箱 SSO 响应失败: %w", err)
	}
	token := sso.Data.Token
	if token == "" {
		token = sso.Result.Token
	}
	if token == "" {
		return nil, fmt.Errorf("邮箱 SSO token 为空")
	}

	loginXML := `<?xml version="1.0" encoding="utf-8"?><object>` +
		`<string name="clientId">10805</string>` +
		`<string name="version">9</string>` +
		`<string name="loginType">7</string>` +
		`<string name="token">` + mail139Escape(token) + `</string>` +
		`<string name="eMode">1</string></object>`
	loginResp, err := api.client.Post(mail139LoginURL, map[string]string{
		"Content-Type": "application/xml",
		"User-Agent":   mail139UserAgent,
	}, loginXML)
	if err != nil {
		return nil, fmt.Errorf("登录 139 邮箱失败: %w", err)
	}
	loginBody, err := api.client.ReadResponseBody(loginResp)
	if err != nil {
		return nil, err
	}
	if loginResp.StatusCode < 200 || loginResp.StatusCode >= 300 {
		return nil, fmt.Errorf("139 邮箱登录 HTTP %d", loginResp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
		Var  struct {
			SID   string `json:"sid"`
			RMKey string `json:"rmkey"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(loginBody), &result); err != nil {
		return nil, fmt.Errorf("解析 139 邮箱登录响应失败: %w", err)
	}
	if result.Code != "S_OK" || result.Var.SID == "" || result.Var.RMKey == "" {
		return nil, fmt.Errorf("139 邮箱登录未成功: code=%s", result.Code)
	}
	return &Mail139Session{SID: result.Var.SID, RMKey: result.Var.RMKey}, nil
}

func build139ComposeXML(senderPhone, recipientEmail, subject, content string) string {
	fields := map[string]string{
		"to": recipientEmail, "cc": "", "bcc": "", "subject": subject,
		"content": content, "account": `"` + senderPhone + `"<` + senderPhone + `@139.com>`,
	}
	var out strings.Builder
	out.WriteString(`<object><object name="attrs">`)
	for _, key := range []string{"to", "cc", "bcc", "subject", "content", "account"} {
		fmt.Fprintf(&out, `<string name="%s">%s</string>`, key, mail139Escape(fields[key]))
	}
	for _, field := range []struct {
		name  string
		value int
	}{
		{"showOneRcpt", 0}, {"isHtml", 0}, {"priority", 3}, {"requestReadReceipt", 0},
		{"saveSentCopy", 1}, {"inlineResources", 0}, {"scheduleDate", 0}, {"normalizeRfc822", 0},
	} {
		fmt.Fprintf(&out, `<int name="%s">%d</int>`, field.name, field.value)
	}
	out.WriteString(`</object><string name="action">deliver</string><int name="returnInfo">1</int></object>`)
	return out.String()
}

func mail139URL(path string, session *Mail139Session, operation string) (string, error) {
	if session == nil || session.SID == "" || session.RMKey == "" {
		return "", fmt.Errorf("139 邮箱会话无效")
	}
	guid, err := mail139GUID()
	if err != nil {
		return "", err
	}
	return mail139BaseURL + path + "?func=" + operation + "&sid=" + url.QueryEscape(session.SID) +
		"&comefrom=5&cguid=" + guid, nil
}

// Send139Mail sends one real message. It does not retry an ambiguous transport
// failure, because the provider might have accepted the email already.
func (api *CaiyunAPI) Send139Mail(session *Mail139Session, senderPhone, recipientEmail, subject, content string) error {
	endpoint, err := mail139URL("/RmWeb/mail", session, "mbox:compose&categroyId=103000000")
	if err != nil {
		return err
	}
	resp, err := api.client.Post(endpoint, map[string]string{
		"Content-Type": "text/xml",
		"Cookie":       "RMKEY=" + session.RMKey,
		"User-Agent":   mail139UserAgent,
		"Referer":      mail139BaseURL + "/",
	}, build139ComposeXML(senderPhone, recipientEmail, subject, content))
	if err != nil {
		return fmt.Errorf("邮箱发信响应不确定: %w", err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return fmt.Errorf("读取邮箱发信响应失败，投递状态不确定: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("邮箱发信 HTTP %d，投递状态不确定", resp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
		Var  struct {
			TID string `json:"tid"`
		} `json:"var"`
	}
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return fmt.Errorf("解析发信响应失败，投递状态不确定: %w", err)
	}
	if result.Code != "S_OK" {
		return fmt.Errorf("%w: code=%s", ErrMail139Rejected, result.Code)
	}
	if result.Var.TID == "" {
		return fmt.Errorf("发信返回 S_OK 但缺少投递编号，状态不确定")
	}
	return nil
}

// Report139MailTask mirrors the mailbox's activity callback. Its failure does
// not mean the email failed to send.
func (api *CaiyunAPI) Report139MailTask(session *Mail139Session) error {
	endpoint, err := mail139URL("/mw2/disk/disk", session, "disk:mailReportToMQ")
	if err != nil {
		return err
	}
	resp, err := api.client.Post(endpoint, map[string]string{
		"Content-Type": "text/xml",
		"Cookie":       "RMKEY=" + session.RMKey,
		"User-Agent":   mail139UserAgent,
		"Referer":      mail139BaseURL + "/m6/html/index.html",
	}, `<object><int name="actionType">1002</int><int name="clientType">3</int>`+
		`<string name="clientVersion">web</string><string name="channel"></string></object>`)
	if err != nil {
		return err
	}
	_, err = api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("邮箱任务上报 HTTP %d", resp.StatusCode)
	}
	return nil
}
