package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// StudentPerksTask（National_StudentPerks 全网学生认证福利）。
//
// 认证在微信侧完成，服务端只提供「同步认证结果」与「领奖」：
//
//	① cert/sync 如实拉取权威源状态（未认证账号同步后仍是未认证，属正常结果）
//	② certStatus=1 时可带短验码领奖，短验码由账号持有人在环境变量中提供
//
// 本任务不伪造认证状态。
type StudentPerksTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewStudentPerksTask 创建学生认证福利任务。
func NewStudentPerksTask(client *http.Client, log *logger.Logger) *StudentPerksTask {
	return &StudentPerksTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *StudentPerksTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询/同步认证状态，并在已认证时按配置领奖。
func (t *StudentPerksTask) Run() error {
	var issues []error
	var parts []string

	status, ok := t.fetchStatus()
	if !ok {
		issues = append(issues, fmt.Errorf("学生认证状态查询失败"))
	} else {
		parts = append(parts, describeStudentPerksStatus(status))
	}

	if syncResp, err := t.api.StudentPerksCertSync(); err != nil {
		issues = append(issues, fmt.Errorf("学生认证结果同步: %w", err))
	} else if responseCodeIs(syncResp, 0) {
		if refreshed, refreshedKnown := t.fetchStatus(); refreshedKnown {
			status = refreshed
			ok = true
			parts = append(parts, "同步后："+describeStudentPerksStatus(refreshed))
		} else {
			ok = false
			issues = append(issues, fmt.Errorf("学生认证同步后状态复查失败"))
		}
	} else if skipErr := rewardResponseError("学生认证结果同步", syncResp, 604, 602, 404); skipErr != nil {
		issues = append(issues, skipErr)
	} else {
		parts = append(parts, "学生认证同步不可用")
	}

	switch code := t.setting("CAIYUN_STUDENT_SMS_CODE"); {
	case !ok:
		parts = append(parts, "认证状态未确认，跳过领奖")
	case !api.StudentPerksReadyToClaim(status):
		parts = append(parts, "当前未处于可领奖状态，跳过领奖")
	case code == "":
		parts = append(parts, "已认证待领奖，未配置 CAIYUN_STUDENT_SMS_CODE，跳过领奖")
	default:
		if resp, err := t.api.StudentPerksPrizeClaim(code); err != nil {
			issues = append(issues, fmt.Errorf("学生认证领奖: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "学生认证奖励已领取")
		} else if responseCodeIs(resp, 411, 602, 404) {
			parts = append(parts, "学生认证奖励已领取或不在活动期")
		} else {
			issues = append(issues, rewardResponseError("学生认证领奖", resp))
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

func (t *StudentPerksTask) fetchStatus() (api.StudentPerksStatus, bool) {
	resp, err := t.api.StudentPerksUserStatus()
	if err != nil || resp == nil || !responseCodeIs(resp, 0) {
		return api.StudentPerksStatus{}, false
	}
	return api.ParseStudentPerksStatus(resp)
}

func describeStudentPerksStatus(status api.StudentPerksStatus) string {
	desc := strings.TrimSpace(status.CertStatusDesc)
	if desc == "" {
		desc = fmt.Sprintf("certStatus=%d", status.CertStatus)
	}
	message := fmt.Sprintf("学生认证：%s（可认证=%v、可领奖=%v）", desc, status.CanCert, status.CanClaim)
	if stock := strings.TrimSpace(status.StockStatusDesc); stock != "" {
		message += "、库存：" + stock
	}
	return message
}
