package api

// National_v13gift（全网云盘焕新权益免费领）。
//
// 奖池与激活接口。`getPrize`smsCode=innerActivation` 是活动侧提供的
// 免验证码官方通道（见 caiyun_hidden_rewards.go 的 ActivateUpgradeGift）；
// 无领奖记录时回 `404 未找到领奖记录`，表示账号不在白名单，属正常结果。
const upgradeGiftBase = MobileMarketURL + "/fivenewcomer/upgradeGifts"

// UpgradeGiftPrizePool 查询奖池（含 100G 云空间年卡等）。
func (api *CaiyunAPI) UpgradeGiftPrizePool() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", upgradeGiftBase+"/getPrizePool", nil, nil)
}

// UpgradeGiftVerCode 触发短信验证码（短信通道激活前调用）。
func (api *CaiyunAPI) UpgradeGiftVerCode() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", upgradeGiftBase+"/getVerCode", nil, nil)
}

// UpgradeGiftPrizePoolEntry 奖池条目。
type UpgradeGiftPrizePoolEntry struct {
	PrizeID     string `json:"prizeId"`
	PrizeName   string `json:"prizeName"`
	Count       int    `json:"count"`
	DailyRemain int    `json:"dailyRemainderCount"`
}

// ParseUpgradeGiftPrizePool 解析奖池列表。
func ParseUpgradeGiftPrizePool(resp *CaiyunResponse) []UpgradeGiftPrizePoolEntry {
	items := resultArray(resp)
	if len(items) == 0 {
		if result, ok := resultMap(resp); ok {
			items, _ = result["prizePool"].([]interface{})
			if len(items) == 0 && mapString(result, "prizeName") != "" {
				items = []interface{}{result}
			}
		}
	}
	entries := make([]UpgradeGiftPrizePoolEntry, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		entries = append(entries, UpgradeGiftPrizePoolEntry{
			PrizeID:     mapString(entry, "prizeId"),
			PrizeName:   mapString(entry, "prizeName"),
			Count:       mapInt(entry, "count"),
			DailyRemain: mapInt(entry, "dailyRemainderCount"),
		})
	}
	return entries
}
