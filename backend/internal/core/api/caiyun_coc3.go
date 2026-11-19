package api

import (
	"context"
	"encoding/json"
	"fmt"
	nethttp "net/http"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/auth"
	"caiyun/internal/core/coc3wasm"
	"caiyun/internal/core/http"
)

// Coc3API 中国移动权益市场（dev.coc.10086.cn/coc3）。
//
// 云盘签发目标票据后才能登录；云盘授权本身不是中国移动活动会话：
//
//	云盘 Authorization --querySpecToken(toSourceId=056002)--> 统一认证票（42字符，一次性）
//	--userLoginV2{ticket, ticketType:11}--> COC3_LOGIN_TOKEN（36字符）
//
// token 用法：Cookie COC3_LOGIN_TOKEN=<token> + 头 phone/token，调 /coc3/* 业务接口。
// 响应信封三态并存：登录族 code:"0"（字符串）、任务族 resultCode:0、抽奖族 code:200，
// 判成功必须逐端点对码。全部端点必须带 Accept: application/json（默认回 XML 包装）。
type Coc3API struct {
	client *http.Client
}

const (
	coc3BaseURL      = "https://dev.coc.10086.cn"
	coc3SourceID     = "056002"
	coc3LoginURL     = coc3BaseURL + "/coc3/gr/login/ssoRights/userLoginV2"
	coc3Referer      = coc3wasm.CanvasURL
	coc3AppPackage   = "com.greenpoint.android.mc10086.activity"
	coc3UserAgent    = coc3wasm.UserAgent
	coc3TaskBase     = coc3BaseURL + "/coc3/coc3-market-activity/cmpp-manage/task/"
	coc3LotteryBase  = coc3BaseURL + "/coc3/coc3-market-activity/cmpp-manage/lottery/"
	coc3BalanceURL   = coc3BaseURL + "/coc3/gr-integral-ability/api/external/score/digIntel/getBalance"
	coc3OrderBase    = coc3BaseURL + "/coc3/coc3-market-order/api/external/"
	coc3TokenHeader  = "COC3_LOGIN_TOKEN"
	coc3LoginTimeout = 30 * time.Second
)

// Coc3LoginResult 登录链产物（token 即业务 cookie 值）。
type Coc3LoginResult struct {
	Token        string
	RefreshToken string
	MobileMask   string
	ProvinceCode string
}

// Coc3Response 统一三种信封的最小解析：任务族 resultCode、其余 code。
type Coc3Response struct {
	Success         bool
	ResultCode      int    // 任务族信封
	Code            string // 登录/抽奖族信封（字符串形态）
	Message         string
	Data            json.RawMessage
	resultCode      string
	declaredSuccess json.RawMessage
}

type Coc3BusinessError struct {
	Operation string
	Code      string
	Message   string
}

func (e *Coc3BusinessError) Error() string {
	return fmt.Sprintf("%s失败: code=%s %s", e.Operation, e.Code, e.Message)
}

func (r *Coc3Response) BusinessError(operation string) error {
	if r.Success {
		return nil
	}
	code := r.Code
	if r.resultCode != "" && r.resultCode != "0" && r.resultCode != "200" {
		code = r.resultCode
	}
	return &Coc3BusinessError{Operation: operation, Code: code, Message: r.Message}
}

// Codes and counters are returned as both JSON numbers and strings. Never let
// an unrecognised value fall through to the integer zero (a success code).
func coc3Scalar(raw json.RawMessage) (string, error) {
	text := strings.TrimSpace(string(raw))
	if text == "" || text == "null" {
		return "", nil
	}
	if strings.HasPrefix(text, `"`) {
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", err
		}
		return strings.TrimSpace(value), nil
	}
	var value json.Number
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", err
	}
	return value.String(), nil
}

func coc3Integer(raw json.RawMessage, name string) (int, error) {
	value, err := coc3Scalar(raw)
	if err != nil || value == "" {
		return 0, fmt.Errorf("%s缺少有效整数", name)
	}
	n, err := strconv.Atoi(value)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s不是有效的非负整数", name)
	}
	return n, nil
}

func (api *Coc3API) decodeResponse(resp *nethttp.Response, operation string, successCodes ...string) (*Coc3Response, error) {
	text, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s HTTP %d", operation, resp.StatusCode)
	}
	var parsed struct {
		Code       json.RawMessage `json:"code"`
		ResultCode json.RawMessage `json:"resultCode"`
		Msg        string          `json:"msg"`
		Message    string          `json:"message"`
		Data       json.RawMessage `json:"data"`
		Success    json.RawMessage `json:"success"`
	}
	if err := json.Unmarshal([]byte(text), &parsed); err != nil {
		return nil, fmt.Errorf("解析%s响应失败: %w", operation, err)
	}
	code, err := coc3Scalar(parsed.Code)
	if err != nil {
		return nil, fmt.Errorf("%s返回无效 code", operation)
	}
	rc, err := coc3Scalar(parsed.ResultCode)
	if err != nil {
		return nil, fmt.Errorf("%s返回无效 resultCode", operation)
	}
	if code == "" && rc == "" {
		return nil, fmt.Errorf("%s响应缺少业务状态码", operation)
	}
	out := &Coc3Response{Code: code, resultCode: rc, Data: parsed.Data, declaredSuccess: parsed.Success,
		Message: normalizeMessageText(parsed.Msg, parsed.Message)}
	if rc != "" {
		out.ResultCode, _ = strconv.Atoi(rc)
	}
	if len(successCodes) == 0 {
		successCodes = []string{"0", "200"}
	}
	accepted := func(value string) bool {
		if value == "" {
			return true
		}
		for _, allowed := range successCodes {
			if value == allowed {
				return true
			}
		}
		return false
	}
	out.Success = accepted(code) && accepted(rc)
	return out, nil
}

func (api *Coc3API) decodeLoginResponse(resp *nethttp.Response, operation string) (json.RawMessage, error) {
	envelope, err := api.decodeResponse(resp, operation, "0")
	if err != nil {
		return nil, err
	}
	if envelope.Code == "" {
		return nil, fmt.Errorf("%s响应缺少登录状态码 code", operation)
	}
	if !envelope.Success {
		code := envelope.Code
		if code == "0" {
			code = envelope.resultCode
		}
		return nil, &Coc3BusinessError{Operation: operation, Code: code, Message: envelope.Message}
	}
	if len(envelope.declaredSuccess) != 0 && string(envelope.declaredSuccess) != "true" {
		return nil, fmt.Errorf("%s响应未确认登录成功", operation)
	}
	return envelope.Data, nil
}

func NewCoc3API(client *http.Client) *Coc3API {
	return &Coc3API{client: client}
}

// LoginCoc3 云盘 Authorization → 056002 统一认证票 → coc3 token（每次现签，票据一次性）。
func (api *Coc3API) LoginCoc3(authorization, phone string) (*Coc3LoginResult, error) {
	authorization = strings.TrimSpace(authorization)
	if authorization == "" {
		return nil, fmt.Errorf("缺少云盘 Authorization，无法换 coc3 token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), coc3LoginTimeout)
	defer cancel()
	ticket, err := auth.NewAuth(api.client).QuerySpecTokenForSourceContext(ctx, authorization, phone, coc3SourceID)
	if err != nil {
		return nil, fmt.Errorf("出票(056002)失败: %w", err)
	}
	if ticket == "" {
		return nil, fmt.Errorf("出票(056002)失败: 票据为空")
	}
	return api.userLoginV2(ctx, ticket)
}

// userLoginV2 统一认证票 → coc3 token。ticketType=11 是 App 内 WebView 的
// YDRZ 通道（扫码是 6），拿错通道号服务端不认票。
func (api *Coc3API) userLoginV2(ctx context.Context, ticket string) (*Coc3LoginResult, error) {
	body := map[string]interface{}{
		"ticket": ticket, "ticketType": 11, "businessType": "RIGHTS_MARKET",
		"wxoaType": "", "m1": "", "m2": "", "s1": "", "s2": "", "from": "",
	}
	resp, err := api.client.RequestWithContextOptions(ctx, "POST", coc3LoginURL, map[string]string{
		"Accept":           "application/json, text/plain, */*",
		"Content-Type":     "application/json;charset=UTF-8",
		"Origin":           coc3BaseURL,
		"Referer":          coc3Referer,
		"X-Requested-With": coc3AppPackage,
		"User-Agent":       coc3UserAgent,
	}, body, http.WithoutCookieJar(), http.WithoutDefaultHeaders(), http.WithoutRedirects())
	if err != nil {
		return nil, fmt.Errorf("userLoginV2 请求失败: %w", err)
	}
	data, err := api.decodeLoginResponse(resp, "userLoginV2")
	if err != nil {
		return nil, err
	}
	var parsed struct {
		Token        string          `json:"token"`
		RefreshToken string          `json:"refreshToken"`
		MobileMask   string          `json:"mobileMask"`
		ProvinceCode json.RawMessage `json:"provinceCode"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, fmt.Errorf("解析 userLoginV2 登录数据失败: %w", err)
	}
	token := strings.TrimSpace(parsed.Token)
	if token == "" {
		return nil, fmt.Errorf("userLoginV2 未返回登录 token")
	}
	province, _ := coc3Scalar(parsed.ProvinceCode)
	return &Coc3LoginResult{
		Token:        token,
		RefreshToken: parsed.RefreshToken,
		MobileMask:   parsed.MobileMask,
		ProvinceCode: province,
	}, nil
}

func (api *Coc3API) coc3Headers(token string, referer string) map[string]string {
	if referer == "" {
		referer = coc3Referer
	}
	return map[string]string{
		"Accept":           "application/json, text/plain, */*",
		"Content-Type":     "application/json;charset=UTF-8",
		"Origin":           coc3BaseURL,
		"Referer":          referer,
		"X-Requested-With": coc3AppPackage,
		"User-Agent":       coc3UserAgent,
		"phone":            token,
		"token":            token,
		"Cookie":           coc3TokenHeader + "=" + token,
	}
}

// callCoc3 统一请求入口：任务族信封 resultCode 0/200 为成功，登录/抽奖族
// code "0"/"200" 为成功。
func (api *Coc3API) callCoc3(path string, token, referer string, body interface{}) (*Coc3Response, error) {
	resp, err := api.client.RequestWithOptions("POST", coc3BaseURL+"/coc3/"+strings.TrimLeft(path, "/"), api.coc3Headers(token, referer), body, http.WithoutCookieJar(), http.WithoutDefaultHeaders())
	if err != nil {
		return nil, err
	}
	return api.decodeResponse(resp, path)
}

func summarizeCoc3(text string) string {
	trimmed := strings.TrimSpace(text)
	if len(trimmed) > 160 {
		return trimmed[:160]
	}
	return trimmed
}

// Coc3SignInfo 签到任务状态。
type Coc3SignInfo struct {
	SignToday         bool
	AccumulateCount   int
	Unavailable       bool // 任务已结束等（-9009）
	UnavailableReason string
}

// GetTaskSignInfo 查询累签任务状态（如 2999 好礼月月领）。
func (api *Coc3API) GetTaskSignInfo(token string, taskID int) (*Coc3SignInfo, error) {
	return api.taskSignInfo(token, map[string]interface{}{"taskId": taskID})
}

func (api *Coc3API) taskSignInfo(token string, payload interface{}) (*Coc3SignInfo, error) {
	resp, err := api.callCoc3("coc3-market-activity/cmpp-manage/task/getTaskSignInfo",
		token, "", payload)
	if err != nil {
		return nil, err
	}
	info := &Coc3SignInfo{}
	if !resp.Success {
		if resp.resultCode == "-9009" || resp.Code == "-9009" {
			info.Unavailable = true
			info.UnavailableReason = resp.BusinessError("签到状态查询").Error()
			return info, nil
		}
		return nil, resp.BusinessError("签到状态查询")
	}
	var data struct {
		SignToday       json.RawMessage `json:"signToday"`
		AccumulateCount json.RawMessage `json:"accumulateCount"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("签到状态数据无效: %w", err)
	}
	value := strings.Trim(strings.TrimSpace(string(data.SignToday)), `"`)
	switch value {
	case "true", "1":
		info.SignToday = true
	case "false", "0":
	default:
		return nil, fmt.Errorf("签到状态缺少有效 signToday")
	}
	if len(data.AccumulateCount) > 0 && string(data.AccumulateCount) != "null" {
		info.AccumulateCount, err = coc3Integer(data.AccumulateCount, "累签天数")
		if err != nil {
			return nil, err
		}
	}
	return info, nil
}

// AccumulateSign 累签提交（成功直接下发奖励；-9014/-1108 已签=幂等成功）。
func (api *Coc3API) AccumulateSign(token string, taskID int, channelCode string) (*Coc3Response, error) {
	return api.callCoc3("coc3-market-activity/cmpp-manage/task/accumulateSign",
		token, "", map[string]interface{}{"taskId": taskID, "channelCode": channelCode})
}

// VisitTask 任务浏览上报（visitUrl 是页面代码非网址，必须带对应活动页 Referer）。
func (api *Coc3API) VisitTask(token, channelCode, visitURL string) (*Coc3Response, error) {
	referer := fmt.Sprintf("%s/coc3/canvas/rightsmarket-h5-canvas/online/%s?channelCode=%s&pageRecorded=true",
		coc3BaseURL, visitURL, channelCode)
	return api.callCoc3("coc3-market-activity/cmpp-manage/task/visit",
		token, referer, map[string]interface{}{"channelCode": channelCode, "visitUrl": visitURL})
}

// GetTaskProgress 任务进度（taskIdList 族；-102 参数错误等按失败返回原信封）。
func (api *Coc3API) GetTaskProgress(token string, taskIDs []int) (*Coc3Response, error) {
	return api.callCoc3("coc3-market-activity/cmpp-manage/task/getTaskList",
		token, "", map[string]interface{}{
			"taskIdList": taskIDs, "scene": 1, "moduleSource": 0, "queryType": 0,
		})
}

// GetBeanBalance 权益豆余额。
func (api *Coc3API) GetBeanBalance(token string) (int, error) {
	resp, err := api.callCoc3("gr-integral-ability/api/external/score/digIntel/getBalance",
		token, "", map[string]interface{}{})
	if err != nil {
		return 0, err
	}
	if !resp.Success {
		return 0, resp.BusinessError("权益豆余额查询")
	}
	return coc3Integer(resp.Data, "权益豆余额")
}

// ResidueLotteryCount 抽奖剩余次数。
func (api *Coc3API) ResidueLotteryCount(token string, activityID int) (int, error) {
	resp, err := api.callCoc3("coc3-market-activity/cmpp-manage/lottery/residueLotteryCount",
		token, "", map[string]interface{}{"activityId": activityID})
	if err != nil {
		return 0, err
	}
	if !resp.Success {
		return 0, resp.BusinessError(fmt.Sprintf("剩余次数查询(%d)", activityID))
	}
	return coc3Integer(resp.Data, "抽奖剩余次数")
}

// CurrentLotterySku 当前抽奖 sku（0 元单提交要用）。
func (api *Coc3API) CurrentLotterySku(token string, activityID int) (int, error) {
	resp, err := api.callCoc3("coc3-market-activity/cmpp-manage/lottery/currentLottery",
		token, "", map[string]interface{}{"activityId": activityID, "scene": 1})
	if err != nil {
		return 0, err
	}
	if !resp.Success {
		return 0, resp.BusinessError(fmt.Sprintf("抽奖 sku 查询(%d)", activityID))
	}
	var data struct {
		SkuID json.RawMessage `json:"skuId"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return 0, fmt.Errorf("抽奖 sku 数据无效: %w", err)
	}
	sku, err := coc3Integer(data.SkuID, "抽奖 sku")
	if err == nil && sku == 0 {
		err = fmt.Errorf("抽奖 sku 必须大于 0")
	}
	return sku, err
}

// SubmitCoc3ZeroOrder 提交 0 元抽奖单（basePayType=3 免费次数通道；不碰任何
// 付费/权益豆路径）。wasmParams 由纯 Go 执行内置 WASM 按需生成，
// CAIYUN_COC3_WASM_PARAMS 只作显式覆盖；令牌为空时禁止提交。
func (api *Coc3API) SubmitCoc3ZeroOrder(token, wasmParams string, activityID, skuID int) (*Coc3Response, error) {
	if strings.TrimSpace(wasmParams) == "" || activityID <= 0 || skuID <= 0 {
		return nil, fmt.Errorf("0元抽奖缺少 wasmParams 或有效活动/sku")
	}
	body := map[string]interface{}{
		"skuId": fmt.Sprint(skuID), "channelCode": "P00000011939",
		"selfChannelCode": "P00000123844", "smsCode": "", "dealType": 0,
		"basePayType": 3, "basePayAmount": 0,
		"actId": activityID, "activityId": activityID,
	}
	headers := api.coc3Headers(token, coc3Referer)
	if strings.TrimSpace(wasmParams) != "" {
		headers["wasmParams"] = strings.TrimSpace(wasmParams)
	}
	resp, err := api.client.RequestWithOptions("POST", coc3OrderBase+"mixedOrder", headers, body, http.WithoutCookieJar(), http.WithoutDefaultHeaders())
	if err != nil {
		return nil, err
	}
	return api.decodeResponse(resp, "0元抽奖提交")
}
