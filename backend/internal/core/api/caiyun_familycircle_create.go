package api

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

type FamilyCircleIcon struct {
	ID json.Number `json:"iconStaticId"`
}

type FamilyCircleTopic struct {
	ID    json.Number        `json:"id"`
	Name  string             `json:"name"`
	Icons []FamilyCircleIcon `json:"defaultIconList"`
}

type FamilyCircleType struct {
	Type   json.Number         `json:"type"`
	Name   string              `json:"name"`
	Icons  []FamilyCircleIcon  `json:"defaultIconList"`
	Topics []FamilyCircleTopic `json:"topicList"`
}

func (api *CaiyunAPI) familyCircleGroupData(path string, payload interface{}) (json.RawMessage, error) {
	resp, err := api.client.Post("https://group.yun.139.com/hcy/group"+path, map[string]string{
		"x-yun-app-channel": "10000023", "x-SvcType": "1", "x-yun-api-version": "v1",
	}, payload)
	if err != nil {
		return nil, err
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}
	if err := validateFileUploadResponse(resp, body); err != nil {
		return nil, fmt.Errorf("家庭圈%s: %w", path, err)
	}
	var response struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal([]byte(body), &response) != nil || len(response.Data) == 0 || string(response.Data) == "null" {
		return nil, fmt.Errorf("家庭圈%s响应缺少数据", path)
	}
	return response.Data, nil
}

func (api *CaiyunAPI) FamilyCircleTypes() ([]FamilyCircleType, error) {
	account := strings.TrimSpace(api.client.GetMarketAccount())
	if account == "" {
		return nil, ErrFamilyCircleAccountMissing
	}
	data, err := api.familyCircleGroupData("/manage/type/list", map[string]string{"account": account})
	if err != nil {
		return nil, err
	}
	var types []FamilyCircleType
	if json.Unmarshal(data, &types) != nil || types == nil {
		return nil, fmt.Errorf("家庭圈类型列表无效")
	}
	return types, nil
}

// CreatePersonalFamilyCircle is an explicitly invoked owner-only operation.
// Normal family tasks never auto-create a circle. No members are invited and
// no carrier-plan/kinship adapters are called. Creation requires owned readback.
func (api *CaiyunAPI) CreatePersonalFamilyCircle(name string, topicID, iconID int64) (*FamilyCircleGroup, error) {
	account := strings.TrimSpace(api.client.GetMarketAccount())
	if account == "" {
		return nil, ErrFamilyCircleAccountMissing
	}
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 20 || topicID <= 0 || iconID <= 0 {
		return nil, fmt.Errorf("家庭圈创建名称或主题图标无效")
	}
	groups, err := api.FamilyCircleGroups()
	if err != nil {
		return nil, err
	}
	owned := func(group FamilyCircleGroup) bool {
		return group.ID != "" && group.Type == "0" && group.Name == name && group.MemberCount == "1" && (group.CreatorAccount == account || group.ManagerAccount == account)
	}
	for _, group := range groups {
		if owned(group) {
			return &group, nil
		}
		if group.Type == "0" {
			return nil, fmt.Errorf("当前账号已有家庭圈，未重复创建或操作现有圈")
		}
	}
	types, err := api.FamilyCircleTypes()
	if err != nil {
		return nil, err
	}
	valid := false
	for _, kind := range types {
		if kind.Type.String() != "0" {
			continue
		}
		for _, topic := range kind.Topics {
			id, _ := topic.ID.Int64()
			if id == topicID {
				for _, icon := range topic.Icons {
					id, _ := icon.ID.Int64()
					valid = valid || id == iconID
				}
			}
		}
	}
	if !valid {
		return nil, fmt.Errorf("家庭圈主题和图标不是官方当前返回的组合")
	}
	data, err := api.familyCircleGroupData("/manage/create", map[string]interface{}{
		"account": account, "name": name, "type": 0, "topicId": topicID, "iconStaticId": iconID,
		"memberList": []string{},
	})
	if err != nil {
		return nil, err
	}
	var created struct {
		Group struct {
			ID string `json:"groupId"`
		} `json:"groupInfo"`
	}
	// Current release responses can omit the bridge DTO's groupInfo.groupId.
	// Confirm the newly created owned single-member circle through myGroupList;
	// an acknowledgement without this record remains unconfirmed and is not retried.
	_ = json.Unmarshal(data, &created)
	groups, err = api.FamilyCircleGroups()
	if err != nil {
		return nil, fmt.Errorf("家庭圈创建后归属回查失败: %w", err)
	}
	for _, group := range groups {
		if (created.Group.ID == "" || group.ID == created.Group.ID) && owned(group) {
			return &group, nil
		}
	}
	return nil, fmt.Errorf("家庭圈创建后未确认本人的单成员家庭圈，未记为任务完成")
}
