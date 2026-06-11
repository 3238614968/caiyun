package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func (api *CaiyunAPI) SignIn() (*SignInResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/startSignIn?client=app", MobileMarketURL),
		api.buildReceiveHeaders(""),
	)
	if err != nil {
		return nil, fmt.Errorf("request sign in failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read sign in response failed: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("sign in http status=%d, body=%s", resp.StatusCode, body)
	}

	var result SignInResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode sign in response failed: %w, body=%s", err, body)
	}
	return &result, nil
}

// GetCloudInfo 获取签到信息
func (api *CaiyunAPI) GetCloudInfo() (*CloudInfoResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/infoV3?client=app", MobileMarketURL),
		api.buildReceiveHeaders(""),
	)
	if err != nil {
		return nil, fmt.Errorf("request cloud info failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read cloud info response failed: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("cloud info http status=%d, body=%s", resp.StatusCode, body)
	}

	var result CloudInfoResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode cloud info response failed: %w, body=%s", err, body)
	}
	return &result, nil
}

// GetPrizeLogPage 获取奖品记录
func (api *CaiyunAPI) GetPrizeLogPage(pageNumber, pageSize int) (*PrizeLogPageResponse, error) {
	if pageNumber <= 0 {
		pageNumber = 1
	}
	if pageSize <= 0 {
		pageSize = 15
	}

	api.prepareSignInCenterSession(true)

	resp, err := api.client.Get(
		fmt.Sprintf("https://m.mcloud.139.com/ycloud/prizeApi/checkPrize/getUserPrizeLogPageV2?currPage=%d&pageSize=%d", pageNumber, pageSize),
		api.buildReceiveHeaders(""),
	)
	if err != nil {
		return nil, fmt.Errorf("request prize log failed: %w", err)
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("read prize log response failed: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("prize log http status=%d, body=%s", resp.StatusCode, body)
	}

	var result PrizeLogPageResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, fmt.Errorf("decode prize log response failed: %w, body=%s", err, body)
	}
	return &result, nil
}

// GetTaskList 获取旧版任务列表
func (api *CaiyunAPI) GetTaskList(marketName string) (*TaskListResponse, error) {
	if marketName == "" {
		marketName = "sign_in_3"
	}
	if strings.TrimSpace(marketName) == "sign_in_3" {
		api.ensureMarketDeviceID()
	}

	urlStr := fmt.Sprintf("%s/signin/task/taskList?marketname=%s&clientVersion=%s",
		MarketURL, url.QueryEscape(marketName), url.QueryEscape(MarketClientVersion))

	resp, err := api.client.Get(urlStr, api.buildMarketHeaders(nil, ""))
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result TaskListResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetTaskListV2 获取新版任务列表
func (api *CaiyunAPI) GetTaskListV2(group string) (*TaskListResponse, error) {
	api.prepareSignInCenterSession(false)

	urlStr := fmt.Sprintf("%s/signin/task/taskListV2", MobileMarketURL)
	payload := map[string]interface{}{
		"marketname":    "sign_in_3",
		"clientVersion": MarketClientVersion,
		"group":         group,
	}

	resp, err := api.client.Post(urlStr, api.buildReceiveHeaders(""), payload)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result TaskListResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DoTaskPost 注册新版云朵中心任务所需 deviceId。
func (api *CaiyunAPI) DoTaskPost() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(false)

	deviceID := strings.TrimSpace(api.client.GetDeviceID())
	if deviceID == "" {
		api.ensureMarketDeviceID()
		deviceID = strings.TrimSpace(api.client.GetDeviceID())
	}

	resp, err := api.client.Post(
		fmt.Sprintf("%s/signin/page/doTaskPost", MobileMarketURL),
		api.buildReceiveHeaders(""),
		map[string]interface{}{
			"client":   "app",
			"deviceId": deviceID,
		},
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DoTaskWithMarket 执行任务
func (api *CaiyunAPI) DoTaskWithMarket(marketName, key, taskID string) error {
	baseURL := MarketURL
	headers := api.buildMarketHeaders(nil, "")
	if strings.TrimSpace(marketName) == "sign_in_3" {
		api.ensureMarketDeviceID()
		baseURL = MobileMarketURL
		headers = api.buildReceiveHeaders("")
	}

	urlStr := fmt.Sprintf("%s/signin/task/click?key=%s&id=%s",
		baseURL, url.QueryEscape(key), url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, headers)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return err
	}
	if !result.IsSuccess() {
		return fmt.Errorf("执行任务失败：%v - %s", result.Code, result.MessageText())
	}
	return nil
}

// DoTask 执行默认云盘任务
func (api *CaiyunAPI) DoTask(key, taskID string) error {
	return api.DoTaskWithMarket("sign_in_3", key, taskID)
}

// ReceiveTaskRewardForMarket 领取任务奖励
func (api *CaiyunAPI) ReceiveTaskRewardForMarket(marketName, taskID string) error {
	baseURL := MarketURL
	headers := api.buildMarketHeaders(nil, "")
	if strings.TrimSpace(marketName) == "sign_in_3" {
		api.ensureMarketDeviceID()
		baseURL = MobileMarketURL
		headers = api.buildReceiveHeaders("")
	}

	urlStr := fmt.Sprintf("%s/signin/page/receiveTask?taskId=%s",
		baseURL, url.QueryEscape(taskID))

	resp, err := api.client.Get(urlStr, headers)
	if err != nil {
		return err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return err
	}

	var result CaiyunResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return err
	}
	if !result.IsSuccess() {
		return fmt.Errorf("领取奖励失败：%v - %s", result.Code, result.MessageText())
	}
	return nil
}

// ReceiveTaskReward 领取默认云盘任务奖励
func (api *CaiyunAPI) ReceiveTaskReward(taskID string) error {
	return api.ReceiveTaskRewardForMarket("sign_in_3", taskID)
}
