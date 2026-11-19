package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// FamilyCircleTask（家庭圈任务）。
//
// 需求体字段名是服务端下发的 `gruopId`（拼写如此），另需当前账号的
// userDomainId。HTTP 错误不能作为没有群组或任务完成的证明。
//
// 群组 ID 从环境变量 CAIYUN_FAMILY_GROUP_ID 读取（可选）。
type FamilyCircleTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
	pending     bool
}

// NewFamilyCircleTask 创建家庭圈任务。
func NewFamilyCircleTask(client *http.Client, log *logger.Logger) *FamilyCircleTask {
	return &FamilyCircleTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *FamilyCircleTask) Message() string { return strings.TrimSpace(t.lastMessage) }
func (t *FamilyCircleTask) Pending() bool   { return t.pending }

// Run 查询家庭圈备份与任务状态。
func (t *FamilyCircleTask) Run() error {
	var issues []error
	var parts []string
	groupID := t.setting("CAIYUN_FAMILY_GROUP_ID")
	t.pending = false
	if groupID == "" {
		groups, err := t.api.FamilyCircleGroups()
		if err != nil {
			t.pending = true
			t.lastMessage = "未提供有效家庭圈群组ID或账号上下文，群组查询未确认，未提交缺少gruopId的请求"
			if errors.Is(err, api.ErrFamilyCircleAccountMissing) {
				return nil
			}
			return err
		}
		// Native metadata defines type=0 as family; type=1 is a friend circle.
		// Do not run family marketing actions against an arbitrary joined circle.
		var familyGroups []api.FamilyCircleGroup
		for _, group := range groups {
			if group.Type == "0" {
				familyGroups = append(familyGroups, group)
			}
		}
		groups = familyGroups
		if len(groups) != 1 {
			t.pending = true
			t.lastMessage = fmt.Sprintf("服务端返回本人群组%d个，需明确家庭圈群组后核验，未提交缺参或任意群组请求", len(groups))
			return nil
		}
		groupID = groups[0].ID
	}

	backupResp, err := t.api.FamilyCircleBackupState(groupID)
	if err != nil {
		issues = append(issues, fmt.Errorf("家庭圈备份状态: %w", err))
		if errors.Is(err, api.ErrFamilyCircleEndpointUnavailable) {
			parts = append(parts, "官方家庭圈备份接口 HTTP 404，备份记录未确认")
		}
	} else if responseCodeIs(backupResp, 0) {
		parts = append(parts, fmt.Sprintf("家庭圈备份服务端状态=%v", backupResp.Result))
	} else {
		issues = append(issues, rewardResponseError("家庭圈备份状态", backupResp))
	}

	taskResp, err := t.api.FamilyCircleTaskState(groupID)
	if err != nil {
		issues = append(issues, fmt.Errorf("家庭圈任务状态: %w", err))
		if errors.Is(err, api.ErrFamilyCircleEndpointUnavailable) {
			parts = append(parts, "官方家庭圈任务接口 HTTP 404，任务完成记录未确认")
		}
	} else if responseCodeIs(taskResp, 0) {
		parts = append(parts, fmt.Sprintf("家庭圈任务服务端状态=%v", taskResp.Result))
	} else if skipErr := rewardResponseError("家庭圈任务状态", taskResp); skipErr != nil {
		issues = append(issues, skipErr)
	} else {
		parts = append(parts, "家庭圈任务接口不可达")
	}

	if len(parts) == 0 {
		parts = append(parts, "无可用动作")
	}
	t.lastMessage = strings.Join(parts, "；")
	// The native response is a scalar status, not a task array or a receipt.
	// Keep it pending until a documented completion/award record is confirmed.
	t.pending = true
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
