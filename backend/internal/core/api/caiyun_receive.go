package api

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

func (api *CaiyunAPI) GetCloudRecord(pageNumber, pageSize, recordType int) (*CaiyunResponse, error) {
	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/public/cloudRecord?type=%d&pageNumber=%d&pageSize=%d",
			MarketURL, recordType, pageNumber, pageSize),
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

// GetReceiveSummary 获取领取云朵后的摘要信息
func (api *CaiyunAPI) GetReceiveSummary() (*CaiyunResponse, error) {
	cloudInfo, err := api.GetCloudInfo()
	if err != nil {
		return nil, fmt.Errorf("get cloud info failed: %w", err)
	}
	if cloudInfo == nil {
		return nil, fmt.Errorf("get cloud info failed: empty response")
	}
	if !cloudInfo.IsSuccess() {
		return &CaiyunResponse{Code: -1, Msg: cloudInfo.MessageText(), Message: cloudInfo.MessageText()}, nil
	}

	prizeResp, err := api.GetPrizeLogPage(1, 15)
	if err != nil {
		return nil, fmt.Errorf("get prize log failed: %w", err)
	}
	if prizeResp == nil {
		return nil, fmt.Errorf("get prize log failed: empty response")
	}
	if !prizeResp.IsSuccess() {
		return &CaiyunResponse{Code: -1, Msg: prizeResp.MessageText(), Message: prizeResp.MessageText()}, nil
	}

	pendingPrizeNames := prizeResp.PendingPrizeNames()
	result := map[string]interface{}{
		"todaySignIn":       cloudInfo.Result.TodaySigned(),
		"total":             cloudInfo.Result.Total,
		"toReceive":         cloudInfo.Result.ToReceive,
		"nextMonthGet":      cloudInfo.Result.NextMonthGet,
		"pendingPrizeNames": pendingPrizeNames,
		"pendingPrizeCount": len(pendingPrizeNames),
	}

	messageParts := []string{fmt.Sprintf("当前云朵%d", cloudInfo.Result.Total)}
	if len(pendingPrizeNames) > 0 {
		messageParts = append(messageParts, fmt.Sprintf("待领奖品%d项", len(pendingPrizeNames)))
	}

	return &CaiyunResponse{
		Code:    0,
		Msg:     strings.Join(messageParts, "，"),
		Success: true,
		Result:  result,
	}, nil
}

// ReceivePendingCloudRewards 领取待领取云朵
func (api *CaiyunAPI) ReceivePendingCloudRewards() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/receiveV2?client=app", MobileMarketURL),
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

// Receive 领取云朵
func (api *CaiyunAPI) Receive() (*CaiyunResponse, error) {
	claimResp, err := api.ReceivePendingCloudRewards()
	if err != nil {
		return nil, err
	}
	if claimResp == nil {
		return nil, fmt.Errorf("receive pending rewards failed: empty response")
	}
	if !claimResp.IsSuccess() {
		return claimResp, nil
	}

	summaryResp, err := api.GetReceiveSummary()
	if err != nil {
		return nil, err
	}
	if summaryResp == nil {
		return nil, fmt.Errorf("get receive summary failed: empty response")
	}
	if !summaryResp.IsSuccess() {
		return summaryResp, nil
	}

	if payload, ok := summaryResp.Result.(map[string]interface{}); ok {
		payload["receiveMessage"] = claimResp.MessageText()
	}
	if msg := summaryResp.MessageText(); msg != "" {
		summaryResp.Msg = msg
		if claimMsg := claimResp.MessageText(); claimMsg != "" && !strings.EqualFold(claimMsg, "success") {
			summaryResp.Msg = fmt.Sprintf("%s，%s", claimMsg, msg)
		}
	}
	return summaryResp, nil
}

// GetTaskExpansion 获取备份翻倍奖励信息
func (api *CaiyunAPI) GetTaskExpansion() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(false)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/taskExpansion", MobileMarketURL),
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

// ReceiveTaskExpansion 领取翻倍奖励
func (api *CaiyunAPI) ReceiveTaskExpansion(acceptDate string) (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)

	resp, err := api.client.Get(
		fmt.Sprintf("%s/signin/page/receiveTaskExpansion?acceptDate=%s",
			MobileMarketURL, url.QueryEscape(acceptDate)),
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

// NoteAuthResult 云笔记鉴权结果
