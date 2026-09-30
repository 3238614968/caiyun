package api

import "fmt"

// 家庭圈任务（ICircleMarketCampaignApi）。
//
// 需求体字段名是服务端下发的 `gruopId`（拼写如此，不是 groupId），
// 另需当前账号的 userDomainId。两个账号都需要**先开通家庭圈群组**
// 才谈得上任务状态，因此无群组时服务端返回空/404 属功能门槛，
// 不是接口问题。
//
// 上游宿主在逆向中未最终确定（`caiyun.feixin.10086.cn/ycloud/south/national/`、
// `gp.mcloud.139.com`、`7071/market` 三处均回 404/405）。这里保留可覆盖的
// 基线地址，便于后续按实际部署调整。
const familyCircleBase = MobileMarketURL + "/south/national/familyCircle"

// SetFamilyCircleBase 覆盖家庭圈接口基线地址（供后续确认真实宿主时调整）。
func (api *CaiyunAPI) SetFamilyCircleBase(base string) {
	if base != "" {
		api.familyCircleBase = base
	}
}

// FamilyCircleTaskState 查询家庭圈任务状态。groupID 为空时只带 userId。
func (api *CaiyunAPI) FamilyCircleTaskState(groupID string) (*CaiyunResponse, error) {
	return api.familyCircleRequest("/queryCircleTaskState", groupID)
}

// FamilyCircleBackupState 查询家庭圈备份状态。
func (api *CaiyunAPI) FamilyCircleBackupState(groupID string) (*CaiyunResponse, error) {
	return api.familyCircleRequest("/queryBackupState", groupID)
}

func (api *CaiyunAPI) familyCircleRequest(path, groupID string) (*CaiyunResponse, error) {
	userID := api.client.GetUserDomainID()
	if userID == "" {
		return nil, fmt.Errorf("缺少 userDomainId，无法查询家庭圈任务")
	}
	payload := map[string]interface{}{"userId": userID}
	if groupID != "" {
		// 服务端字段名就是 gruopId（活动侧原始拼写）。
		payload["gruopId"] = groupID
	}
	base := api.familyCircleBase
	if base == "" {
		base = familyCircleBase
	}
	return api.marketJSONRequest("POST", base+path, payload, nil)
}

// FamilyCircleHasGroup 判断响应里是否解析出了家庭圈群组。
func FamilyCircleHasGroup(resp *CaiyunResponse) bool {
	result, ok := resultMap(resp)
	if !ok {
		return false
	}
	return mapString(result, "gruopId") != "" || mapString(result, "groupId") != ""
}
