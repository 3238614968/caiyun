package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// RedInviteTask（National_Invitingtask 全网红包邀请）。
//
// 邀请方生成邀请码；被邀请方提交邀请码（请求体里的 data 是「公钥加密的
// 活动名 + 毫秒时间戳」）。服务端有一道前置风控：`risk` 回
// success=false / body=999 时前端连 acceptInvite 都不会发，
// 此时任务如实报告风控结论而不视为失败。
//
// 作为被邀请方时，邀请码从环境变量 CAIYUN_REDINVITE_CODE 读取。
type RedInviteTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewRedInviteTask 创建红包邀请任务。
func NewRedInviteTask(client *http.Client, log *logger.Logger) *RedInviteTask {
	return &RedInviteTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *RedInviteTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询邀请额度与风控状态，并在配置了邀请码且风控放行时接受邀请。
func (t *RedInviteTask) Run() error {
	var issues []error
	var parts []string

	if resp, err := t.api.RedInviteMonthInfo(); err != nil {
		issues = append(issues, fmt.Errorf("红包邀请额度查询: %w", err))
	} else if info, ok := api.ParseRedInviteMonthInfo(resp); ok {
		parts = append(parts, fmt.Sprintf(
			"本月额度：邀新%dG/%s元、邀老%dG/%s元、已邀请%d人",
			info.NewUserCloudNum, info.NewUserRedNum,
			info.OldUserCloudNum, info.OldUserRedNum,
			info.NewUserNum+info.OldUserNum,
		))
	} else {
		issues = append(issues, rewardResponseError("红包邀请额度查询", resp, 602, 604))
	}

	riskPassed := false
	if resp, err := t.api.RedInviteRisk(); err != nil {
		issues = append(issues, fmt.Errorf("红包邀请风控检查: %w", err))
	} else {
		riskPassed = api.RedInviteRiskPassed(resp)
		if !riskPassed {
			parts = append(parts, "风控未放行（success=false/body=999），服务端会拒绝接受邀请")
		}
	}

	if code := t.setting("CAIYUN_REDINVITE_CODE"); code != "" && riskPassed {
		if resp, err := t.api.RedInviteAccept(code); err != nil {
			issues = append(issues, fmt.Errorf("接受红包邀请: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "已接受红包邀请")
		} else if skipErr := rewardResponseError("接受红包邀请", resp, 3008, 3009, 3010, 411, 602); skipErr != nil {
			issues = append(issues, skipErr)
		} else {
			// 3008/3009/3010 都是活动侧资格或风控结论，如实报告即可。
			parts = append(parts, fmt.Sprintf("接受邀请被拒绝：%s", resp.MessageText()))
		}
	} else if code == "" {
		if generated, err := t.api.RedInviteGenerateCode(); err != nil {
			issues = append(issues, fmt.Errorf("生成红包邀请码: %w", err))
		} else {
			parts = append(parts, fmt.Sprintf("已生成本月邀请码（长度%d）", len(generated)))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
