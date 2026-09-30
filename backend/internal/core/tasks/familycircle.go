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
// userDomainId。家庭圈必须先开通群组才谈得上任务状态，因此无群组时
// 服务端返回空/404 属功能门槛，如实报告即可。
//
// 群组 ID 从环境变量 CAIYUN_FAMILY_GROUP_ID 读取（可选）。
type FamilyCircleTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewFamilyCircleTask 创建家庭圈任务。
func NewFamilyCircleTask(client *http.Client, log *logger.Logger) *FamilyCircleTask {
	return &FamilyCircleTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *FamilyCircleTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询家庭圈备份与任务状态。
func (t *FamilyCircleTask) Run() error {
	var issues []error
	var parts []string
	groupID := t.setting("CAIYUN_FAMILY_GROUP_ID")

	backupResp, err := t.api.FamilyCircleBackupState(groupID)
	if err != nil {
		issues = append(issues, fmt.Errorf("家庭圈备份状态: %w", err))
	} else if responseCodeIs(backupResp, 0) && api.FamilyCircleHasGroup(backupResp) {
		parts = append(parts, "家庭圈备份状态已获取")
	} else {
		parts = append(parts, "家庭圈未开通或无群组（功能门槛）")
		issues = append(issues, rewardResponseError("家庭圈备份状态", backupResp, 404, 405))
	}

	taskResp, err := t.api.FamilyCircleTaskState(groupID)
	if err != nil {
		issues = append(issues, fmt.Errorf("家庭圈任务状态: %w", err))
	} else if responseCodeIs(taskResp, 0) {
		parts = append(parts, fmt.Sprintf("家庭圈任务条目%d条", len(resultItems(taskResp))))
	} else if skipErr := rewardResponseError("家庭圈任务状态", taskResp, 404, 405); skipErr != nil {
		issues = append(issues, skipErr)
	} else {
		parts = append(parts, "家庭圈任务接口不可达")
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
