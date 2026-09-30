package api

import (
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
				claimErr = fmt.Errorf("领取云朵业务失败: %s", message)
			}
			issues = append(issues, claimErr)
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
