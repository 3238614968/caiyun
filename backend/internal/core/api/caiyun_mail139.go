package api

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net/url"
	"regexp"
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
	if err := decode139MailResponse(loginBody, &result); err != nil {
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
	from := "5"
	if strings.HasPrefix(operation, "disk:") {
		from = "54"
	}
	return mail139BaseURL + path + "?func=" + operation + "&sid=" + url.QueryEscape(session.SID) +
		"&comefrom=" + from + "&cguid=" + guid, nil
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
	if err := decode139MailResponse(body, &result); err != nil {
		return fmt.Errorf("解析发信响应失败，投递状态不确定: %w", err)
	}
	if result.Code == "" {
		return fmt.Errorf("发信响应缺少结果码，投递状态不确定")
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
	endpoint, err := mail139URL("/mw2/file/disk", session, "disk:mailReportToMQ")
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
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("邮箱任务上报 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := decode139MailResponse(body, &result); err != nil {
		return fmt.Errorf("解析邮箱上报结果失败: %w", err)
	}
	if result.Code != "S_OK" {
		return fmt.Errorf("邮箱任务上报未成功: code=%s", result.Code)
	}
	return nil
}

// Backup139Mail stores real mailbox messages in the cloud (task 1020).
func (api *CaiyunAPI) Backup139Mail(session *Mail139Session) error {
	endpoint, err := mail139URL("/mw2/file/disk", session, "disk:backupMail")
	if err != nil {
		return err
	}
	resp, err := api.client.Post(endpoint, map[string]string{
		"Content-Type": "text/xml", "Cookie": "RMKEY=" + session.RMKey,
		"User-Agent": mail139UserAgent, "Referer": mail139BaseURL + "/m6/html/index.html",
	}, `<object></object>`)
	if err != nil {
		return err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("邮箱转存 HTTP %d", resp.StatusCode)
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := decode139MailResponse(body, &result); err != nil {
		return err
	}
	if result.Code != "S_OK" {
		return fmt.Errorf("邮箱转存未成功: code=%s", result.Code)
	}
	return nil
}

// post139Mail 向 RmWeb/disk 端点提交 XML 并返回原始响应体。
// mbox:listMessages 的响应不是严格 JSON（单引号、null/true 混排），
// 调用方需要自行解析，因此这里只负责传输与 HTTP 状态。
func (api *CaiyunAPI) post139Mail(session *Mail139Session, path, operation, payload string) (string, error) {
	endpoint, err := mail139URL(path, session, operation)
	if err != nil {
		return "", err
	}
	resp, err := api.client.Post(endpoint, map[string]string{
		"Content-Type": "text/xml",
		"Cookie":       "RMKEY=" + session.RMKey,
		"User-Agent":   mail139UserAgent,
		"Referer":      mail139BaseURL + "/m6/html/index.html",
	}, payload)
	if err != nil {
		return "", err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("139 邮箱请求 HTTP %d", resp.StatusCode)
	}
	return body, nil
}

// Export139MailPDF 把指定邮件导出为 PDF（任务 1014 的真实端点）。
// 实测 `GET /bmail/s?func=mail:exportPdf&sid=&mid=&title=` 返回 %PDF-1.7；
// 与参考实现里的 disk:backupMail convertType=1 不同，这个接口只导出单封邮件，
// 不会触发整箱转存。任务是否翻转仍以服务端状态为准。
func (api *CaiyunAPI) Export139MailPDF(session *Mail139Session, mid, title string) error {
	if session == nil || session.SID == "" || session.RMKey == "" {
		return fmt.Errorf("139 邮箱会话无效")
	}
	mid = strings.TrimSpace(mid)
	if mid == "" {
		return fmt.Errorf("邮件 mid 为空")
	}
	if strings.TrimSpace(title) == "" {
		title = "mcloud"
	}
	query := url.Values{}
	query.Set("func", "mail:exportPdf")
	query.Set("sid", session.SID)
	query.Set("mid", mid)
	query.Set("title", title)
	resp, err := api.client.Get(mail139BaseURL+"/bmail/s?"+query.Encode(), map[string]string{
		"Cookie":     "RMKEY=" + session.RMKey,
		"User-Agent": mail139UserAgent,
		"Referer":    mail139BaseURL + "/m6/html/index.html",
	})
	if err != nil {
		return err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("PDF 导出 HTTP %d", resp.StatusCode)
	}
	if !strings.HasPrefix(strings.TrimSpace(body), "%PDF") {
		return fmt.Errorf("PDF 导出响应不是 PDF")
	}
	return nil
}

// BackupAllBillMail 转存全部账单邮件（任务 1029 的真实端点）。
// 实测空 body 返回 S_OK；具体邮件仍可再按 mailIds 转存一次。
func (api *CaiyunAPI) BackupAllBillMail(session *Mail139Session) error {
	body, err := api.post139Mail(session, "/mw2/file/disk", "disk:backupAllBillMail", `<object></object>`)
	if err != nil {
		return err
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := decode139MailResponse(body, &result); err != nil {
		return fmt.Errorf("解析账单邮件转存结果失败: %w", err)
	}
	if result.Code != "S_OK" {
		return fmt.Errorf("账单邮件转存未成功: code=%s", result.Code)
	}
	return nil
}

// List139MailMessages 拉取一页邮件列表原始响应（mbox:listMessages）。
// start 从 1 开始，与参考实现的 RmWeb 分页一致。
func (api *CaiyunAPI) List139MailMessages(session *Mail139Session, start, total int) (string, error) {
	if start < 1 {
		start = 1
	}
	if total <= 0 || total > 100 {
		total = 30
	}
	payload := fmt.Sprintf(
		`<object><int name="fid">1</int><string name="order">date</string><int name="desc">1</int><int name="start">%d</int><int name="total">%d</int></object>`,
		start, total)
	return api.post139Mail(session, "/RmWeb/mail", "mbox:listMessages", payload)
}

// Read139MailMessage 标记一封邮件为已读（任务 1029 的邮箱侧动作）。
func (api *CaiyunAPI) Read139MailMessage(session *Mail139Session, mid string) error {
	mid = strings.TrimSpace(mid)
	if mid == "" {
		return fmt.Errorf("邮件 mid 为空")
	}
	body, err := api.post139Mail(session, "/RmWeb/mail", "mbox:readMessage",
		`<object><string name="mid">`+mail139Escape(mid)+`</string></object>`)
	if err != nil {
		return err
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := decode139MailResponse(body, &result); err == nil && result.Code != "" && result.Code != "S_OK" {
		return fmt.Errorf("邮件已读上报未成功: code=%s", result.Code)
	}
	return nil
}

// Backup139MailMessages 把指定邮件转存到云盘（任务 1029 的邮箱侧动作）。
func (api *CaiyunAPI) Backup139MailMessages(session *Mail139Session, mailIDs, channel string) error {
	mailIDs = strings.TrimSpace(mailIDs)
	if mailIDs == "" {
		return fmt.Errorf("待转存邮件 ID 为空")
	}
	if strings.TrimSpace(channel) == "" {
		channel = "10242000"
	}
	body, err := api.post139Mail(session, "/mw2/file/disk", "disk:backupMail",
		`<object><string name="mailIds">`+mail139Escape(mailIDs)+`</string>`+
			`<string name="backupType"></string>`+
			`<string name="channel">`+mail139Escape(channel)+`</string></object>`)
	if err != nil {
		return err
	}
	var result struct {
		Code string `json:"code"`
	}
	if err := decode139MailResponse(body, &result); err != nil {
		return fmt.Errorf("解析邮件转存结果失败: %w", err)
	}
	if result.Code != "S_OK" {
		return fmt.Errorf("指定邮件转存未成功: code=%s", result.Code)
	}
	return nil
}

// Mail139ListMessage 是从 mbox:listMessages 原始文本中解析出的一条邮件。
type Mail139ListMessage struct {
	MID     string
	Subject string
}

var (
	// mid 可能是 26 位十六进制，也可能是旧邮件的 8~14 位纯数字。
	mail139MIDPattern     = regexp.MustCompile(`(?i)['"]mid['"]\s*:\s*['"]?([A-Za-z0-9_-]{8,})`)
	mail139SubjectPattern = regexp.MustCompile(`(?i)['"]subject['"]\s*:\s*['"]?([^"'\r\n]{2,80})`)
)

// Parse139MailList 解析 RmWeb 邮件列表（响应为单引号/裸值混排，不是 JSON）。
// 真实响应每条记录以 mid 开头、后续字段包含 subject；按 mid 分段配对，
// 避免某条记录缺少 subject 时整体错位。
func Parse139MailList(raw string) []Mail139ListMessage {
	midMatches := mail139MIDPattern.FindAllStringSubmatchIndex(raw, -1)
	if len(midMatches) == 0 {
		return nil
	}
	messages := make([]Mail139ListMessage, 0, len(midMatches))
	for i, match := range midMatches {
		end := len(raw)
		if i+1 < len(midMatches) {
			end = midMatches[i+1][0]
		}
		message := Mail139ListMessage{MID: raw[match[2]:match[3]]}
		if subject := mail139SubjectPattern.FindStringSubmatch(raw[match[1]:end]); len(subject) == 2 {
			message.Subject = strings.TrimRight(strings.TrimSpace(subject[1]), " },")
		}
		messages = append(messages, message)
	}
	return messages
}

// Find139BillMail 在邮件列表中查找账单/话费邮件。
func Find139BillMail(messages []Mail139ListMessage) *Mail139ListMessage {
	for i := range messages {
		if strings.Contains(messages[i].Subject, "账单") || strings.Contains(messages[i].Subject, "话费") {
			return &messages[i]
		}
	}
	return nil
}
