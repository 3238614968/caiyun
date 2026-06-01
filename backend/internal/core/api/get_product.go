package api

import (
	"encoding/json"
	"fmt"
)

// GetProductList 获取商品列表
func (api *CaiyunAPI) GetProductList() (*CaiyunResponse, error) {
	const productListURL = "https://m.mcloud.139.com/market/signin/page/exchangeList?client=app&clientVersion=12.5.3"

	api.ensureMarketDeviceID()

	resp, err := api.client.Get(productListURL, map[string]string{
		"showloading":     "true",
		"Accept-Encoding": "gzip, deflate",
	})
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("解析商品列表响应失败: %w, body: %s", err, body)
	}

	return &result, nil
}
