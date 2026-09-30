package tasks

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// NoticeSwitchTask 补齐云朵中心里「通知 / 开关」类前置动作。
//
//	reportAppNoticeStatus -> 551 连续31天开启云盘APP通知、1021
//	openSmsSwitch         -> 1079 邮箱短信通知连续天数
//
// 这两个动作本身不发奖，但决定服务端是否开始给连续天数记账。
// 旧实现只读取状态与领奖，未做过「上报开关已开」，因此这几项长期停在初始进度。
type NoticeSwitchTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewNoticeSwitchTask 创建通知开关任务。
func NewNoticeSwitchTask(client *http.Client, log *logger.Logger) *NoticeSwitchTask {
	return &NoticeSwitchTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *NoticeSwitchTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 上报 APP 通知状态并开启邮箱短信通知开关。
func (t *NoticeSwitchTask) Run() error {
	var issues []error
	var parts []string

	statusRaw := t.setting("CAIYUN_APP_NOTICE_STATUS")
	status, statusErr := strconv.Atoi(statusRaw)
	if statusRaw == "" {
		parts = append(parts, "未提供此账号的真实 APP 通知状态，跳过上报")
	} else if statusErr != nil || (status != 0 && status != 1) {
		issues = append(issues, fmt.Errorf("APP 通知真实状态必须为0或1"))
	} else if resp, err := t.api.ReportAppNoticeStatus(status); err != nil {
		issues = append(issues, fmt.Errorf("上报APP通知状态: %w", err))
	} else if responseCodeIs(resp, 0) {
		parts = append(parts, fmt.Sprintf("APP通知已按实际值上报=%d", status))
	} else if skipErr := rewardResponseError("上报APP通知状态", resp, 604, 602, 501); skipErr != nil {
		issues = append(issues, skipErr)
	} else {
		parts = append(parts, "APP通知上报被活动侧拒绝，已跳过")
	}

	enabled, known, statusErr := t.emailSmsSwitchEnabled()
	if statusErr != nil {
		issues = append(issues, statusErr)
	}
	switch {
	case !known:
		parts = append(parts, "邮箱短信开关状态未知")
	case enabled:
		parts = append(parts, "邮箱短信通知开关已开启")
	default:
		if resp, err := t.api.OpenEmailSmsSwitch(); err != nil {
			issues = append(issues, fmt.Errorf("开启邮箱短信通知: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "邮箱短信通知开关已开启")
		} else if skipErr := rewardResponseError("开启邮箱短信通知", resp, 604, 602); skipErr != nil {
			issues = append(issues, skipErr)
		} else {
			parts = append(parts, "邮箱短信通知开关不符合号码资格或活动已结束")
		}
	}

	if len(parts) == 0 {
		parts = append(parts, "无可用动作")
	}
	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

// emailSmsSwitchEnabled 查询邮箱短信通知开关，第二个返回值表示是否成功取到状态。
func (t *NoticeSwitchTask) emailSmsSwitchEnabled() (bool, bool, error) {
	resp, err := t.api.GetEmailSmsSwitchStatus()
	if err != nil {
		return false, false, err
	}
	if !responseCodeIs(resp, 0) {
		return false, false, rewardResponseError("邮箱短信通知状态", resp, 604, 602)
	}
	enabled, known := api.EmailSmsSwitchEnabled(resp)
	if !known {
		return false, false, fmt.Errorf("邮箱短信通知状态缺少有效 result")
	}
	return enabled, true, nil
}
