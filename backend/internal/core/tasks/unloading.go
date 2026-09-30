package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// Unloading1TTask（newgifts1T 移动云盘 1T 新礼）。
//
// 逆向实测：认证通过后服务端回 `503 远程调用失败`（后端未部署）。
// 因此本任务只做探测与如实报告；提供兑换码时才尝试领取。
type Unloading1TTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewUnloading1TTask 创建 1T 新礼任务。
func NewUnloading1TTask(client *http.Client, log *logger.Logger) *Unloading1TTask {
	return &Unloading1TTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *Unloading1TTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 探测活动状态并在配置了兑换码时尝试领取。
func (t *Unloading1TTask) Run() error {
	var issues []error
	var parts []string

	switch resp, err := t.api.UnloadUserInfo(); {
	case err != nil:
		issues = append(issues, fmt.Errorf("1T新礼资格查询: %w", err))
	case !responseCodeIs(resp, 0):
		// 503 远程调用失败 / 404 / 602 都是活动侧状态，如实报告。
		parts = append(parts, fmt.Sprintf("1T新礼不可用：code=%v %s", resp.Code, resp.MessageText()))
		issues = append(issues, rewardResponseError("1T新礼资格", resp, 503, 404, 602))
	default:
		parts = append(parts, "1T新礼接口可达")
		if records, err := t.api.UnloadPrizeRecords(); err != nil {
			issues = append(issues, fmt.Errorf("1T新礼领奖记录: %w", err))
		} else {
			parts = append(parts, fmt.Sprintf("已领记录%d条", len(resultItems(records))))
			issues = append(issues, rewardResponseError("1T新礼领奖记录", records, 503, 404, 602))
		}
	}

	if code := t.setting("CAIYUN_UNLOAD_CODE"); code != "" {
		prizeType := t.setting("CAIYUN_UNLOAD_TYPE")
		if resp, err := t.api.UnloadSendPrize(prizeType, code); err != nil {
			issues = append(issues, fmt.Errorf("1T新礼领取: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "1T新礼已领取")
		} else if skipErr := rewardResponseError("1T新礼领取", resp, 411, 602, 503); skipErr != nil {
			issues = append(issues, skipErr)
		} else {
			parts = append(parts, fmt.Sprintf("1T新礼领取被拒绝：%s", resp.MessageText()))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

// resultItems 把响应的 result 统一成列表，便于计数。
func resultItems(resp *api.CaiyunResponse) []interface{} {
	if resp == nil {
		return nil
	}
	if items, ok := resp.Result.([]interface{}); ok {
		return items
	}
	if value, ok := resp.Result.(map[string]interface{}); ok {
		for _, key := range []string{"records", "list", "result"} {
			if items, ok := value[key].([]interface{}); ok {
				return items
			}
		}
	}
	return nil
}
