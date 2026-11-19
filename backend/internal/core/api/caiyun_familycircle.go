package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"

	corehttp "caiyun/internal/core/http"
)

// 家庭圈任务（ICircleMarketCampaignApi）。
//
// 需求体字段名是服务端下发的 `gruopId`（拼写如此，不是 groupId），
// 另需当前账号的 userDomainId。APK MarketCampaignRetrofitFactory uses the
// caiyun release gateway; queryBackupState is GET, queryCircleTaskState is POST.
const familyCircleBase = "https://caiyun.feixin.10086.cn/ycloud/south/national/familyCircle"

var ErrFamilyCircleAccountMissing = errors.New("家庭圈群组查询缺少账号标识")

// A missing marketing route says nothing about group ownership or backup
// completion. Keep this separate from discovery of the actual family circle.
var ErrFamilyCircleEndpointUnavailable = errors.New("家庭圈营销状态接口不可用")

type familyCircleTransportError struct {
	operation string
	cause     error
}

func (e *familyCircleTransportError) Error() string {
	return fmt.Sprintf("家庭圈 %s 请求未确认", e.operation)
}

func (e *familyCircleTransportError) Unwrap() error { return e.cause }

type FamilyCircleGroup struct {
	ID             string `json:"id"`
	Type           string `json:"type"`
	CreatorAccount string `json:"creatorAccount"`
	ManagerAccount string `json:"managerAccount"`
	Name           string `json:"name"`
	MemberCount    string `json:"memberNum"`
}

func (group *FamilyCircleGroup) UnmarshalJSON(data []byte) error {
	var wire struct {
		ID             json.RawMessage `json:"id"`
		Type           json.RawMessage `json:"type"`
		Members        json.RawMessage `json:"memberNum"`
		CreatorAccount string          `json:"creatorAccount"`
		ManagerAccount string          `json:"managerAccount"`
		Name           string          `json:"name"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	var err error
	if group.ID, err = familyCircleText(wire.ID); err != nil {
		return err
	}
	if group.Type, err = familyCircleText(wire.Type); err != nil {
		return err
	}
	if group.MemberCount, err = familyCircleText(wire.Members); err != nil {
		return err
	}
	group.CreatorAccount, group.ManagerAccount, group.Name = wire.CreatorAccount, wire.ManagerAccount, wire.Name
	return nil
}

func familyCircleText(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var text string
	if json.Unmarshal(raw, &text) == nil {
		return text, nil
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil && !strings.ContainsAny(number.String(), ".eE") {
		return number.String(), nil
	}
	return "", fmt.Errorf("家庭圈标识或类型不是字符串/整数")
}

// FamilyCircleGroups reads only this account's groups. Native myGroupList has
// pageInfo.list, not the grouped marketing-task response shape.
func (api *CaiyunAPI) FamilyCircleGroups() ([]FamilyCircleGroup, error) {
	account := strings.TrimSpace(api.client.GetMarketAccount())
	if account == "" {
		return nil, ErrFamilyCircleAccountMissing
	}
	var groups []FamilyCircleGroup
	seen := make(map[string]bool)
	for page := 1; page <= 20; page++ {
		resp, err := api.client.Post("https://group.yun.139.com/hcy/group/manage/circle/myGroupList", map[string]string{
			"x-yun-app-channel": "10000023", "x-SvcType": "1", "x-yun-api-version": "v1",
		}, map[string]interface{}{"account": account, "pageNumber": page, "pageSize": 100})
		if err != nil {
			return nil, fmt.Errorf("家庭圈群组查询失败: %w", err)
		}
		body, err := api.client.ReadResponseBody(resp)
		if err != nil {
			return nil, err
		}
		if err := validateFileUploadResponse(resp, body); err != nil {
			return nil, fmt.Errorf("家庭圈群组查询失败: %w", err)
		}
		var result struct {
			Data struct {
				PageInfo *struct {
					List       []FamilyCircleGroup `json:"list"`
					IsLastPage bool                `json:"isLastPage"`
					Pages      int                 `json:"pages"`
				} `json:"pageInfo"`
			} `json:"data"`
		}
		if err := json.Unmarshal([]byte(body), &result); err != nil {
			return nil, err
		}
		if result.Data.PageInfo == nil || result.Data.PageInfo.List == nil {
			return nil, fmt.Errorf("家庭圈响应缺少 pageInfo.list，不能判定没有群组")
		}
		for _, group := range result.Data.PageInfo.List {
			if group.ID == "" || seen[group.ID] {
				continue
			}
			seen[group.ID] = true
			groups = append(groups, group)
		}
		if result.Data.PageInfo.IsLastPage || len(result.Data.PageInfo.List) == 0 || (result.Data.PageInfo.Pages > 0 && page >= result.Data.PageInfo.Pages) {
			return groups, nil
		}
	}
	return nil, fmt.Errorf("家庭圈群组列表超过分页上限，结果不完整")
}

// SetFamilyCircleBase 覆盖家庭圈接口基线地址（供后续确认真实宿主时调整）。
func (api *CaiyunAPI) SetFamilyCircleBase(base string) {
	if base != "" {
		api.familyCircleBase = base
	}
}

// FamilyCircleTaskState queries the selected group's native scalar task state.
func (api *CaiyunAPI) FamilyCircleTaskState(groupID string) (*CaiyunResponse, error) {
	return api.familyCircleRequest("/queryCircleTaskState", groupID)
}

// FamilyCircleBackupState 查询家庭圈备份状态。
func (api *CaiyunAPI) FamilyCircleBackupState(groupID string) (*CaiyunResponse, error) {
	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("家庭圈备份查询缺少 gruopId")
	}
	userID := api.client.GetUserDomainID()
	if userID == "" {
		return nil, fmt.Errorf("缺少 userDomainId，无法查询家庭圈备份")
	}
	base := api.familyCircleBase
	if base == "" {
		base = familyCircleBase
	}
	query := url.Values{"userId": {userID}, "gruopId": {groupID}}
	return api.familyCircleStateRequest("GET", base+"/queryBackupState?"+query.Encode(), nil, "queryBackupState")
}

func (api *CaiyunAPI) familyCircleRequest(path, groupID string) (*CaiyunResponse, error) {
	userID := api.client.GetUserDomainID()
	if userID == "" {
		return nil, fmt.Errorf("缺少 userDomainId，无法查询家庭圈任务")
	}
	if strings.TrimSpace(groupID) == "" {
		return nil, fmt.Errorf("家庭圈任务查询缺少 gruopId")
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
	return api.familyCircleStateRequest("POST", base+path, payload, "queryCircleTaskState")
}

func (api *CaiyunAPI) familyCircleStateRequest(method, endpoint string, payload interface{}, operation string) (*CaiyunResponse, error) {
	response, err := api.client.RequestWithOptions(method, endpoint, api.buildMarketHeaders(nil, ""), payload,
		corehttp.WithoutRetries(), corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(10*time.Second))
	if err != nil {
		// Do not copy query parameters containing user/group IDs into task logs.
		return nil, &familyCircleTransportError{operation: operation, cause: err}
	}
	defer response.Body.Close()
	if response.StatusCode == 404 {
		return nil, fmt.Errorf("%w：%s HTTP 404，不能据此判定无家庭圈或任务完成", ErrFamilyCircleEndpointUnavailable, operation)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("家庭圈 %s HTTP %d", operation, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil {
		return nil, &familyCircleTransportError{operation: operation + " 响应读取", cause: err}
	}
	if len(body) > 64<<10 {
		return nil, fmt.Errorf("家庭圈 %s 响应读取未确认", operation)
	}
	var result CaiyunResponse
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	decoder.UseNumber()
	if err := decoder.Decode(&result); err != nil || result.Code == nil {
		return nil, fmt.Errorf("家庭圈 %s 缺少有效状态信封", operation)
	}
	if decoder.Decode(new(interface{})) != io.EOF {
		return nil, fmt.Errorf("家庭圈 %s 状态信封含额外数据", operation)
	}
	if fmt.Sprint(result.Code) == "0" {
		var scalarErr error
		switch state := result.Result.(type) {
		case json.Number:
			_, scalarErr = state.Int64()
		case string:
			_, scalarErr = strconv.ParseInt(strings.TrimSpace(state), 10, 64)
		default:
			scalarErr = errors.New("missing scalar state")
		}
		if scalarErr != nil {
			return nil, fmt.Errorf("家庭圈 %s 缺少原生整型状态", operation)
		}
	}
	// Native result is a scalar status. The caller must independently read a
	// documented completion record; code=0/result=1 alone is not that record.
	return &result, nil
}

// FamilyCircleHasGroup 判断响应里是否解析出了家庭圈群组。
func FamilyCircleHasGroup(resp *CaiyunResponse) bool {
	result, ok := resultMap(resp)
	if !ok {
		return false
	}
	return mapString(result, "gruopId") != "" || mapString(result, "groupId") != ""
}
