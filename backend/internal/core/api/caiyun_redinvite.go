package api

import (
	"fmt"
	"net/url"
	"strings"
)

// National_Invitingtask（全网红包邀请）。
//
// 链路可通：getInviteCode 生成邀请码 -> 被邀请方 acceptInvite（请求体里的 data
// 是「公钥加密的 时间戳+活动名」）-> 邀请方领 500G/0.99 元或 20G/0.2 元。
//
// 但服务端有一道前置风控：`risk` 回 success:false / body:999 时，前端连
// acceptInvite 都不会发。业务码语义：
//
//	3008 系统识别疑似存在异常行为
//	3009 被邀请方 30 天内登录过移动云盘 APP，不能接受邀请
//	3010 活动太火爆了（风控闸门）
const RedInviteMarketName = "National_Invitingtask"

const redInviteBase = MobileMarketURL + "/redInvite/page"

// RedInviteMonthInfo 本月邀请奖励额度。
type RedInviteMonthInfo struct {
	NewUserCloudNum       int    `json:"newUserCloudNum"`
	OldUserCloudNum       int    `json:"oldUserCloudNum"`
	NewUserRedNum         string `json:"newUserRedNum"`
	OldUserRedNum         string `json:"oldUserRedNum"`
	NonMobileNewUserCloud int    `json:"nonMobileNewUserCloudNum"`
	UserTotal             int    `json:"userTotal"`
	NewUserNum            int    `json:"newUserNum"`
	OldUserNum            int    `json:"oldUserNum"`
	CloudTotal            int    `json:"cloudTotal"`
	RedTotal              string `json:"redTotal"`
}

// RedInviteMonthInfo 查询本月邀请额度与已邀请人数。
func (api *CaiyunAPI) RedInviteMonthInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		redInviteBase+"/statMonthInfo?marketName="+url.QueryEscape(RedInviteMarketName),
		nil,
		nil,
	)
}

// RedInviteRisk 前置风控检查。返回 success=false 时接受邀请必定失败。
func (api *CaiyunAPI) RedInviteRisk() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		redInviteBase+"/risk?marketName="+url.QueryEscape(RedInviteMarketName),
		nil,
		nil,
	)
}

// RedInviteRiskPassed 判断风控接口是否放行（returnCode==0 且 success==true）。
func RedInviteRiskPassed(resp *CaiyunResponse) bool {
	if resp == nil {
		return false
	}
	result, ok := resultMap(resp)
	if ok {
		if code := mapString(result, "returnCode"); code != "" && code != "0" {
			return false
		}
		if _, has := result["success"]; has && !mapBool(result, "success") {
			return false
		}
		if _, has := result["body"]; has && mapInt(result, "body") == 999 {
			return false
		}
		return mapBool(result, "success")
	}
	if resp.ReturnCode != nil && fmt.Sprint(resp.ReturnCode) != "0" {
		return false
	}
	if resp.Code != nil && fmt.Sprint(resp.Code) != "0" {
		return false
	}
	return resp.Success && fmt.Sprint(resp.Body) != "999"
}

// RedInviteGenerateCode 生成一个新的邀请码（result 是纯字符串）。
func (api *CaiyunAPI) RedInviteGenerateCode() (string, error) {
	resp, err := api.marketJSONRequest(
		"POST",
		redInviteBase+"/getInviteCode",
		map[string]string{"marketName": RedInviteMarketName, "op": "getInviteCode"},
		nil,
	)
	if err != nil {
		return "", err
	}
	code := resultString(resp)
	if code == "" {
		return "", fmt.Errorf("生成邀请码失败: code=%v %s", resp.Code, resp.MessageText())
	}
	return code, nil
}

// RedInviteState 查询邀请任务进度。
func (api *CaiyunAPI) RedInviteState() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"POST",
		redInviteBase+"/getInviteState",
		map[string]string{"marketName": RedInviteMarketName, "op": "getInviteState"},
		nil,
	)
}

// RedInviteAccept 接受邀请。邀请码由被邀请方账号提交，data 字段是加密时间戳。
func (api *CaiyunAPI) RedInviteAccept(inviteCode string) (*CaiyunResponse, error) {
	code := strings.TrimSpace(inviteCode)
	if code == "" {
		return nil, fmt.Errorf("邀请码为空")
	}
	encrypted, err := EncryptActivityPayload(map[string]interface{}{
		"marketName":  RedInviteMarketName,
		"encryptTime": api.ActivityEncryptTimeMillis(),
	})
	if err != nil {
		return nil, err
	}
	return api.marketJSONRequest(
		"POST",
		redInviteBase+"/acceptInvite",
		map[string]interface{}{
			"op":           "inviteNewUser",
			"month":        true,
			"inviteNumber": code,
			"data":         encrypted,
		},
		nil,
	)
}

// ParseRedInviteMonthInfo 解析本月邀请额度。
func ParseRedInviteMonthInfo(resp *CaiyunResponse) (RedInviteMonthInfo, bool) {
	result, ok := resultMap(resp)
	if !ok {
		return RedInviteMonthInfo{}, false
	}
	return RedInviteMonthInfo{
		NewUserCloudNum:       mapInt(result, "newUserCloudNum"),
		OldUserCloudNum:       mapInt(result, "oldUserCloudNum"),
		NewUserRedNum:         mapString(result, "newUserRedNum"),
		OldUserRedNum:         mapString(result, "oldUserRedNum"),
		NonMobileNewUserCloud: mapInt(result, "nonMobileNewUserCloudNum"),
		UserTotal:             mapInt(result, "userTotal"),
		NewUserNum:            mapInt(result, "newUserNum"),
		OldUserNum:            mapInt(result, "oldUserNum"),
		CloudTotal:            mapInt(result, "cloudTotal"),
		RedTotal:              mapString(result, "redTotal"),
	}, true
}
