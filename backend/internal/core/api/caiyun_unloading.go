package api

import (
	"fmt"
	"net/url"
	"strings"
)

// newgifts1T（移动云盘 1T 新礼）。
//
// 这批接口挂在 `/market/*` 下，认证头是 `jwtToken`（与 `jwttoken` 同值，
// 但服务端只读这个头名）；缺了会静默回 `90001 未登录`。
// buildMarketHeaders 已同时写入两个头名，这里直接复用。
//
// 逆向实测：奖品为「1T 个人云空间月卡」，但认证通过后回 `503 远程调用失败`
// （后端未部署）。因此本实现只做探测与如实报告。
const unloadMarketName = "unload_once"

const unloadBase = MarketURL + "/unLoading"

// MarketHost 活动层宿主（不含 /market 前缀），用于 /market/xxx 这类平铺路径。
const MarketHost = "https://caiyun.feixin.10086.cn"

// UnloadUserInfo 查询 1T 新礼资格。
func (api *CaiyunAPI) UnloadUserInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		unloadBase+"/userInfo?marketName="+url.QueryEscape(unloadMarketName),
		nil,
		nil,
	)
}

// UnloadPrizeRecords 已领奖记录。
func (api *CaiyunAPI) UnloadPrizeRecords() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		unloadBase+"/getPrizeRecords?marketName="+url.QueryEscape(unloadMarketName),
		nil,
		nil,
	)
}

// UnloadLinksList 活动链接列表。
func (api *CaiyunAPI) UnloadLinksList() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		unloadBase+"/getLinksList?marketName="+url.QueryEscape(unloadMarketName),
		nil,
		nil,
	)
}

// UnloadSendPrize 领取指定类型的奖励，code 为明文兑换码，内部转 MD5 提交。
func (api *CaiyunAPI) UnloadSendPrize(prizeType, code string) (*CaiyunResponse, error) {
	prizeType = strings.TrimSpace(prizeType)
	if prizeType == "" {
		prizeType = "1"
	}
	if strings.TrimSpace(code) == "" {
		return nil, fmt.Errorf("领取 1T 新礼需要兑换码")
	}
	return api.marketJSONRequest(
		"POST",
		fmt.Sprintf("%s/market/sendPrize/%s?code=%s", MarketHost, url.QueryEscape(prizeType), md5HexLower(code)),
		map[string]interface{}{"marketName": unloadMarketName},
		nil,
	)
}
