package tasks

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/core/utils"
)

// InviteFriendsTask 邀请好友看电影任务（分享有奖）
type InviteFriendsTask struct {
	client *http.Client
	logger *logger.Logger
	phone  string
}

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
	t.logger.Start("------【邀请好友看电影】------")
	t.logger.Info("测试中。。。")

	// 获取本月剩余分享次数
	remaining := t.getRemainingShareCount()
	if remaining <= 0 {
		t.logger.Info("本月已分享")
		return nil
	}

	// 尝试首次分享
	t.shareToDatacenter()
	utils.Sleep(1000)

	// 领取云朵
	t.receiveCloud()
	utils.Sleep(1000)

	remaining--

	// 继续分享
	for remaining > 0 {
		remaining--
		t.logger.Debug("邀请好友")
		t.shareToDatacenter()
		utils.Sleep(2000)
	}

	// 最终领取
	t.receiveCloud()

	return nil
}

// getRemainingShareCount 获取本月剩余分享次数（每月20次）
func (t *InviteFriendsTask) getRemainingShareCount() int {
	records, err := t.getCloudRecord()
	if err != nil {
		return 20 // 默认20次
	}

	// 统计本月分享次数
	now := time.Now()
	count := 0
	for _, record := range records {
		if record.Mark == "fxnrplus5" {
			recordTime, err := time.Parse("2006-01-02 15:04:05", record.UpdateTime)
			if err != nil {
				continue
			}
			if recordTime.Year() == now.Year() && recordTime.Month() == now.Month() {
				count++
			}
		}
	}

	return 20 - count
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

	var result struct {
		Code    int    `json:"code"`
		Message string `json:"msg"`
		Result  struct {
			Records []CloudRecord `json:"records"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(responseBody), &result); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	return result.Result.Records, nil
}

// shareToDatacenter 分享到数据中心（模拟分享行为）
func (t *InviteFriendsTask) shareToDatacenter() {
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

	body := fmt.Sprintf("data=%s&ext=crc=%d", encodedData, crc)

	resp, err := t.client.Post("https://datacenter.mail.10086.cn/datacenter/", headers, body)
	if err != nil {
		t.logger.Error("分享有奖异常", err)
		return
	}
	if resp != nil {
		resp.Body.Close()
	}
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
		"url":                  fmt.Sprintf("https://h.139.com/content/discoverNewVersion?columnId=20&token=STuid00000%d%s&targetSourceId=001005", now.UnixMilli(), utils.RandomString(20)),
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

// receiveCloud 领取云朵
func (t *InviteFriendsTask) receiveCloud() {
	resp, err := api.NewCaiyunAPI(t.client).ReceivePendingCloudRewards()
	if err != nil {
		t.logger.Error("领取云朵失败", err)
		return
	}
	if resp != nil && !resp.IsSuccess() {
		msg := resp.MessageText()
		if msg == "" {
			msg = "领取云朵失败"
		}
		t.logger.Error("领取云朵失败", fmt.Errorf("%s", msg))
	}
}
