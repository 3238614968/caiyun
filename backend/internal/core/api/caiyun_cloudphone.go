package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/utils"
)

const (
	cloudPhoneBase    = "https://cpactiv.buy.139.com/cloudphone-market"
	cloudPhoneSource  = "001216"
	cloudPhoneVersion = "SYS_CONFIG_Y"
	cloudPhoneAppID   = "12345681"
	cloudPhoneSalt    = "e10adc3949ba59abbe56e057f20f883e"
	cloudPhoneOrigin  = "https://cpactiv.buy.139.com/#/redEnvelopeParty/home?loginType=0&channelSrc=red-mcloudapp-001&enableShare=1&isThrApp=true"

	// cloud.139.com 新链（2026-11 实测旧链全会话 401 时的兜底）：
	// 登录走 /uniphone 的 h5 签名（appId/salt 与 market 不同），
	// market 业务路径兜底仍用 market 签名（两边 appId/salt 相同）。
	cloud139Base          = "https://cloud.139.com"
	cloudPhoneH5AppID     = "3099801"
	cloudPhoneH5Salt      = "esdkabncgfsfkmlondn"
	cloudPhoneH5ClientID  = "X3qUAu6hA1yO6CegzcMSrQJknuZ7aEs1"
	cloudPhoneH5ClientKey = "AqXZ3iQtf713yN70ydXRZkCkxdeGo3jo"
)

type CloudPhoneTask struct {
	ID          json.Number `json:"id"`
	TaskCode    string      `json:"taskCode"`
	TaskName    string      `json:"taskName"`
	PrizeAmount json.Number `json:"prizeAmount"`
}

type CloudPhoneTaskList struct {
	Novice  []CloudPhoneTask `json:"configTaskNoviceList"`
	Daily   []CloudPhoneTask `json:"configTaskDailyList"`
	Monthly []CloudPhoneTask `json:"configTaskMonthlyList"`
	Buy     []CloudPhoneTask `json:"configTaskBuyList"`
	Install []CloudPhoneTask `json:"configTaskInstallList"`
}

func (l CloudPhoneTaskList) Tasks() []CloudPhoneTask {
	var tasks []CloudPhoneTask
	for _, group := range [][]CloudPhoneTask{l.Novice, l.Daily, l.Monthly, l.Buy, l.Install} {
		tasks = append(tasks, group...)
	}
	return tasks
}

// CloudPhoneAction 的 status=1 只表示登记成功，不能用作云币到账凭据。
type CloudPhoneAction struct {
	Status    json.Number `json:"status"`
	ErrorCode string      `json:"errorCode"`
	ErrorMsg  string      `json:"errorMsg"`
}

func cloudPhoneHeaders(token string) map[string]string {
	now := time.Now()
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	rid := now.In(time.FixedZone("CST", 8*3600)).Format("20060102150405") + ts + utils.RandomString(8)
	return map[string]string{
		"Content-Type": "application/json;charset=UTF-8", "Accept": "application/json, text/plain, */*",
		"User-Agent": MarketUserAgent, "appId": cloudPhoneAppID, "requestId": rid, "timestamp": ts,
		"sign": strings.ToLower(utils.MD5(rid + cloudPhoneAppID + token + cloudPhoneSalt + ts)), "token": token,
		"x-DeviceInfo": "|h5|1.0.0|v1.0.0||" + MarketUserAgent + "|" + ts + "|||1080X2400|zh||",
		"x-origin":     cloudPhoneOrigin, "x-channelSrc": "red-mcloudapp-001",
		"Origin": "https://cpactiv.buy.139.com", "Referer": cloudPhoneOrigin,
	}
}

// cloud139Headers 是 cloud.139.com /uniphone 登录链的 h5 签名头。
// sign 形状与 market 相同（md5(requestId+appId+token+salt+timestamp)），
// 但 appId/salt 换成 h5 专用值；登录前 token 为空串。
func cloud139Headers(token string) map[string]string {
	now := time.Now()
	ts := strconv.FormatInt(now.UnixMilli(), 10)
	rid := now.In(time.FixedZone("CST", 8*3600)).Format("20060102150405") + ts + utils.RandomString(8)
	return map[string]string{
		"Content-Type": "application/json;charset=UTF-8", "Accept": "application/json, text/plain, */*",
		"User-Agent": MarketUserAgent, "appId": cloudPhoneH5AppID, "requestId": rid, "timestamp": ts,
		"sign": strings.ToLower(utils.MD5(rid + cloudPhoneH5AppID + token + cloudPhoneH5Salt + ts)), "token": token,
		"platform": "h5", "x-channelSrc": "02040",
		"x-kpcc-clientid": cloudPhoneH5ClientID, "x-kpcc-clientkey": cloudPhoneH5ClientKey,
	}
}

// cloudPhoneAuthStatus 识别会话被拒的 401 族状态码（旧链实测全会话失效）。
func cloudPhoneAuthStatus(status string) bool {
	switch status {
	case "401", "4001024", "4001025", "500022":
		return true
	}
	return false
}

// cloudPhoneEnvelope 请求 base+path 并解析 {header:{status},data} 信封，
// 返回业务 status 供调用方判断是否需要换链兜底。
func (api *CaiyunAPI) cloudPhoneEnvelope(base, path string, payload interface{}, headers map[string]string, target interface{}) (string, error) {
	body, err := api.marketRawRequest("POST", base+path, payload, headers)
	if err != nil {
		return "", err
	}
	var envelope struct {
		Header struct {
			Status json.Number `json:"status"`
			ErrMsg string      `json:"errMsg"`
		} `json:"header"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		return "", fmt.Errorf("解析云手机响应失败: %w", err)
	}
	status := envelope.Header.Status.String()
	if status != "200" {
		return status, fmt.Errorf("云手机接口 %s: status=%s %s", path, status, envelope.Header.ErrMsg)
	}
	if len(envelope.Data) == 0 || string(envelope.Data) == "null" {
		return status, fmt.Errorf("云手机接口 %s 缺少 data", path)
	}
	if err := json.Unmarshal(envelope.Data, target); err != nil {
		return status, fmt.Errorf("解析云手机接口 %s 数据失败: %w", path, err)
	}
	return status, nil
}

func (api *CaiyunAPI) cloudPhoneCall(token, path string, payload, target interface{}) error {
	status, err := api.cloudPhoneEnvelope(cloudPhoneBase, path, payload, cloudPhoneHeaders(token), target)
	if err == nil || !cloudPhoneAuthStatus(status) {
		return err
	}
	// 旧链会话被拒：market 签名 appId/salt 两边相同，同路径换 cloud.139.com 重试。
	_, err = api.cloudPhoneEnvelope(cloud139Base+"/cloudphone-market", path, payload, cloudPhoneHeaders(token), target)
	return err
}

// cloudPhoneSpecToken 换取云手机专属 SSO（toSourceId=001216，单次消费）。
func (api *CaiyunAPI) cloudPhoneSpecToken(phone string) (string, error) {
	body, err := api.marketRawRequest("POST",
		"https://orches.yun.139.com/orchestration/auth-rebuild/token/v1.0/querySpecToken",
		map[string]string{"account": phone, "toSourceId": cloudPhoneSource}, nil)
	if err != nil {
		return "", err
	}
	var sso struct {
		Data struct {
			Token string `json:"token"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &sso); err != nil || strings.TrimSpace(sso.Data.Token) == "" {
		return "", fmt.Errorf("获取云手机专属 SSO 失败")
	}
	return sso.Data.Token, nil
}

// CloudPhoneLogin 必须重新换取云手机专属 SSO；云盘主 SSO 不能复用。
// 旧链 tokenValidate 被拒（401 族）或未返回 token 时，SSO 已被单次消费，
// 需重新 querySpecToken 并走 cloud.139.com/uniphone 的 h5 新链兜底。
func (api *CaiyunAPI) CloudPhoneLogin(phone string) (string, error) {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		return "", fmt.Errorf("云手机登录缺少手机号")
	}
	sso, err := api.cloudPhoneSpecToken(phone)
	if err != nil {
		return "", err
	}
	var session struct {
		Token string `json:"token"`
	}
	// deviceId/loginConfig 会切换到蜂窝取号登录，故请求体只含这三个键。
	status, err := api.cloudPhoneEnvelope(cloudPhoneBase, "/user/tokenValidate", map[string]interface{}{
		"version": "1.0", "pintype": 13, "token": sso,
	}, cloudPhoneHeaders(""), &session)
	if err == nil && strings.TrimSpace(session.Token) != "" {
		return session.Token, nil
	}
	if err != nil && !cloudPhoneAuthStatus(status) {
		return "", err
	}
	fresh, err := api.cloudPhoneSpecToken(phone)
	if err != nil {
		return "", err
	}
	session.Token = ""
	if _, err := api.cloudPhoneEnvelope(cloud139Base+"/uniphone", "/user/tokenValidate", map[string]interface{}{
		"version": "1.2", "pintype": 5, "token": fresh,
	}, cloud139Headers(""), &session); err != nil {
		return "", err
	}
	if strings.TrimSpace(session.Token) == "" {
		return "", fmt.Errorf("云手机登录未返回会话 token")
	}
	return session.Token, nil
}

func (api *CaiyunAPI) CloudPhoneBalance(token string) (int64, error) {
	var data struct {
		Info struct {
			CanAmount json.Number `json:"canAmount"`
		} `json:"info"`
	}
	if err := api.cloudPhoneCall(token, "/redpacket/userAccountInfo", map[string]interface{}{}, &data); err != nil {
		return 0, err
	}
	balance, err := data.Info.CanAmount.Int64()
	if err != nil || balance < 0 {
		return 0, fmt.Errorf("云手机账户缺少有效云币余额")
	}
	return balance, nil
}

func (api *CaiyunAPI) CloudPhoneTasks(token string) (*CloudPhoneTaskList, error) {
	var tasks CloudPhoneTaskList
	err := api.cloudPhoneCall(token, "/redpacket/configTaskList", map[string]string{"version": cloudPhoneVersion}, &tasks)
	return &tasks, err
}

func (api *CaiyunAPI) CloudPhoneSign(token string) (*CloudPhoneAction, error) {
	var result CloudPhoneAction
	err := api.cloudPhoneCall(token, "/redpacket/userSign", map[string]interface{}{
		"version": cloudPhoneVersion, "platformType": 1,
	}, &result)
	return &result, err
}

func (api *CaiyunAPI) CloudPhoneCompleteTask(token string, id json.Number) (*CloudPhoneAction, error) {
	if value, err := id.Int64(); err != nil || value <= 0 {
		return nil, fmt.Errorf("云手机任务 ID 无效")
	}
	var result CloudPhoneAction
	err := api.cloudPhoneCall(token, "/redpacket/userCompleteTask", map[string]interface{}{
		"version": cloudPhoneVersion, "platformType": 1, "taskId": id,
	}, &result)
	return &result, err
}
