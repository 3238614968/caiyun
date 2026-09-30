package api

import (
	"fmt"
	"strings"
)

// 本文件补齐「通知 / 开关」类前置动作。这些动作本身不直接发奖，但决定
// 云朵中心里若干连续天数任务的进度是否被服务端记账：
//
//	reportAppNoticeStatus -> 551 连续31天开启云盘APP通知、1021
//	openSmsSwitch         -> 1079 邮箱短信通知连续天数
//
// 旧实现只读取了 msgPushOn/task/status 与 openEmailsms/reward（领奖），
// 没有做过「上报开关已开」这一步，因此这几项长期停在初始进度。

// ReportAppNoticeStatus 上报「APP 通知已开启」。status=1 表示已开启。
func (api *CaiyunAPI) ReportAppNoticeStatus(status int) (*CaiyunResponse, error) {
	if status != 0 && status != 1 {
		return nil, fmt.Errorf("无效的 APP 通知状态: %d", status)
	}
	return api.marketJSONRequest(
		"GET",
		fmt.Sprintf("%s/signin/page/reportAppNoticeStatus?status=%d", MobileMarketURL, status),
		nil,
		api.buildReceiveHeaders(""),
	)
}

// GetEmailSmsSwitchStatus 查询邮箱短信通知开关是否已开启。
func (api *CaiyunAPI) GetEmailSmsSwitchStatus() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		MobileMarketURL+"/signin/emailStatus/currEmailStatus",
		nil,
		api.signInMarketHeaders("newsign_139mail"),
	)
}

// EmailSmsSwitchEnabled 解析 currEmailStatus 的返回值。服务端只回 true/false。
func EmailSmsSwitchEnabled(resp *CaiyunResponse) (bool, bool) {
	if resp == nil {
		return false, false
	}
	switch value := resp.Result.(type) {
	case bool:
		return value, true
	case string:
		text := strings.TrimSpace(strings.ToLower(value))
		if text == "true" || text == "false" {
			return text == "true", true
		}
	}
	return false, false
}

// OpenEmailSmsSwitch 开启「邮箱短信通知」开关（无请求体）。
func (api *CaiyunAPI) OpenEmailSmsSwitch() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"POST",
		MobileMarketURL+"/openemailsms-service/openEmailsms/openSmsSwitch",
		nil,
		api.signInMarketHeaders("newsign_139mail"),
	)
}

// GetCloudNum 读取当前云朵数（只读）。
func (api *CaiyunAPI) GetCloudNum() (*CaiyunResponse, error) {
	return api.marketJSONRequest(
		"GET",
		MobileMarketURL+"/signin/page/getCloudNum",
		nil,
		api.buildReceiveHeaders(""),
	)
}
