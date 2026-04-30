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
	MarketURL            = "https://caiyun.feixin.10086.cn/market"
	MobileMarketURL      = "https://m.mcloud.139.com/market"
	AIYunURL             = "https://ai.yun.139.com"
	MarketClientVersion  = "12.5.4"
	MarketSourceID       = "1097"
	MarketUserAgent      = "Mozilla/5.0 (Linux; Android 10; MI 8 Build/QKQ1.190828.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/143.0.7499.146 Mobile Safari/537.36 MCloudApp/12.5.4 AppLanguage/zh-CN"
	ShareUserAgent       = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/143.0.0.0 Safari/537.36"
	AICameraSampleBase64 = "data:image/jpg;base64,/9j/4AAQSkZJRgABAQAAAQABAAD/2wCEAAkGBxAQEBUQEBAVFRUVFRUVFRUVFRUVFRUVFRUWFhUVFRUYHSggGBolHRUVITEhJSkrLi4uFx8zODMsNygtLisBCgoKDg0OGhAQGi0lHyUtLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLS0tLf/AABEIAAEAAQMBIgACEQEDEQH/xAAXAAEBAQEAAAAAAAAAAAAAAAAAAQID/8QAFhEBAQEAAAAAAAAAAAAAAAAAABEh/9oADAMBAAIQAxAAAAHhAH//xAAZEAEBAQEBAQAAAAAAAAAAAAABEQIhMUH/2gAIAQEAAT8A2M4Kxqf/xAAWEQEBAQAAAAAAAAAAAAAAAAAAESH/2gAIAQIBAT8Ap//EABYRAQEBAAAAAAAAAAAAAAAAAAABEf/aAAgBAwEBPwCf/9k="
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
	Code    interface{} `json:"code"`
	Message string      `json:"message"`
	Msg     string      `json:"msg"`
	Success bool        `json:"success"`
	Result  interface{} `json:"result"`
	Data    interface{} `json:"data"`
}

type SignInCalendar struct {
	Today  bool        `json:"t"`
	Signed interface{} `json:"s"`
}

// SignInResult 签到结果
type SignInResult struct {
	TodaySignIn              bool             `json:"todaySignIn"`
	SignInPoints             int              `json:"signInPoints"`
	Total                    int              `json:"total"`
	ToReceive                int              `json:"toReceive"`
	CurMonthBackup           bool             `json:"curMonthBackup"`
	CurMonthBackupTaskAccept bool             `json:"curMonthBackupTaskAccept"`
	CurMonthBackupSignAccept bool             `json:"curMonthBackupSignAccept"`
	NextMonthGet             int              `json:"nextMonthGet"`
	Cal                      []SignInCalendar `json:"cal"`
}

func (r SignInResult) TodaySigned() bool {
	if r.TodaySignIn {
		return true
	}
	for _, day := range r.Cal {
		if day.Today && boolFromAny(day.Signed) {
			return true
		}
	}
	return false
}

// SignInResponse 签到响应
type SignInResponse struct {
	Code    *int         `json:"code"`
	Message string       `json:"message"`
	Msg     string       `json:"msg"`
	Success bool         `json:"success"`
	Result  SignInResult `json:"result"`
}

// CloudInfoResponse 云朵信息响应
type CloudInfoResponse = SignInResponse

// PrizeLogPageResponse 奖品记录响应
type PrizeLogPageResponse struct {
	Code    *int               `json:"code"`
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

// TaskListResponse 任务列表响应
type TaskListResponse struct {
	Code    int               `json:"code"`
	Message string            `json:"message"`
	Msg     string            `json:"msg"`
	Success bool              `json:"success"`
	Result  map[string][]Task `json:"result"`
}

// Task 任务信息
type Task struct {
	ID          int      `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Status      int      `json:"status"`
	Reward      int      `json:"reward"`
	Group       string   `json:"group"`
	StepTypeSet []string `json:"stepTypeSet"`
	State       string   `json:"state"`
	Enable      int      `json:"enable"`
	GroupID     string   `json:"groupid"`
	MarketName  string   `json:"marketname"`
	CurrStep    int      `json:"currstep"`
	Process     int      `json:"process"`
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

func normalizeMessageText(msg, message string) string {
	if strings.TrimSpace(msg) != "" {
		return strings.TrimSpace(msg)
	}
	return strings.TrimSpace(message)
}

func boolFromAny(v interface{}) bool {
	switch value := v.(type) {
	case bool:
		return value
	case int:
		return value != 0
	case int64:
		return value != 0
	case float64:
		return int(value) != 0
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		return value == "1" || value == "true" || value == "yes"
	default:
		return false
	}
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

func intCodeText(code *int) string {
	if code == nil {
		return "missing"
	}
	return fmt.Sprintf("%d", *code)
}

func isSuccessIntCode(code *int) bool {
	return code != nil && *code == 0
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

func (r *SignInResponse) CodeText() string {
	if r == nil {
		return "missing"
	}
	return intCodeText(r.Code)
}

func (r *SignInResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	msg := r.MessageText()
	return isSuccessIntCode(r.Code) || r.Success || strings.EqualFold(msg, "success")
}

func (r *PrizeLogPageResponse) MessageText() string {
	if r == nil {
		return ""
	}
	return normalizeMessageText(r.Msg, r.Message)
}

func (r *PrizeLogPageResponse) CodeText() string {
	if r == nil {
		return "missing"
	}
	return intCodeText(r.Code)
}

func (r *PrizeLogPageResponse) IsSuccess() bool {
	if r == nil {
		return false
	}
	msg := r.MessageText()
	return isSuccessIntCode(r.Code) || r.Success || strings.EqualFold(msg, "success")
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

func (r *TaskListResponse) MessageText() string {
	if r == nil {
		return ""
	}
	return normalizeMessageText(r.Msg, r.Message)
}

func (api *CaiyunAPI) buildMarketPageURL(sourceID string) string {
	currentSourceID := strings.TrimSpace(sourceID)
	if currentSourceID == "" {
		currentSourceID = MarketSourceID
	}
	return fmt.Sprintf(
		"https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin&sourceid=%s&enableShare=1&token=%s&targetSourceId=001005",
		url.QueryEscape(currentSourceID),
		url.QueryEscape(strings.TrimSpace(api.client.GetSSOToken())),
	)
}

func (api *CaiyunAPI) buildMarketHeaders(extraHeaders map[string]string, referer string) map[string]string {
	headers := map[string]string{
		"User-Agent":       MarketUserAgent,
		"Accept":           "*/*",
		"Origin":           "https://m.mcloud.139.com",
		"X-Requested-With": "com.chinamobile.mcloud",
	}
	if token := strings.TrimSpace(api.client.GetJWTToken()); token != "" {
		headers["jwtToken"] = token
		headers["jwttoken"] = token
	}
	if referer == "" {
		referer = api.buildMarketPageURL("")
	}
	headers["Referer"] = referer
	for key, value := range extraHeaders {
		if strings.TrimSpace(value) != "" {
			headers[key] = value
		}
	}
	return headers
}

func (api *CaiyunAPI) buildReceiveHeaders(sourceID string) map[string]string {
	headers := api.buildMarketHeaders(map[string]string{
		"showLoading": "true",
		"appVersion":  MarketClientVersion + ".0",
		"activityId":  "sign_in_3",
	}, api.buildMarketPageURL(sourceID))
	if deviceID := strings.TrimSpace(api.client.GetDeviceID()); deviceID != "" {
		headers["deviceId"] = deviceID
	}
	return headers
}

func (api *CaiyunAPI) prepareSignInCenterSession(forReceive bool) {
	pageURL := api.buildMarketPageURL("")
	if resp, err := api.client.Get(pageURL, api.buildMarketHeaders(nil, pageURL)); err == nil && resp != nil {
		_, _ = api.client.ReadResponseBody(resp)
	}

	keywords := []string{
		"newsignin_index_pv",
		"newsignin_index_client",
		"newsignin_index_app_client",
		"newsignin_index_cookie_login",
		"newsignin_index_cookie",
		"newsignin_index_app_cookie_login",
	}
	for _, keyword := range keywords {
		api.postSignInJournaling(keyword)
	}
	if forReceive {
		api.postSignInJournaling("newsignin_index_receive_type")
	}
}

func (api *CaiyunAPI) postSignInJournaling(keyword string) {
	payload := fmt.Sprintf("module=uservisit&optkeyword=%s&sourceid=%s&marketName=sign_in_3",
		url.QueryEscape(keyword), url.QueryEscape(MarketSourceID))
	headers := api.buildMarketHeaders(map[string]string{
		"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8",
	}, api.buildMarketPageURL(""))
	if resp, err := api.client.Post("https://m.mcloud.139.com/ycloud/visitlog/journaling", headers, payload); err == nil && resp != nil {
		_, _ = api.client.ReadResponseBody(resp)
	}
}

// SignIn 网盘签到
func (api *CaiyunAPI) SignIn() (*SignInResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/startSignIn?client=app", MobileMarketURL),
		api.buildReceiveHeaders(""),
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

// GetCloudInfo 获取签到信息
func (api *CaiyunAPI) GetCloudInfo() (*CloudInfoResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/infoV3?client=app", MobileMarketURL),
		api.buildReceiveHeaders(""),
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

// GetPrizeLogPage 获取奖品记录
func (api *CaiyunAPI) GetPrizeLogPage(pageNumber, pageSize int) (*PrizeLogPageResponse, error) {
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = 15
	}

	api.prepareSignInCenterSession(true)

	resp, err := api.client.Get(
		fmt.Sprintf("https://m.mcloud.139.com/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2?currPage=%d&pageSize=%d", pageNumber, pageSize),
		api.buildReceiveHeaders(""),
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

// GetTaskList 获取旧版任务列表
func (api *CaiyunAPI) GetTaskList(marketName string) (*TaskListResponse, error) {
	if marketName == "" {
		marketName = "sign_in_3"
	}

	urlStr := fmt.Sprintf("%s/signin/task/taskList?marketname=%s&clientVersion=%s",
		MarketURL, url.QueryEscape(marketName), url.QueryEscape(MarketClientVersion))

	resp, err := api.client.Get(urlStr, api.buildMarketHeaders(nil, ""))
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

// GetTaskListV2 获取新版任务列表
func (api *CaiyunAPI) GetTaskListV2(group string) (*TaskListResponse, error) {
	api.prepareSignInCenterSession(false)

	urlStr := fmt.Sprintf("%s/signin/task/taskListV2?marketname=sign_in_3&clientVersion=%s&group=%s",
		MobileMarketURL, url.QueryEscape(MarketClientVersion), url.QueryEscape(group))

	resp, err := api.client.Get(urlStr, api.buildReceiveHeaders(""))
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

// DoTaskWithMarket 执行任务
func (api *CaiyunAPI) DoTaskWithMarket(marketName, key, taskID string) error {
	baseURL := MarketURL
	headers := api.buildMarketHeaders(nil, "")
	if strings.TrimSpace(marketName) == "sign_in_3" {
		baseURL = MobileMarketURL
		headers = api.buildReceiveHeaders("")
	}

	urlStr := fmt.Sprintf("%s/signin/task/click?key=%s&id=%s",
		baseURL, url.QueryEscape(key), url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, headers)
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

// DoTask 执行默认云盘任务
func (api *CaiyunAPI) DoTask(key, taskID string) error {
	return api.DoTaskWithMarket("sign_in_3", key, taskID)
}

// ReceiveTaskRewardForMarket 领取任务奖励
func (api *CaiyunAPI) ReceiveTaskRewardForMarket(marketName, taskID string) error {
	baseURL := MarketURL
	headers := api.buildMarketHeaders(nil, "")
	if strings.TrimSpace(marketName) == "sign_in_3" {
		baseURL = MobileMarketURL
		headers = api.buildReceiveHeaders("")
	}

	urlStr := fmt.Sprintf("%s/signin/page/receiveTask?taskId=%s",
		baseURL, url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, headers)
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

// ReceiveTaskReward 领取默认云盘任务奖励
func (api *CaiyunAPI) ReceiveTaskReward(taskID string) error {
	return api.ReceiveTaskRewardForMarket("sign_in_3", taskID)
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

// ReceiveRevivalReward 领取复活卡奖励
func (api *CaiyunAPI) ReceiveRevivalReward() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)

	resp, err := api.client.Post(
		fmt.Sprintf("%s/signin/page/receiveRevivalReward", MobileMarketURL),
		api.buildReceiveHeaders(""),
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

// GetCloudRecord 获取云朵记录
func (api *CaiyunAPI) GetCloudRecord(pageNumber, pageSize, recordType int) (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/public/cloudRecord?type=%d&pageNumber=%d&pageSize=%d",
			MarketURL, recordType, pageNumber, pageSize),
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

// GetReceiveSummary 获取领取云朵后的摘要信息
func (api *CaiyunAPI) GetReceiveSummary() (*CaiyunResponse, error) {
	cloudInfo, err := api.GetCloudInfo()
	if err != nil {
		return nil, fmt.Errorf("get cloud info failed: %w", err)
	}
	if cloudInfo == nil {
		return nil, fmt.Errorf("get cloud info failed: empty response")
	}
	if !cloudInfo.IsSuccess() {
		return &CaiyunResponse{Code: -1, Msg: cloudInfo.MessageText(), Message: cloudInfo.MessageText()}, nil
	}

	prizeResp, err := api.GetPrizeLogPage(1, 15)
	if err != nil {
		return nil, fmt.Errorf("get prize log failed: %w", err)
	}
	if prizeResp == nil {
		return nil, fmt.Errorf("get prize log failed: empty response")
	}
	if !prizeResp.IsSuccess() {
		return &CaiyunResponse{Code: -1, Msg: prizeResp.MessageText(), Message: prizeResp.MessageText()}, nil
	}

	pendingPrizeNames := prizeResp.PendingPrizeNames()
	result := map[string]interface{}{
		"todaySignIn":       cloudInfo.Result.TodaySigned(),
		"total":             cloudInfo.Result.Total,
		"toReceive":         cloudInfo.Result.ToReceive,
		"nextMonthGet":      cloudInfo.Result.NextMonthGet,
		"pendingPrizeNames": pendingPrizeNames,
		"pendingPrizeCount": len(pendingPrizeNames),
	}

	messageParts := []string{fmt.Sprintf("当前云朵%d", cloudInfo.Result.Total)}
	if len(pendingPrizeNames) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("待领奖品%d项", len(pendingPrizeNames)))
	}

	return &CaiyunResponse{
		Code:    0,
		Msg:     strings.Join(messageParts, "，"),
		Success: true,
		Result:  result,
	}, nil
}

// ReceivePendingCloudRewards 领取待领取云朵
func (api *CaiyunAPI) ReceivePendingCloudRewards() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/receive", MarketURL),
		api.buildReceiveHeaders(""),
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

// Receive 领取云朵
func (api *CaiyunAPI) Receive() (*CaiyunResponse, error) {
	claimResp, err := api.ReceivePendingCloudRewards()
	if err != nil {
		return nil, err
	}
	if claimResp == nil {
		return nil, fmt.Errorf("receive pending rewards failed: empty response")
	}
	if !claimResp.IsSuccess() {
		return claimResp, nil
	}

	summaryResp, err := api.GetReceiveSummary()
	if err != nil {
		return nil, err
	}
	if summaryResp == nil {
		return nil, fmt.Errorf("get receive summary failed: empty response")
	}
	if !summaryResp.IsSuccess() {
		return summaryResp, nil
	}

	if payload, ok := summaryResp.Result.(map[string]interface{}); ok {
		payload["receiveMessage"] = claimResp.MessageText()
	}
	if msg := summaryResp.MessageText(); msg != "" {
		summaryResp.Msg = msg
		if claimMsg := claimResp.MessageText(); claimMsg != "" && !strings.EqualFold(claimMsg, "success") {
			summaryResp.Msg = fmt.Sprintf("%s，%s", claimMsg, msg)
		}
	}
	return summaryResp, nil
}

// GetTaskExpansion 获取备份翻倍奖励信息
func (api *CaiyunAPI) GetTaskExpansion() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/taskExpansion", MarketURL),
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
			MarketURL, url.QueryEscape(acceptDate)),
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

// GetNoteAuthToken 获取云笔记鉴权头
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

// CreateNote 创建云笔记
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

// DeleteNote 删除云笔记
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
		Result struct {
			ResultCode string `json:"resultCode"`
			ResultDesc string `json:"resultDesc"`
		} `json:"result"`
		GetOutLinkRes struct {
			GetOutLinkResSet []struct {
				LinkID string `json:"linkID"`
			} `json:"getOutLinkResSet"`
		} `json:"getOutLinkRes"`
	} `json:"data"`
}

func (api *CaiyunAPI) buildShareHeaders() map[string]string {
	return map[string]string{
		"x-yun-api-version":    "v1",
		"x-yun-app-channel":    "10000023",
		"x-yun-client-info":    "||9|12.5.4|Chrome|143.0.7499.146|codextestshare||Windows 10||zh-CN|||Q2hyb21l||",
		"x-yun-module-type":    "100",
		"x-yun-svc-type":       "1",
		"x-SvcType":            "1",
		"x-yun-channel-source": "10000023",
		"x-huawei-channelSrc":  "10000023",
		"CMS-DEVICE":           "default",
		"User-Agent":           ShareUserAgent,
		"Referer":              "https://yun.139.com/shareweb/",
		"Origin":               "https://yun.139.com",
		"Content-Type":         "application/json;charset=UTF-8",
	}
}

// GetOutLink 创建分享链接
func (api *CaiyunAPI) GetOutLink(phone string, fileIDs []string, dedicatedName string) (*OutLinkResponse, error) {
	bodyReq := map[string]interface{}{
		"getOutLinkReq": map[string]interface{}{
			"subLinkType":   0,
			"encrypt":       0,
			"coIDLst":       fileIDs,
			"caIDLst":       []interface{}{},
			"pubType":       1,
			"dedicatedName": dedicatedName,
			"periodUnit":    1,
			"viewerLst":     []interface{}{},
			"extInfo":       map[string]interface{}{"isWatermark": 0, "shareChannel": "3001"},
			"commonAccountInfo": map[string]interface{}{
				"account":     phone,
				"accountType": 1,
			},
		},
	}

	resp, err := api.client.Post("https://yun.139.com/orchestration/personalCloud-rebuild/outlink/v1.0/getOutLink", api.buildShareHeaders(), bodyReq)
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

// DelOutLink 删除分享链接
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

	resp, err := api.client.Post("https://yun.139.com/orchestration/personalCloud-rebuild/outlink/v1.0/delOutLink", api.buildShareHeaders(), bodyReq)
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

func (api *CaiyunAPI) buildAIHeaders(useClientInfo bool) map[string]string {
	tid := fmt.Sprintf("%d", time.Now().UnixNano())
	headers := map[string]string{
		"Connection":         "keep-alive",
		"sec-ch-ua-platform": "\"Android\"",
		"x-yun-api-version":  "v1",
		"x-yun-tid":          tid,
		"sec-ch-ua":          "\"Android WebView\";v=\"143\", \"Chromium\";v=\"143\", \"Not A(Brand\";v=\"24\"",
		"sec-ch-ua-mobile":   "?1",
		"X-Requested-With":   "com.chinamobile.mcloud",
		"Origin":             "https://frontend.mcloud.139.com",
		"Referer":            "https://frontend.mcloud.139.com/",
		"User-Agent":         "Mozilla/5.0 (Linux; Android 10; MI 8 Build/QKQ1.190828.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/143.0.7499.146 Mobile Safari/537.36 MCloudApp/12.5.4 tid/" + tid,
		"Content-Type":       "application/json",
		"Sec-Fetch-Site":     "same-site",
		"Sec-Fetch-Mode":     "cors",
		"Sec-Fetch-Dest":     "empty",
		"Accept-Encoding":    "gzip, deflate, br, zstd",
		"Accept-Language":    "zh,zh-CN;q=0.9,en-US;q=0.8,en;q=0.7",
	}
	if useClientInfo {
		headers["Accept"] = "text/event-stream"
		headers["x-yun-client-info"] = "4||1|12.5.4||MI 8|" + tid + "||android 10|||||"
		headers["x-yun-app-channel"] = "101"
		return headers
	}
	headers["Accept"] = "*/*"
	headers["x-DeviceInfo"] = "||36|12.5.4||MI 8|" + tid + "||android 10|||||"
	return headers
}

func (api *CaiyunAPI) isAICameraChatSuccess(text string) bool {
	for _, line := range strings.Split(strings.TrimSpace(text), "\n") {
		payload := strings.TrimSpace(line)
		if strings.HasPrefix(payload, "data:") {
			payload = strings.TrimSpace(strings.TrimPrefix(payload, "data:"))
		}
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var data map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &data); err != nil {
			continue
		}
		if boolFromAny(data["success"]) {
			return true
		}
		if code, ok := data["code"].(string); ok && code == "0000" {
			return true
		}
	}
	return false
}

// CompleteAICameraTask 完成 AI 相机任务
func (api *CaiyunAPI) CompleteAICameraTask() error {
	userDomainID := strings.TrimSpace(api.client.GetUserDomainID())
	if userDomainID == "" {
		return fmt.Errorf("缺少 userDomainId")
	}

	recognizeBody := map[string]interface{}{
		"channelId":     "101",
		"userId":        userDomainID,
		"recognizeType": "1",
		"base64":        AICameraSampleBase64,
		"sendType":      "2",
		"imageExt":      "jpg",
		"uploadToCloud": true,
		"timeout":       30000,
	}
	recognizeResp, err := api.client.Post(AIYunURL+"/api/image/aiRecognize", api.buildAIHeaders(false), recognizeBody)
	if err != nil {
		return err
	}

	recognizeText, err := api.client.ReadResponseBody(recognizeResp)
	if err != nil {
		return err
	}

	var recognizeResult struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
		Data    struct {
			FileID string      `json:"fileId"`
			TaskID interface{} `json:"taskId"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(recognizeText), &recognizeResult); err != nil {
		return fmt.Errorf("解析 AI 相机识图响应失败: %w", err)
	}
	if !recognizeResult.Success {
		return fmt.Errorf("AI 相机识图失败: %s", normalizeMessageText("", recognizeResult.Message))
	}
	if recognizeResult.Data.FileID == "" {
		return fmt.Errorf("AI 相机识图失败: 缺少 fileId")
	}

	cst := time.FixedZone("CST", 8*3600)
	fileName := fmt.Sprintf("%d.jpeg", time.Now().UnixMilli())
	chatBody := map[string]interface{}{
		"userId":          userDomainID,
		"sessionId":       "",
		"applicationType": "chat",
		"applicationId":   "",
		"sourceChannel":   "101",
		"dialogueInput": map[string]interface{}{
			"dialogue":                        "？",
			"prompt":                          "",
			"inputTime":                       time.Now().In(cst).Format("2006-01-02T15:04:05.000-07:00"),
			"enableForceLlm":                  false,
			"enableForceNetworkSearch":        true,
			"enableModelThinking":             false,
			"enableAllNetworkSearch":          false,
			"enableKnowledgeAndNetworkSearch": false,
			"enableRegenerate":                false,
			"versionInfo":                     map[string]interface{}{"h5Version": "2.7.6"},
			"extInfo":                         "{}",
			"sortInfo":                        map[string]interface{}{},
			"toolSetting":                     map[string]interface{}{"imageToolSetting": map[string]interface{}{"enableLlmDescribe": true}},
			"attachment": map[string]interface{}{
				"attachmentTypeList": []int{3},
				"fileList": []map[string]interface{}{
					{"fileId": recognizeResult.Data.FileID, "name": fileName},
				},
			},
		},
	}

	chatResp, err := api.client.Post(AIYunURL+"/api/outer/assistant/chat/v2/add", api.buildAIHeaders(true), chatBody)
	if err != nil {
		return err
	}
	chatText, err := api.client.ReadResponseBody(chatResp)
	if err != nil {
		return err
	}
	if api.isAICameraChatSuccess(chatText) {
		return nil
	}

	var chatResult map[string]interface{}
	if err := json.Unmarshal([]byte(chatText), &chatResult); err == nil {
		if boolFromAny(chatResult["success"]) {
			return nil
		}
		if code, ok := chatResult["code"].(string); ok && code == "0000" {
			return nil
		}
		msg := normalizeMessageText(fmt.Sprint(chatResult["msg"]), fmt.Sprint(chatResult["message"]))
		if msg == "" {
			msg = "响应解析失败"
		}
		return fmt.Errorf("AI 相机对话失败: %s", msg)
	}

	return fmt.Errorf("AI 相机对话失败: 响应解析失败")
}
