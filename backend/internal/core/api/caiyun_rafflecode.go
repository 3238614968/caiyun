package api

import "net/url"

// /market/rafflecode/*（抽奖码）。
//
// 与 `/market/unLoading/*` 同族：认证头是 `jwtToken`。逆向实测当前
// `rafflecodeConfigList` 为空数组（无进行中的场次），最后一期 20260729
// 已于 2026-07-29 结束。因此这里只做查询与如实报告。
const rafflecodeBase = "https://m.mcloud.139.com/market/rafflecode"

// The official lotteryIphone page uses qs.stringify for these endpoints,
// with JSON reserved for getRafflecodeData. Preserve that distinction.
func (api *CaiyunAPI) rafflecodeForm(path string, values url.Values) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+path, values.Encode(), map[string]string{
		"Content-Type":     "application/x-www-form-urlencoded;charset=UTF-8",
		"X-Requested-With": "XMLHttpRequest",
		"Referer":          "https://m.mcloud.139.com/portal/cloudCircle/index.html?path=lotteryIphone",
	})
}

// RafflecodeInfo 抽奖码概览（未读记录、订阅开关）。
func (api *CaiyunAPI) RafflecodeInfo() (*CaiyunResponse, error) {
	return api.rafflecodeForm("/info", url.Values{"marketName": {"rafflecode"}})
}

// RafflecodeList 当前场次。官方页面和参考主脚本调用独立配置端点；
// /list 在当前部署回 HTTP 405，不能将它当作配置列表接口。
func (api *CaiyunAPI) RafflecodeList() (*CaiyunResponse, error) {
	return api.rafflecodeForm("/rafflecodeConfigList", url.Values{"marketName": {"rafflecode"}})
}

// RafflecodeMyPrize 我的抽奖码奖品。
func (api *CaiyunAPI) RafflecodeMyPrize() (*CaiyunResponse, error) {
	return api.rafflecodeForm("/getMyPrize", url.Values{"marketName": {"rafflecode"}})
}

// RafflecodeData 抽奖码数据。
func (api *CaiyunAPI) RafflecodeData() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", rafflecodeBase+"/getRafflecodeData", map[string]interface{}{}, nil)
}

// RafflecodeLotteryRecord 开奖记录。
func (api *CaiyunAPI) RafflecodeLotteryRecord() (*CaiyunResponse, error) {
	return api.rafflecodeForm("/getlotteryRecordV2", url.Values{"marketName": {"rafflecode"}})
}

// RafflecodeActiveSessions 返回当前可参与的场次数量。
func RafflecodeActiveSessions(resp *CaiyunResponse) int {
	if resp != nil {
		if sessions, ok := resp.Result.([]interface{}); ok {
			return len(sessions)
		}
	}
	result, ok := resultMap(resp)
	if !ok {
		return 0
	}
	sessions, _ := result["rafflecodeConfigList"].([]interface{})
	return len(sessions)
}
