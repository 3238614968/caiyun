package api

import (
	"encoding/json"
)

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
