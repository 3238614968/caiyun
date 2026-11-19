package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const maxCloudClaimsPerRun = 200

// receivePendingCloudRewardsV3 follows the captured App flow: infoV3.recordId
// becomes receiveV3.cloudId, followed by an infoV3 confirmation in the same
// account session. Aggregate/future entries without an eligible ID are skipped.
func (api *CaiyunAPI) receivePendingCloudRewardsV3() (*CaiyunResponse, error) {
	api.prepareSignInCenterSession(true)
	if err := api.client.EnsureShumeiDeviceIDWithUserAgent(context.Background(), MarketUserAgent); err != nil {
		return nil, fmt.Errorf("领取云朵设备认证失败，未提交领取: %w", err)
	}
	deviceID := strings.TrimSpace(api.client.GetDeviceID())
	if deviceID == "" {
		return nil, fmt.Errorf("领取云朵缺少设备标识")
	}
	before, err := api.getCloudInfoPrepared()
	if err != nil {
		return nil, err
	}
	if before == nil || !before.IsSuccess() || before.Result.ReceiveList == nil {
		return nil, fmt.Errorf("领取前云朵信息查询失败")
	}
	current := before
	received, claimed := 0, 0
	var issues []error
	seen := make(map[int64]bool)
	for _, item := range before.Result.ReceiveList {
		id := item.RewardID()
		if item.CloudType != 0 || id <= 0 || seen[id] {
			continue
		}
		seen[id] = true
		if len(seen) > maxCloudClaimsPerRun {
			issues = append(issues, fmt.Errorf("待领记录超过单次处理上限"))
			break
		}
		if !cloudRewardStillPending(current.Result.ReceiveList, id) {
			continue
		}
		headers := api.buildReceiveHeaders("")
		headers["isDeviceId"] = "true"
		headers["Content-Type"] = "application/json;charset=UTF-8"
		ack, claimErr := api.marketJSONRequest("POST", MobileMarketURL+"/signin/page/receiveV3", map[string]interface{}{
			"client": "app", "cloudId": id, "cloudType": item.CloudType, "deviceId": deviceID,
		}, headers)
		if claimErr != nil || ack == nil || !ack.IsSuccess() {
			if claimErr == nil {
				message := "响应为空"
				if ack != nil {
					message = ack.MessageText()
				}
				var code interface{}
				if ack != nil {
					code = ack.Code
				}
				claimErr = fmt.Errorf("领取云朵业务失败: code=%v %s", code, message)
			}
			issues = append(issues, fmt.Errorf("云朵记录%d: %w", id, claimErr))
			// A mutation with an ambiguous response is not blindly retried.
			continue
		}
		result, valid := resultMap(ack)
		amount, amountErr := strconv.Atoi(fmt.Sprint(result["receive"]))
		ackTotal, totalErr := strconv.Atoi(fmt.Sprint(result["total"]))
		if !valid || amountErr != nil || totalErr != nil || amount < 0 {
			issues = append(issues, fmt.Errorf("领取响应缺少有效的实际到账数量"))
			continue
		}
		previousTotal := current.Result.Total
		after, checkErr := api.getCloudInfoPrepared()
		if checkErr != nil || after == nil || !after.IsSuccess() || after.Result.ReceiveList == nil {
			issues = append(issues, fmt.Errorf("领取后状态确认失败"))
			break
		}
		current = after
		if cloudRewardStillPending(after.Result.ReceiveList, id) {
			issues = append(issues, fmt.Errorf("领取接口返回成功但记录仍待领取"))
			continue
		}
		if ackTotal < previousTotal+amount || after.Result.Total < ackTotal {
			issues = append(issues, fmt.Errorf("领取回包与余额变化不一致，到账结果待确认"))
			continue
		}
		received += amount
		claimed++
	}
	// Even a rejected/ambiguous acknowledgement may race with another claim.
	// Re-read pending state before reporting the amount left from an old snapshot.
	if len(issues) > 0 {
		if latest, checkErr := api.getCloudInfoPrepared(); checkErr == nil && latest != nil && latest.IsSuccess() && latest.Result.ReceiveList != nil {
			current = latest
		}
	}
	if current.Result.ToReceive > 0 {
		issues = append(issues, fmt.Errorf("仍有%d云朵待领取", current.Result.ToReceive))
	} else {
		for _, item := range current.Result.ReceiveList {
			if item.CloudType == 0 && item.RewardID() > 0 {
				issues = append(issues, fmt.Errorf("复查列表仍存在可领取记录"))
				break
			}
		}
	}
	message := fmt.Sprintf("已确认领取%d项，实际到账%d云朵，当前云朵%d，待领%d", claimed, received, current.Result.Total, current.Result.ToReceive)
	if len(seen) == 0 && current.Result.ToReceive == 0 {
		message = fmt.Sprintf("当前没有可领取云朵，余额%d", current.Result.Total)
	}
	response := &CaiyunResponse{Code: 0, Msg: message, Success: len(issues) == 0, Result: map[string]interface{}{
		"receivedCloud": received, "claimedCount": claimed, "total": current.Result.Total,
		"toReceive": current.Result.ToReceive,
	}}
	if len(issues) > 0 {
		response.Code = -1
	}
	return response, errors.Join(issues...)
}

func cloudRewardStillPending(items []CloudReceiveItem, id int64) bool {
	for _, item := range items {
		if item.CloudType == 0 && item.RewardID() == id {
			return true
		}
	}
	return false
}

func (api *CaiyunAPI) getGPCloudInfo() (*CloudInfoResponse, error) {
	resp, err := api.client.Get(GPMarketURL+"/signin/page/infoV3?client=app", api.buildReceiveHeaders(""))
	if err != nil {
		return nil, fmt.Errorf("gp云朵信息查询失败: %w", err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取gp云朵信息响应失败: %w", err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("gp云朵信息 HTTP %d", resp.StatusCode)
	}
	var info CloudInfoResponse
	if err := json.Unmarshal([]byte(body), &info); err != nil {
		return nil, fmt.Errorf("解析gp云朵信息失败: %w", err)
	}
	if !isSuccessIntCode(info.Code) {
		return nil, fmt.Errorf("gp云朵信息查询失败: code=%s %s", info.CodeText(), info.MessageText())
	}
	if info.Result.ReceiveList == nil {
		return nil, fmt.Errorf("gp云朵信息缺少待领列表")
	}
	return &info, nil
}

// ReceivePendingCloudRewardsGP 探测并领取 gp 域待领云朵。
// 614/615 归因 App 验证；成功回包须经同一 gp 会话复查记录与余额后才记为到账。
func (api *CaiyunAPI) ReceivePendingCloudRewardsGP() (*CaiyunResponse, error) {
	info, err := api.getGPCloudInfo()
	if err != nil {
		return nil, err
	}
	rewards, pending := pendingCloudRewards(info.Result.ReceiveList)
	items := len(rewards)
	result := map[string]interface{}{
		"pendingCloud": pending, "pendingCount": items,
		"receivedCloud": 0, "claimedCount": 0,
		"total": info.Result.Total, "toReceive": info.Result.ToReceive,
	}
	if items == 0 {
		return &CaiyunResponse{Code: 0, Success: true, Result: result,
			Msg: fmt.Sprintf("gp当前没有可领取云朵，余额%d", info.Result.Total)}, nil
	}
	headers := api.buildReceiveHeaders("")
	headers["Content-Type"] = "application/json;charset=UTF-8"
	ack, err := api.marketJSONRequest("POST", GPMarketURL+"/signin/page/receiveV3", map[string]interface{}{
		"client": "app", "cloudId": "", "cloudType": 4,
	}, headers)
	if err != nil {
		return nil, fmt.Errorf("gp领取云朵请求失败: %w", err)
	}
	if ack == nil {
		return nil, fmt.Errorf("gp领取云朵响应为空")
	}
	switch fmt.Sprint(ack.Code) {
	case "614", "615":
		result["appRequired"] = true
		return &CaiyunResponse{Code: 0, Success: true, Result: result,
			Msg: fmt.Sprintf("gp待领云朵%d（%d项），需App领取（code=%v）", pending, items, ack.Code)}, nil
	}
	if fmt.Sprint(ack.Code) != "0" {
		return nil, fmt.Errorf("gp领取云朵业务失败: code=%v %s", ack.Code, ack.MessageText())
	}
	unconfirmed := func(cause error) (*CaiyunResponse, error) {
		return &CaiyunResponse{Code: -1, Result: result,
			Msg: fmt.Sprintf("gp领取云朵结果待确认（已确认%d云朵、%d项）：%v", result["receivedCloud"], result["claimedCount"], cause)}, cause
	}
	payload, valid := resultMap(ack)
	amount, amountErr := strconv.Atoi(fmt.Sprint(payload["receive"]))
	ackTotal, totalErr := strconv.Atoi(fmt.Sprint(payload["total"]))
	if !valid || amountErr != nil || totalErr != nil || amount < 0 || ackTotal < 0 {
		return unconfirmed(fmt.Errorf("gp领取响应缺少有效的到账数量或余额"))
	}
	after, checkErr := api.getGPCloudInfo()
	if checkErr != nil {
		return unconfirmed(fmt.Errorf("gp领取后状态确认失败: %w", checkErr))
	}
	remaining, pending := pendingCloudRewards(after.Result.ReceiveList)
	result["pendingCloud"], result["pendingCount"] = pending, len(remaining)
	result["total"], result["toReceive"] = after.Result.Total, after.Result.ToReceive
	claimed, confirmedCloud := 0, 0
	for id, item := range rewards {
		if _, stillPending := remaining[id]; !stillPending {
			claimed++
			confirmedCloud += item.CloudNum
		}
	}
	if confirmedCloud != amount || (amount > 0 && claimed == 0) {
		return unconfirmed(fmt.Errorf("gp领取回包与待领记录变化不一致"))
	}
	if ackTotal < info.Result.Total+amount || after.Result.Total < ackTotal {
		return unconfirmed(fmt.Errorf("gp领取回包与余额变化不一致"))
	}
	result["receivedCloud"] = amount
	result["claimedCount"] = claimed
	if len(remaining) > 0 || after.Result.ToReceive > 0 {
		return unconfirmed(fmt.Errorf("gp复查仍有云朵待领取"))
	}
	return &CaiyunResponse{Code: 0, Success: true, Result: result,
		Msg: fmt.Sprintf("gp已领取%d云朵（%d项），当前云朵%d", amount, claimed, after.Result.Total)}, nil
}

func pendingCloudRewards(items []CloudReceiveItem) (map[int64]CloudReceiveItem, int) {
	rewards := make(map[int64]CloudReceiveItem)
	pending := 0
	for _, item := range items {
		id := item.RewardID()
		if item.CloudType != 0 || id <= 0 {
			continue
		}
		if _, seen := rewards[id]; seen {
			continue
		}
		rewards[id] = item
		pending += item.CloudNum
	}
	return rewards, pending
}
