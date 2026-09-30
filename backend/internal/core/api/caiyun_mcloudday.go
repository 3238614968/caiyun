package api

import "fmt"

// National_MCloudDay（全网移动云盘会员日）。
//
// activityInfo 的 online / extGiftOnline / blindboxOnline 分别表示主活动、
// 额外礼包、盲盒是否开放 —— 「主活动关、额外礼包开」是常见组合，
// 因此以各业务接口的实际回执为准，而不是只看 online。
const mcloudDayBase = MobileMarketURL + "/mcloudday"

// MCloudDayActivityInfo 会员日开放状态。
type MCloudDayActivityInfo struct {
	Online         bool `json:"online"`
	ExtGiftOnline  bool `json:"extGiftOnline"`
	BlindboxOnline bool `json:"blindboxOnline"`
}

// MCloudDayActivityInfo 查询会员日开放状态。
func (api *CaiyunAPI) MCloudDayActivityInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", mcloudDayBase+"/common/activityInfo", nil, nil)
}

// MCloudDayGiftList 会员日实物/权益礼品列表。
func (api *CaiyunAPI) MCloudDayGiftList() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", mcloudDayBase+"/gift/list", nil, nil)
}

// MCloudDayGiftVerify 校验领取资格（服务端回执含 hasStock 等字段）。
func (api *CaiyunAPI) MCloudDayGiftVerify() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", mcloudDayBase+"/gift/verify", map[string]interface{}{}, nil)
}

// MCloudDayGiftReceive 领取礼品。
func (api *CaiyunAPI) MCloudDayGiftReceive() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", mcloudDayBase+"/gift/receive", map[string]interface{}{}, nil)
}

// MCloudDayBlindboxLottery 盲盒抽奖。
func (api *CaiyunAPI) MCloudDayBlindboxLottery() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", mcloudDayBase+"/blindbox/lottery", map[string]interface{}{}, nil)
}

// ParseMCloudDayActivityInfo 解析会员日开放状态。
func ParseMCloudDayActivityInfo(resp *CaiyunResponse) (MCloudDayActivityInfo, bool) {
	result, ok := resultMap(resp)
	if !ok {
		return MCloudDayActivityInfo{}, false
	}
	return MCloudDayActivityInfo{
		Online:         mapBool(result, "online"),
		ExtGiftOnline:  mapBool(result, "extGiftOnline"),
		BlindboxOnline: mapBool(result, "blindboxOnline"),
	}, true
}

// MCloudDayGiftHasStock 判断礼品列表里是否还有库存。
func MCloudDayGiftHasStock(resp *CaiyunResponse) bool {
	items := resultArray(resp)
	if len(items) == 0 {
		if result, ok := resultMap(resp); ok {
			items, _ = result["list"].([]interface{})
		}
	}
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		if mapBool(entry, "hasStock") {
			return true
		}
	}
	return false
}

// MCloudDayMessage 汇总会员日状态文案，供任务日志使用。
func MCloudDayMessage(info MCloudDayActivityInfo, hasStock bool) string {
	return fmt.Sprintf(
		"会员日：主活动=%v、额外礼包=%v、盲盒=%v、礼品有货=%v",
		info.Online, info.ExtGiftOnline, info.BlindboxOnline, hasStock,
	)
}
