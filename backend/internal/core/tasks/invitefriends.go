package tasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/url"
	"strings"
	"time"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// InviteFriendsTask 邀请好友看电影任务（分享有奖）
type InviteFriendsTask struct {
	client  *http.Client
	logger  *logger.Logger
	phone   string
	pending bool
	message string
}

const inviteShareAttempts = 3

// inviteShareRetryDelay 为分享上报重试的退避基数（毫秒），测试可调小。
var inviteShareRetryDelay = 1000

// NewInviteFriendsTask 创建邀请好友看电影任务
func NewInviteFriendsTask(client *http.Client, log *logger.Logger) *InviteFriendsTask {
	return &InviteFriendsTask{
		client: client,
		logger: log,
	}
}

// SetPhone 设置手机号（用于分享埋点）
func (t *InviteFriendsTask) SetPhone(phone string) *InviteFriendsTask {
	t.phone = phone
	return t
}

// Run 执行邀请好友看电影任务
func (t *InviteFriendsTask) Run() error {
	t.pending, t.message = false, ""
	t.logger.Start("------【邀请好友看电影】------")
	remaining, err := t.getRemainingShareCount()
	if err != nil {
		return err
	}
	if remaining <= 0 {
		t.message = "本月20次分享奖励已确认；待领云朵由领取任务统一处理"
		return nil
	}
	confirmed := 0
	for attempt := 0; attempt < 20 && remaining > 0; attempt++ {
		if err := t.shareToDatacenter(); err != nil {
			// 上报失败多为容器内 DNS 抖动或网关限流；保留待重试状态，
			// 不把整批任务判失败（下次运行会继续补分享）。
			t.pending = true
			t.message = fmt.Sprintf("分享事件上报未完成（%v）；本月剩余%d次，待下次重试", err, remaining)
			t.logger.Warn(t.message)
			return nil
		}
		utils.Sleep(1000)
		after, err := t.getRemainingShareCount()
		if err != nil {
			return fmt.Errorf("分享后复查奖励流水: %w", err)
		}
		if after >= remaining {
			t.pending = true
			break
		}
		confirmed += remaining - after
		remaining = after
	}
	t.message = fmt.Sprintf("服务端新确认分享奖励%d次，本月剩余%d次；待领云朵由领取任务统一处理", confirmed, remaining)
	if t.pending {
		t.message += "；本次分享已提交，等待服务端奖励记录"
	}
	return nil
}

func (t *InviteFriendsTask) Pending() bool   { return t.pending }
func (t *InviteFriendsTask) Message() string { return t.message }

// getRemainingShareCount 获取本月剩余分享次数（每月20次）
func (t *InviteFriendsTask) getRemainingShareCount() (int, error) {
	records, err := t.getCloudRecord()
	if err != nil {
		return 0, err
	}

	// 统计本月分享次数
	zone := time.FixedZone("CST", 8*60*60)
	now := time.Now().In(zone)
	count := 0
	for _, record := range records {
		if record.Mark == "fxnrplus5" {
			recordTime, err := time.ParseInLocation("2006-01-02 15:04:05", record.UpdateTime, zone)
			if err != nil {
				continue
			}
			if recordTime.Year() == now.Year() && recordTime.Month() == now.Month() {
				count++
			}
		}
	}

	return max(0, 20-count), nil
}

// CloudRecord 云朵记录
type CloudRecord struct {
	Mark       string `json:"mark"`
	UpdateTime string `json:"updatetime"`
}

// getCloudRecord 获取云朵记录
func (t *InviteFriendsTask) getCloudRecord() ([]CloudRecord, error) {
	// 查询云朵记录应使用 cloudRecord 接口，不能复用领取接口。
	url := "https://mrp.mcloud.139.com/market/signin/public/cloudRecord?type=1&pageNumber=1&pageSize=200"
	headers := map[string]string{
		"Content-Type": "application/json",
	}

	resp, err := t.client.Get(url, headers)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}

	responseBody, err := t.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("查询分享奖励流水 HTTP %d", resp.StatusCode)
	}

	var result struct {
		Code    interface{} `json:"code"`
		Message string      `json:"msg"`
		Result  struct {
			Records []CloudRecord `json:"records"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if fmt.Sprint(result.Code) != "0" || result.Result.Records == nil {
		return nil, fmt.Errorf("查询分享奖励流水失败: code=%v %s", result.Code, result.Message)
	}

	return result.Result.Records, nil
}

// shareToDatacenter 分享到数据中心（模拟分享行为）
func (t *InviteFriendsTask) shareToDatacenter() error {
	// 构造分享事件数据
	eventData := t.buildShareEventData()
	encodedData := base64.StdEncoding.EncodeToString([]byte(eventData))

	// 计算 CRC（与 mjs gf 函数一致）
	crc := t.calcCRC(encodedData)

	// 使用 application/x-www-form-urlencoded 格式（与 mjs 一致）
	headers := map[string]string{
		"Content-Type": "application/x-www-form-urlencoded",
		"platform":     "h5",
	}

	body := url.Values{"data": {encodedData}, "ext": {fmt.Sprintf("crc=%d", crc)}}.Encode()

	// 分享事件是不可撤销的埋点上报，重试只会重复一次事件，不会重复领奖；
	// 网络/DNS 抖动在容器里很常见，这里做退避重试。
	const attempts = inviteShareAttempts
	var lastErr error
	for attempt := 1; attempt <= attempts; attempt++ {
		resp, err := t.client.Post("https://datacenter.mail.10086.cn/datacenter/", headers, body)
		if err != nil {
			lastErr = err
			if attempt < attempts {
				utils.Sleep(attempt * inviteShareRetryDelay)
				continue
			}
			break
		}
		if _, err := t.client.ReadResponseBody(resp); err != nil {
			return err
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("分享有奖 HTTP %d", resp.StatusCode)
		}
		return nil
	}
	return fmt.Errorf("分享有奖请求失败（已重试%d次）: %w", attempts, lastErr)
}

// calcCRC 计算 CRC（对应 mjs gf 函数）
func (t *InviteFriendsTask) calcCRC(s string) int32 {
	var e int32 = 0
	for i := 0; i < len(s); i++ {
		ch := int32(s[i])
		e = (e << 5) - e + ch
		e = e & e // 模拟 JS 的 32 位整数溢出
	}
	return e
}

// buildShareEventData 构建分享事件数据
func (t *InviteFriendsTask) buildShareEventData() string {
	now := time.Now()
	phone := t.phone
	if phone == "" {
		phone = "anonymous"
	}

	event := map[string]interface{}{
		"traceId":              rand.Int63(),
		"tackTime":             now.UnixMilli(),
		"distinctId":           utils.RandomString(15),
		"eventName":            "discoverNewVersion.Page.Share.QQ",
		"event":                "$manual",
		"flushTime":            now.UnixMilli(),
		"model":                "",
		"osVersion":            "",
		"appVersion":           "",
		"manufacture":          "",
		"screenHeight":         895,
		"os":                   "Android",
		"screenWidth":          393,
		"lib":                  "js",
		"libVersion":           "1.17.2",
		"networkType":          "",
		"resumeFromBackground": "",
		"screenName":           "",
		"title":                "【精选】一站式资源宝库",
		"eventDuration":        "",
		"elementPosition":      "",
		"elementId":            "",
		"elementContent":       "",
		"elementType":          "",
		"downloadChannel":      "",
		"crashedReason":        "",
		"phoneNumber":          phone,
		"storageTime":          "",
		"channel":              "",
		"activityName":         "",
		"platform":             "h5",
		"sdkVersion":           "1.0.1",
		"elementSelector":      "",
		"referrer":             "",
		"scene":                "",
		"latestScene":          "",
		"source":               "content-open",
		"urlPath":              "",
		"IP":                   "",
		"url":                  "https://h.139.com/content/discoverNewVersion?" + url.Values{"columnId": {"20"}, "token": {strings.TrimSpace(t.client.GetSSOToken())}, "targetSourceId": {"001005"}}.Encode(),
		"elementName":          "",
		"browser":              "Chrome WebView",
		"elementTargetUrl":     "",
		"referrerHost":         "",
		"browerVersion":        "122.0.6261.106",
		"latitude":             "",
		"pageDuration":         "",
		"longtitude":           "",
		"urlQuery":             "",
		"shareDepth":           "",
		"arriveTimeStamp":      "",
		"spare":                map[string]string{"mobile": phone, "channel": ""},
		"public":               "",
		"province":             "",
		"city":                 "",
		"carrier":              "",
	}

	data, _ := json.Marshal(event)
	return string(data)
}
