package api

// /market/rafflecode/*（抽奖码）。
//
// 与 `/market/unLoading/*` 同族：认证头是 `jwtToken`。逆向实测当前
// `rafflecodeConfigList` 为空数组（无进行中的场次），最后一期 20260729
// 已于 2026-07-29 结束。因此这里只做查询与如实报告。
const rafflecodeBase = MarketURL + "/rafflecode"

// RafflecodeInfo 抽奖码概览（未读记录、订阅开关）。
func (api *CaiyunAPI) RafflecodeInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/info", map[string]interface{}{}, nil)
}

// RafflecodeList 抽奖码列表（含 rafflecodeConfigList 场次配置）。
func (api *CaiyunAPI) RafflecodeList() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/list", map[string]interface{}{}, nil)
}

// RafflecodeMyPrize 我的抽奖码奖品。
func (api *CaiyunAPI) RafflecodeMyPrize() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/getMyPrize", map[string]interface{}{}, nil)
}

// RafflecodeData 抽奖码数据。
func (api *CaiyunAPI) RafflecodeData() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/getRafflecodeData", map[string]interface{}{}, nil)
}

// RafflecodeLotteryRecord 开奖记录。
func (api *CaiyunAPI) RafflecodeLotteryRecord() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/getlotteryRecordV2", map[string]interface{}{}, nil)
}

// RafflecodeActiveSessions 返回当前可参与的场次数量。
func RafflecodeActiveSessions(resp *CaiyunResponse) int {
	result, ok := resultMap(resp)
	if !ok {
		return 0
	}
	sessions, _ := result["rafflecodeConfigList"].([]interface{})
	return len(sessions)
}
