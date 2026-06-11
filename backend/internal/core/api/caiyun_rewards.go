package api

import (
	"encoding/json"
	"fmt"
)

// Shake 摇一摇
func (api *CaiyunAPI) Shake() (*ShakeResponse, error) {
	resp, err := api.client.Post(
		fmt.Sprintf("%s/shake-server/shake/shakeIt?flag=1", Market7071URL),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}

	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, err
	}

	var result ShakeResponse
	if err := json.Unmarshal([]byte(body), &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SignInWx 微信签到
func (api *CaiyunAPI) SignInWx() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/followSignInfo?isWx=true", MarketURL),
		nil,
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

// GetDrawInWx 获取微信抽奖信息
func (api *CaiyunAPI) GetDrawInWx() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/drawInfo", MarketURL),
		nil,
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

// WxDraw 微信抽奖
func (api *CaiyunAPI) WxDraw() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/playoffic/draw", MarketURL),
		nil,
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

// GetMsgPushStatus 获取消息推送状态
func (api *CaiyunAPI) GetMsgPushStatus() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/msgPushOn/task/status", MarketURL),
		nil,
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

// ObtainMsgPushOn 领取消息推送奖励
func (api *CaiyunAPI) ObtainMsgPushOn() (*CaiyunResponse, error) {
	resp, err := api.client.Post(
		fmt.Sprintf("%s/msgPushOn/task/obtain", MarketURL),
		nil,
		map[string]interface{}{"type": 2},
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

// GetBackupGift 获取备份好礼状态
func (api *CaiyunAPI) GetBackupGift() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/backupgift/info", Market7071URL),
		nil,
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

// ReceiveBackupGift 领取备份好礼
func (api *CaiyunAPI) ReceiveBackupGift() (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/backupgift/receive", Market7071URL),
		nil,
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

// CloudMultiple 领取新版签到页云朵翻倍奖励。
func (api *CaiyunAPI) CloudMultiple() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/multiple", MobileMarketURL),
		api.buildReceiveHeaders(""),
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

// ReceiveRevivalReward 领取复活卡奖励
func (api *CaiyunAPI) ReceiveRevivalReward() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)

	resp, err := api.client.Post(
		fmt.Sprintf("%s/signin/page/receiveRevivalReward", MobileMarketURL),
		api.buildReceiveHeaders(""),
		map[string]interface{}{},
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

// GetCloudRecord 获取云朵记录
