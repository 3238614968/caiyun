package api

// National_Meitubackup（全网美图授权备份领好礼）—— 仅移动号可参与。
//
// 两个前置条件来自设备侧：`authStatus` 是「用户是否授权美图访问」、
// `backUpStatus` 是「设备相册备份开关是否打开」。二者都由客户端上报，
// 因此本实现只做「查询 + 在服务端已记录满足时领奖」，不伪造设备状态。
const meituBase = MobileMarketURL + "/meitu/new"

// MeituBackupStatus 美图备份活动的状态汇总。
type MeituBackupStatus struct {
	AuthStatus     int `json:"authStatus"`
	AuthAwarded    int `json:"-"`
	BackupStatus   int `json:"backupStatus"`
	BackupAwarded  int `json:"-"`
	AuthTotalCount int `json:"authTotalCount"`
	AuthDailyCount int `json:"authDailyCount"`
	BackupTotal    int `json:"backupTotalCount"`
	BackupDaily    int `json:"backupDailyCount"`
}

// MeituPrizeCount 剩余可领次数。
func (api *CaiyunAPI) MeituPrizeCount() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/prizeCount", nil, nil)
}

// MeituPrizeList 已领记录。
func (api *CaiyunAPI) MeituPrizeList() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/prizeList", nil, nil)
}

// MeituAuthStatus 授权状态。
func (api *CaiyunAPI) MeituAuthStatus() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/authStatus", nil, nil)
}

// MeituBackupStatus 相册备份开关状态。
func (api *CaiyunAPI) MeituBackupState() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/backUpStatus", nil, nil)
}

// MeituIsRemind 提醒开关状态。
func (api *CaiyunAPI) MeituIsRemind() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/isRemind", nil, nil)
}

// MeituOpenRemind 打开提醒（不涉及奖励判定）。
func (api *CaiyunAPI) MeituOpenRemind() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/openRemind", nil, nil)
}

// MeituPrize 立即领取。服务端在前置未满足时回 `404 奖品不存在`。
func (api *CaiyunAPI) MeituPrize() (*CaiyunResponse, error) {
	return api.marketJSONRequest("GET", meituBase+"/prize", nil, nil)
}

// MeituSendSms 短信引导。
func (api *CaiyunAPI) MeituSendSms() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", meituBase+"/sendSms", map[string]interface{}{}, nil)
}

// ParseMeituAuthStatus 解析 authStatus 回执。
func ParseMeituAuthStatus(resp *CaiyunResponse) (status, awarded int, ok bool) {
	result, parsed := resultMap(resp)
	if !parsed {
		return 0, 0, false
	}
	return mapInt(result, "authStatus"), mapInt(result, "isAward"), true
}

// ParseMeituBackupStatus 解析 backUpStatus 回执。
func ParseMeituBackupStatus(resp *CaiyunResponse) (status, awarded int, ok bool) {
	result, parsed := resultMap(resp)
	if !parsed {
		return 0, 0, false
	}
	return mapInt(result, "backupStatus"), mapInt(result, "isAward"), true
}

// ParseMeituPrizeCount 解析剩余可领次数。
func ParseMeituPrizeCount(resp *CaiyunResponse) (authTotal, backupTotal int, ok bool) {
	result, parsed := resultMap(resp)
	if !parsed {
		return 0, 0, false
	}
	return mapInt(result, "authTotalCount"), mapInt(result, "backupTotalCount"), true
}

// MeituBackupEligible 判断服务端已记录的「授权 + 开备份」是否都已满足。
func MeituBackupEligible(authStatus, backupStatus int) bool {
	return authStatus == 1 && backupStatus == 1
}
