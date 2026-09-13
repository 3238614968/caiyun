package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// 移动云盘 WebView 活动（算力大作战、许愿、趣玩AI、校园海报）共用同一套鉴权方式：
// 通过带 SSO token 的活动页建立 Cookie 会话，再携带 jwtToken/activityid 头访问接口。
const (
	TokenPKMarketName  = "National_TokenPK"
	MakeWishMarketName = "National_MakeWish"
	FunAIMarketName    = "National_playAI"
	PosterMarketName   = "National_PlayAISpecial"
)

// activityPortalConfig 描述各活动 H5 入口页（不同活动挂载在不同的 portal 路径下）。
type activityPortalConfig struct {
	pagePath string
	sourceID string
}

var activityPortals = map[string]activityPortalConfig{
	TokenPKMarketName:  {pagePath: "yunClound", sourceID: "1073"},
	MakeWishMarketName: {pagePath: "hcyview", sourceID: "1075"},
	FunAIMarketName:    {pagePath: "huiyuanri", sourceID: "1170"},
	PosterMarketName:   {pagePath: "yunpanpage", sourceID: "2073"},
}

func (api *CaiyunAPI) buildActivityPageURL(marketName, extraQuery string) string {
	conf := activityPortals[marketName]
	if conf.pagePath == "" {
		conf = activityPortalConfig{pagePath: "yunClound", sourceID: MarketSourceID}
	}
	pageURL := fmt.Sprintf(
		"https://m.mcloud.139.com/portal/%s/index.html?path=%s&sourceid=%s&enableShare=1&token=%s",
		conf.pagePath,
		url.QueryEscape(marketName),
		url.QueryEscape(conf.sourceID),
		url.QueryEscape(strings.TrimSpace(api.client.GetSSOToken())),
	)
	if extra := strings.TrimSpace(extraQuery); extra != "" {
		pageURL += "&" + strings.TrimPrefix(extra, "&")
	}
	return pageURL
}

func (api *CaiyunAPI) buildActivityHeaders(marketName, referer string, extraHeaders map[string]string) map[string]string {
	headers := map[string]string{
		"User-Agent":       MarketUserAgent,
		"Accept":           "*/*",
		"Origin":           "https://m.mcloud.139.com",
		"X-Requested-With": "com.chinamobile.mcloud",
		"Cache-Control":    "no-cache",
		"showLoading":      "true",
		"appVersion":       MarketClientVersion + ".0",
		"activityId":       marketName,
	}
	if token := strings.TrimSpace(api.client.GetJWTToken()); token != "" {
		headers["jwtToken"] = token
		headers["jwttoken"] = token
	}
	if referer == "" {
		referer = api.buildActivityPageURL(marketName, "")
	}
	headers["Referer"] = referer
	for key, value := range extraHeaders {
		if strings.TrimSpace(value) != "" {
			headers[key] = value
		}
	}
	return headers
}

// PrepareActivitySession 打开活动 H5 入口页，让服务端基于 SSO token 建立
// JSESSIONID 会话。每个活动任务在执行接口序列前调用一次即可。
func (api *CaiyunAPI) PrepareActivitySession(marketName string) {
	api.ensureMarketDeviceID()

	pageURL := api.buildActivityPageURL(marketName, "")
	if resp, err := api.client.Get(pageURL, api.buildActivityHeaders(marketName, pageURL, nil)); err == nil && resp != nil {
		_, _ = api.client.ReadResponseBody(resp)
	}
}

// activityGetBody 请求活动 GET 接口并返回响应正文。
func (api *CaiyunAPI) activityGetBody(marketName, urlStr string) (string, error) {
	resp, err := api.client.Get(urlStr, api.buildActivityHeaders(marketName, "", nil))
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", urlStr, err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取 %s 响应失败: %w", urlStr, err)
	}
	return body, nil
}

// activityPostBody 请求活动 POST JSON 接口并返回响应正文。
func (api *CaiyunAPI) activityPostBody(marketName, urlStr string, payload interface{}) (string, error) {
	resp, err := api.client.Post(urlStr, api.buildActivityHeaders(marketName, "", map[string]string{
		"Content-Type": "application/json",
	}), payload)
	if err != nil {
		return "", fmt.Errorf("请求 %s 失败: %w", urlStr, err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return "", fmt.Errorf("读取 %s 响应失败: %w", urlStr, err)
	}
	return body, nil
}

// activityResponse 是 ycloud 活动接口统一的 {code,msg,result} 信封。
type activityResponse struct {
	Code    json.Number     `json:"code"`
	Msg     string          `json:"msg"`
	Message string          `json:"message"`
	Result  json.RawMessage `json:"result"`
}

func (r *activityResponse) OK() bool {
	return r.Code.String() == "0"
}

func (r *activityResponse) MessageText() string {
	if msg := strings.TrimSpace(r.Msg); msg != "" {
		return msg
	}
	return strings.TrimSpace(r.Message)
}

// decodeActivityBody 解析活动接口响应；非 0 code 时返回带业务文案的错误。
func decodeActivityBody(op, body string) (*activityResponse, error) {
	var envelope activityResponse
	if err := json.Unmarshal([]byte(strings.TrimSpace(body)), &envelope); err != nil {
		summary := strings.TrimSpace(body)
		if len(summary) > 200 {
			summary = summary[:200]
		}
		return nil, fmt.Errorf("%s 响应解析失败: %s", op, summary)
	}
	if !envelope.OK() {
		return &envelope, fmt.Errorf("%s 失败: code=%s %s", op, envelope.Code.String(), envelope.MessageText())
	}
	return &envelope, nil
}
