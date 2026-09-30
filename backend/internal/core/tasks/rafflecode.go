package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// RafflecodeTask（/market/rafflecode/* 抽奖码）。
//
// 这一族挂在 `/market/*` 下，认证头是 `jwtToken`（与 `jwttoken` 同值，
// 但服务端只读这个头名），缺了会静默回 `90001 未登录`。
// 逆向实测当前无进行中的场次（rafflecodeConfigList 为空），
// 因此本任务以「查询 + 如实报告场次」为主。
type RafflecodeTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewRafflecodeTask 创建抽奖码任务。
func NewRafflecodeTask(client *http.Client, log *logger.Logger) *RafflecodeTask {
	return &RafflecodeTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *RafflecodeTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询抽奖码场次与我的奖品。
func (t *RafflecodeTask) Run() error {
	var issues []error
	var parts []string

	listResp, err := t.api.RafflecodeList()
	if err != nil {
		return fmt.Errorf("抽奖码列表查询: %w", err)
	}
	if !responseCodeIs(listResp, 0) && !listResp.IsSuccess() {
		// 90001 未登录等状态如实报告，不视为批次失败。
		parts = append(parts, fmt.Sprintf("抽奖码不可用：code=%v %s", listResp.Code, listResp.MessageText()))
		issues = append(issues, rewardResponseError("抽奖码列表", listResp, 404, 602))
	} else {
		sessions := api.RafflecodeActiveSessions(listResp)
		parts = append(parts, fmt.Sprintf("抽奖码场次%d个", sessions))
		if sessions == 0 {
			parts = append(parts, "当前无进行中的场次")
		}
	}

	if resp, err := t.api.RafflecodeMyPrize(); err != nil {
		issues = append(issues, fmt.Errorf("抽奖码我的奖品: %w", err))
	} else if responseCodeIs(resp, 0) {
		parts = append(parts, fmt.Sprintf("我的抽奖码奖品%d条", len(resultItems(resp))))
	} else {
		issues = append(issues, rewardResponseError("抽奖码奖品", resp, 404, 602))
	}

	if resp, err := t.api.RafflecodeInfo(); err != nil {
		issues = append(issues, fmt.Errorf("抽奖码概览: %w", err))
	} else if responseCodeIs(resp, 0) {
		if result, ok := resp.Result.(map[string]interface{}); ok {
			unread := 0
			if items, ok := result["unReadRafflecodeRecordList"].([]interface{}); ok {
				unread = len(items)
			}
			if unread > 0 || toBool(result["subscribeSwitch"]) {
				parts = append(parts, fmt.Sprintf("未读记录%d条", unread))
			}
		}
	} else {
		issues = append(issues, rewardResponseError("抽奖码概览", resp, 404, 602))
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
