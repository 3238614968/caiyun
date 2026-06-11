package api

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"
)

func (api *CaiyunAPI) ensureMarketDeviceID() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_ = api.client.EnsureShumeiDeviceID(ctx)
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
	api.ensureMarketDeviceID()

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
