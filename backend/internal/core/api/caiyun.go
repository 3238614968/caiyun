package api

import (
	"fmt"
	"strings"

	"caiyun/internal/core/http"
)

const (
	MarketURL            = "https://caiyun.feixin.10086.cn/market"
	Market7071URL        = "https://caiyun.feixin.10086.cn:7071/market"
	MobileMarketURL      = "https://m.mcloud.139.com/ycloud"
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
