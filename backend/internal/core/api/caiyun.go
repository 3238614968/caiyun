package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"time"

	"caiyun/internal/core/http"
)

const (
	BaseURL         = "https://caiyun.feixin.10086.cn"
	MarketURL       = "https://caiyun.feixin.10086.cn/market"
	MrpMarketURL    = "https://mrp.mcloud.139.com/market"
	MobileMarketURL = "https://m.mcloud.139.com/market"
	PortalURL       = "https://caiyun.feixin.10086.cn/portal"
	NoteURL         = "https://note.mcloud.139.com"
	PersonalURL     = "https://personal-kd-njs.yun.139.com"
	YunURL          = "https://yun.139.com"
	AICloudURL      = "https://yun.139.com/mrpInfo/ycloud/aixt"
	CloudPhoneURL   = "https://cpactiv.buy.139.com/cloudphone-market"
)

// CaiyunAPI 彩云 API 客户端
type CaiyunAPI struct {
	client *http.Client
}

// NewCaiyunAPI 创建彩云 API 客户端
func NewCaiyunAPI(client *http.Client) *CaiyunAPI {
	return &CaiyunAPI{client: client}
}

// CaiyunResponse 彩云 API 通用响应
type CaiyunResponse struct {
	Code    interface{} `json:"code"` // 可能是 int 或 string
	Message string      `json:"message"`
	Msg     string      `json:"msg"`
	Success bool        `json:"success"`
	Result  interface{} `json:"result"`
	Data    interface{} `json:"data"`
}

// SignInResponse 签到响应
type SignInResult struct {
	TodaySignIn              bool `json:"todaySignIn"`
	SignInPoints             int  `json:"signInPoints"`
	Total                    int  `json:"total"`
	ToReceive                int  `json:"toReceive"`
	CurMonthBackup           bool `json:"curMonthBackup"`
	CurMonthBackupTaskAccept bool `json:"curMonthBackupTaskAccept"`
	CurMonthBackupSignAccept bool `json:"curMonthBackupSignAccept"`
	NextMonthGet             int  `json:"nextMonthGet"`
}

type SignInResponse struct {
	Code    int          `json:"code"`
	Message string       `json:"message"`
	Msg     string       `json:"msg"`
	Success bool         `json:"success"`
	Result  SignInResult `json:"result"`
}

func normalizeMessageText(msg, message string) string {
	if strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	return strings.TrimSpace(message)
}

func isSuccessCode(code interface{}) bool {
	switch v := code.(type) {
	case nil:
		return false
	case int:
		return v == 0
	case int64:
		return v == 0
	case float64:
		return int(v) == 0
	case string:
		return v == "" || v == "0"
	default:
		return false
	}
}

func (r *CaiyunResponse) MessageText() string {
	if r == nil {
		return ""
	}
	return normalizeMessageText(r.Msg, r.Message)
}

func (r *CaiyunResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	msg := r.MessageText()
	return isSuccessCode(r.Code) || r.Success || strings.EqualFold(msg, "success")
}

func (r *SignInResponse) MessageText() string {
	if r == nil {
		return ""
	}
	return normalizeMessageText(r.Msg, r.Message)
}

func (r *SignInResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	msg := r.MessageText()
	return r.Code == 0 || r.Success || strings.EqualFold(msg, "success")
}

// CloudInfoResponse 云朵信息响应（与 SignInResponse 相同）
type CloudInfoResponse = SignInResponse

type PrizeLogPageResponse struct {
	Code    int                `json:"code"`
	Message string             `json:"message"`
	Msg     string             `json:"msg"`
	Success bool               `json:"success"`
	Result  PrizeLogPageResult `json:"result"`
}

type PrizeLogPageResult struct {
	Result  []PrizeLog `json:"result"`
	Records []PrizeLog `json:"records"`
}

type PrizeLog struct {
	PrizeName string `json:"prizeName"`
	Flag      int    `json:"flag"`
}

func (r *PrizeLogPageResponse) MessageText() string {
	if r == nil {
		return ""
	}
	return normalizeMessageText(r.Msg, r.Message)
}

func (r *PrizeLogPageResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	msg := r.MessageText()
	return r.Code == 0 || r.Success || strings.EqualFold(msg, "success")
}

func (r *PrizeLogPageResponse) PrizeLogs() []PrizeLog {
	if r == nil {
		return nil
	}
	if len(r.Result.Result) > 0 {
		return r.Result.Result
	}
	return r.Result.Records
}

func (r *PrizeLogPageResponse) PendingPrizeNames() []string {
	items := r.PrizeLogs()
	pending := make([]string, 0, len(items))
	for _, item := range items {
		if item.Flag == 1 && strings.TrimSpace(item.PrizeName) != "" {
			pending = append(pending, strings.TrimSpace(item.PrizeName))
		}
	}
	return pending
}

// TaskListResponse 任务列表响应
type TaskListResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Result  map[string][]Task `json:"result"`
}

// Task 任务信息
type Task struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Status      int    `json:"status"` // 0-未完成 1-已完成 2-已领取
	Reward      int    `json:"reward"`
	Group       string `json:"group"` // day, month, new, time, hidden
	// 兼容不同 marketName 的任务列表字段（原版 mjs 中使用这些字段）
	State      string `json:"state"`      // FINISH 等
	Enable     int    `json:"enable"`     // 1-启用
	GroupID    string `json:"groupid"`    // day, month, cloudEmail 等
	MarketName string `json:"marketname"` // sign_in_3, newsign_139mail 等
	CurrStep   int    `json:"currstep"`   // 某些任务的阶段进度
	Process    int    `json:"process"`    // 某些任务的累计进度
}

// ShakeResponse 摇一摇响应
type ShakeResponse struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Result  struct {
		PrizeName string `json:"prizeName"`
		Explain   string `json:"explain"`
		Img       string `json:"img"`
	} `json:"result"`
}

// SignIn 网盘签到（获取签到信息）
func (api *CaiyunAPI) SignIn() (*SignInResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/startSignIn?client=app", MobileMarketURL),
		map[string]string{"origin": "https://m.mcloud.139.com"},
	)
	if err != nil {
		return nil, fmt.Errorf("request sign in failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read sign in response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sign in http status=%d, body=%s", resp.StatusCode, body)
	}

	var result SignInResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode sign in response failed: %w, body=%s", err, body)
	}

	return &result, nil
}

func (api *CaiyunAPI) GetCloudInfo() (*CloudInfoResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/infoV3?client=app", MobileMarketURL),
		map[string]string{"origin": "https://m.mcloud.139.com"},
	)
	if err != nil {
		return nil, fmt.Errorf("request cloud info failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read cloud info response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cloud info http status=%d, body=%s", resp.StatusCode, body)
	}

	var result CloudInfoResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode cloud info response failed: %w, body=%s", err, body)
	}

	return &result, nil
}

func (api *CaiyunAPI) GetPrizeLogPage(pageNumber, pageSize int) (*PrizeLogPageResponse, error) {
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = 15
	}

	resp, err := api.client.Get(
		fmt.Sprintf("https://m.mcloud.139.com/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2?currPage=%d&pageSize=%d", pageNumber, pageSize),
		map[string]string{"origin": "https://m.mcloud.139.com"},
	)
	if err != nil {
		return nil, fmt.Errorf("request prize log failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read prize log response failed: %w", err)
	}

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("prize log http status=%d, body=%s", resp.StatusCode, body)
	}

	var result PrizeLogPageResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode prize log response failed: %w, body=%s", err, body)
	}

	return &result, nil
}

// GetTaskList 获取任务列表
func (api *CaiyunAPI) GetTaskList(marketName string) (*TaskListResponse, error) {
	if marketName == "" {
		marketName = "sign_in_3"
	}

	urlStr := fmt.Sprintf("%s/signin/task/taskList?marketname=%s&clientVersion=",
		MrpMarketURL, url.QueryEscape(marketName))

	resp, err := api.client.Get(urlStr, nil)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result TaskListResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// DoTask 执行任务
func (api *CaiyunAPI) DoTask(key, taskID string) error {
	urlStr := fmt.Sprintf("%s/signin/task/click?key=%s&id=%s",
		MrpMarketURL, url.QueryEscape(key), url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, nil)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return err
	}

	if !result.IsSuccess() {
		return fmt.Errorf("执行任务失败：%v - %s", result.Code, result.MessageText())
	}

	return nil
}

// ReceiveTaskReward 领取任务奖励
func (api *CaiyunAPI) ReceiveTaskReward(taskID string) error {
	urlStr := fmt.Sprintf("%s/signin/page/receiveTask?taskId=%s",
		MarketURL, url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, nil)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return err
	}

	if !result.IsSuccess() {
		return fmt.Errorf("领取奖励失败：%v - %s", result.Code, result.MessageText())
	}

	return nil
}

// Shake 摇一摇
func (api *CaiyunAPI) Shake() (*ShakeResponse, error) {
	resp, err := api.client.Post(
		fmt.Sprintf("%s/shake-server/shake/shakeIt?flag=1", MarketURL),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result ShakeResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// SignInWx 微信签到
func (api *CaiyunAPI) SignInWx() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/followSignInfo?isWx=true", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetDrawInWx 获取微信抽奖信息
func (api *CaiyunAPI) GetDrawInWx() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/drawInfo", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// WxDraw 微信抽奖
func (api *CaiyunAPI) WxDraw() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/draw", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetMsgPushStatus 获取消息推送状态
func (api *CaiyunAPI) GetMsgPushStatus() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/msgPushOn/task/status", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ObtainMsgPushOn 领取消息推送奖励
func (api *CaiyunAPI) ObtainMsgPushOn() (*CaiyunResponse, error) {
	resp, err := api.client.Post(
		fmt.Sprintf("%s/msgPushOn/task/obtain", MarketURL),
		nil,
		map[string]interface{}{"type": 2},
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetBackupGift 获取备份好礼状态
func (api *CaiyunAPI) GetBackupGift() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/backupgift/info", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ReceiveBackupGift 领取备份好礼
func (api *CaiyunAPI) ReceiveBackupGift() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/backupgift/receive", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetCloudRecord 获取云朵记录
// ReceiveRevivalReward claims the monthly revival reward.
func (api *CaiyunAPI) ReceiveRevivalReward() (*CaiyunResponse, error) {
	resp, err := api.client.Post(
		fmt.Sprintf("%s/signin/page/receiveRevivalReward", MobileMarketURL),
		map[string]string{"showloading": "true"},
		map[string]interface{}{},
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (api *CaiyunAPI) GetCloudRecord(pageNumber, pageSize, recordType int) (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/public/cloudRecord?type=%d&pageNumber=%d&pageSize=%d",
			MrpMarketURL, recordType, pageNumber, pageSize),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// Receive 领取云朵摘要（不触发签到，仅查询当前云朵与待领奖品）
func (api *CaiyunAPI) Receive() (*CaiyunResponse, error) {
	cloudInfo, cloudErr := api.GetCloudInfo()
	prizeResp, prizeErr := api.GetPrizeLogPage(1, 15)

	result := map[string]interface{}{}
	messageParts := make([]string, 0, 2)

	if cloudInfo != nil {
		if cloudInfo.IsSuccess() {
			result["todaySignIn"] = cloudInfo.Result.TodaySignIn
			result["total"] = cloudInfo.Result.Total
			result["toReceive"] = cloudInfo.Result.ToReceive
			result["nextMonthGet"] = cloudInfo.Result.NextMonthGet
			messageParts = append(messageParts, fmt.Sprintf("当前云朵%d", cloudInfo.Result.Total))
		} else if msg := cloudInfo.MessageText(); msg != "" {
			messageParts = append(messageParts, msg)
		}
	}

	if prizeResp != nil {
		if prizeResp.IsSuccess() {
			pendingPrizeNames := prizeResp.PendingPrizeNames()
			result["pendingPrizeNames"] = pendingPrizeNames
			result["pendingPrizeCount"] = len(pendingPrizeNames)
			if len(pendingPrizeNames) > 0 {
				messageParts = append(messageParts, fmt.Sprintf("待领奖品%d项", len(pendingPrizeNames)))
			}
		} else if msg := prizeResp.MessageText(); msg != "" {
			messageParts = append(messageParts, msg)
		}
	}

	if cloudErr != nil && prizeErr != nil {
		return nil, fmt.Errorf("receive summary failed: cloud=%w, prize=%v", cloudErr, prizeErr)
	}

	msg := "success"
	if len(messageParts) > 0 {
		msg = strings.Join(messageParts, "，")
	}

	return &CaiyunResponse{
		Code:    0,
		Msg:     msg,
		Success: true,
		Result:  result,
	}, nil
}

// ReceivePendingCloudRewards 领取待领取的云朵奖励
func (api *CaiyunAPI) ReceivePendingCloudRewards() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/receive", MarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// GetTaskExpansion 获取备份额外奖励信息
func (api *CaiyunAPI) GetTaskExpansion() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/taskExpansion", MrpMarketURL),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ReceiveTaskExpansion 领取翻倍奖励
func (api *CaiyunAPI) ReceiveTaskExpansion(acceptDate string) (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/receiveTaskExpansion?acceptDate=%s",
			MrpMarketURL, url.QueryEscape(acceptDate)),
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}

	return &result, nil
}

// NoteAuthResult 云笔记鉴权结果
type NoteAuthResult struct {
	Body    string
	Headers map[string]string
}

// GetNoteAuthToken 获取云笔记鉴权头（参考 mjs getNoteAuthToken）
func (api *CaiyunAPI) GetNoteAuthToken(authToken, phone string) (*NoteAuthResult, error) {
	if authToken == "" || phone == "" {
		return nil, fmt.Errorf("云笔记鉴权参数不完整")
	}

	headers := map[string]string{
		"APP_CP":              "android",
		"APP_NUMBER":          phone,
		"CP_VERSION":          "3.2.0",
		"x-huawei-channelsrc": "10001400",
		"User-Agent":          "mobile",
		"Content-Type":        "application/json",
	}

	bodyReq := map[string]interface{}{
		"authToken": authToken,
		"userPhone": phone,
	}

	resp, err := api.client.Post("https://note.mcloud.139.com/noteServer/api/authTokenRefresh.do", headers, bodyReq)
	if err != nil {
		return nil, err
	}

	appAuth := resp.Header.Get("app_auth")
	if appAuth == "" {
		appAuth = resp.Header.Get("App_Auth")
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	resultHeaders := map[string]string{}
	if appAuth != "" {
		resultHeaders["app_auth"] = appAuth
	}

	return &NoteAuthResult{
		Body:    body,
		Headers: resultHeaders,
	}, nil
}

// CreateNote 创建云笔记（参考 mjs createNote）
func (api *CaiyunAPI) CreateNote(noteID, title, phone string, extraHeaders map[string]string, tags []string) error {
	if noteID == "" || phone == "" {
		return fmt.Errorf("创建云笔记参数不完整")
	}

	nowMs := fmt.Sprintf("%d", time.Now().UnixMilli())
	headers := map[string]string{
		"APP_CP":       "pc",
		"APP_NUMBER":   phone,
		"CP_VERSION":   "7.7.1.20240115",
		"Content-Type": "application/json",
	}
	for k, v := range extraHeaders {
		if v != "" {
			headers[k] = v
		}
	}

	reqBody := map[string]interface{}{
		"archived":        0,
		"attachmentdir":   "",
		"attachmentdirid": "",
		"attachments":     []interface{}{},
		"contentid":       "",
		"contents": []map[string]interface{}{
			{"data": "<span></span>", "noteId": noteID, "sortOrder": 0, "type": "TEXT"},
		},
		"cp":          "",
		"createtime":  nowMs,
		"description": "",
		"expands":     map[string]interface{}{"noteType": 0},
		"landMark":    []interface{}{},
		"latlng":      "",
		"location":    "",
		"noteid":      noteID,
		"remindtime":  "",
		"remindtype":  0,
		"revision":    "1",
		"system":      "",
		"tags":        tags,
		"title":       title,
		"topmost":     "0",
		"updatetime":  nowMs,
		"userphone":   phone,
		"version":     "",
		"visitTime":   nowMs,
	}

	resp, err := api.client.Post("https://mnote.caiyun.feixin.10086.cn/noteServer/api/createNote.do", headers, reqBody)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	// 返回体格式不稳定，优先校验 code/msg 字段，缺失时只要 HTTP 成功即视为成功。
	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err == nil {
		if code, ok := result["code"]; ok && fmt.Sprint(code) != "0" && fmt.Sprint(code) != "" {
			msg := fmt.Sprint(result["msg"])
			if msg == "" {
				msg = fmt.Sprint(result["message"])
			}
			return fmt.Errorf("创建云笔记失败: code=%v, msg=%s", code, msg)
		}
	}

	return nil
}

// DeleteNote 删除云笔记（移动到回收站）
func (api *CaiyunAPI) DeleteNote(noteID string, extraHeaders map[string]string) error {
	if noteID == "" {
		return fmt.Errorf("noteID 为空")
	}

	headers := map[string]string{
		"APP_CP":       "pc",
		"CP_VERSION":   "7.7.1.20240115",
		"Content-Type": "application/json",
	}
	for k, v := range extraHeaders {
		if v != "" {
			headers[k] = v
		}
	}

	reqBody := map[string]interface{}{
		"noteids": []map[string]string{{"noteid": noteID}},
	}

	resp, err := api.client.Post("https://mnote.caiyun.feixin.10086.cn/noteServer/api/moveToRecycleBin.do", headers, reqBody)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(body), &result); err == nil {
		if code, ok := result["code"]; ok {
			codeStr := fmt.Sprint(code)
			if codeStr != "" && codeStr != "0" {
				msg := fmt.Sprint(result["msg"])
				if msg == "" {
					msg = fmt.Sprint(result["message"])
				}
				return fmt.Errorf("删除云笔记失败: code=%v, msg=%s", code, msg)
			}
		}
	}

	return nil
}

// OutLinkResponse 分享文件响应
type OutLinkResponse struct {
	Success bool        `json:"success"`
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Data    struct {
		GetOutLinkRes struct {
			GetOutLinkResSet []struct {
				LinkID string `json:"linkID"`
			} `json:"getOutLinkResSet"`
		} `json:"getOutLinkRes"`
	} `json:"data"`
}

// GetOutLink 创建分享链接（参考 mjs getOutLink）
func (api *CaiyunAPI) GetOutLink(phone string, fileIDs []string, dedicatedName string) (*OutLinkResponse, error) {
	headers := map[string]string{
		"caller":               "web",
		"cms-device":           "default",
		"mcloud-channel":       "1000101",
		"mcloud-version":       "7.14.4",
		"x-deviceinfo":         "||9|7.14.4|edge||||linux unknow||zh-CN|||",
		"x-huawei-channelsrc":  "10000034",
		"x-svctype":            "1",
		"x-yun-api-version":    "v1",
		"x-yun-app-channel":    "10000034",
		"x-yun-channel-source": "10000034",
		"x-yun-client-info":    "||9|7.14.4|edge||||linux unknow||zh-CN|||||",
		"x-yun-module-type":    "100",
		"x-yun-svc-type":       "1",
		"Content-Type":         "application/json",
	}

	bodyReq := map[string]interface{}{
		"getOutLinkReq": map[string]interface{}{
			"subLinkType":   0,
			"encrypt":       1,
			"coIDLst":       fileIDs,
			"caIDLst":       []interface{}{},
			"pubType":       1,
			"dedicatedName": dedicatedName,
			"period":        1,
			"periodUnit":    1,
			"viewerLst":     []interface{}{},
			"extInfo":       map[string]interface{}{"isWatermark": 0, "shareChannel": "3001"},
			"commonAccountInfo": map[string]interface{}{
				"account":     phone,
				"accountType": 1,
			},
		},
	}

	resp, err := api.client.Post("https://yun.139.com/orchestration/personalCloud-rebuild/outlink/v1.0/getOutLink", headers, bodyReq)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result OutLinkResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DelOutLink 删除分享链接（参考 mjs delOutLink）
func (api *CaiyunAPI) DelOutLink(phone string, linkIDs []string) (*CaiyunResponse, error) {
	bodyReq := map[string]interface{}{
		"delOutLinkReq": map[string]interface{}{
			"linkIDs": linkIDs,
			"commonAccountInfo": map[string]interface{}{
				"account":     phone,
				"accountType": 1,
			},
		},
	}

	resp, err := api.client.Post("https://yun.139.com/orchestration/personalCloud-rebuild/outlink/v1.0/delOutLink", nil, bodyReq)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}
