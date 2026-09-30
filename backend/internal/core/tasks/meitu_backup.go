package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// MeituBackupTask（National_Meitubackup 全网美图授权备份领好礼，仅移动号）。
//
// 两个前置条件属于设备本地状态：`authStatus` 是用户是否授权美图、
// `backUpStatus` 是设备相册备份开关。二者由客户端上报，本任务只按服务端
// 已记录的状态判断是否领奖，不伪造设备状态；非移动号会回 `604`。
type MeituBackupTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewMeituBackupTask 创建美图授权备份任务。
func NewMeituBackupTask(client *http.Client, log *logger.Logger) *MeituBackupTask {
	return &MeituBackupTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *MeituBackupTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询前置状态并在服务端已记录满足时领奖。
func (t *MeituBackupTask) Run() error {
	var issues []error
	var parts []string

	authResp, err := t.api.MeituAuthStatus()
	if err != nil {
		return err
	}
	if !responseCodeIs(authResp, 0) {
		t.lastMessage = "美图授权查询不符合号码资格或不在活动期"
		return rewardResponseError("美图授权查询", authResp, 604, 602)
	}
	backupResp, err := t.api.MeituBackupState()
	if err != nil {
		return err
	}
	if !responseCodeIs(backupResp, 0) {
		t.lastMessage = "美图备份查询不符合号码资格或不在活动期"
		return rewardResponseError("美图备份查询", backupResp, 604, 602)
	}
	authStatus, authAwarded, authOK := api.ParseMeituAuthStatus(authResp)
	backupStatus, backupAwarded, backupOK := api.ParseMeituBackupStatus(backupResp)
	if !authOK || !backupOK {
		return fmt.Errorf("美图备份状态响应缺少有效 result")
	}
	parts = append(parts, fmt.Sprintf(
		"美图备份：授权=%d(已领%d)、备份=%d(已领%d)",
		authStatus, authAwarded, backupStatus, backupAwarded,
	))

	if countResp, err := t.api.MeituPrizeCount(); err != nil {
		issues = append(issues, err)
	} else if responseCodeIs(countResp, 0) {
		if total, _, ok := api.ParseMeituPrizeCount(countResp); ok {
			parts = append(parts, fmt.Sprintf("剩余可领次数%d", total))
		}
	} else {
		issues = append(issues, rewardResponseError("美图备份奖励次数", countResp, 604, 602))
	}

	if !api.MeituBackupEligible(authStatus, backupStatus) {
		parts = append(parts, "授权或备份前置未满足，跳过领奖")
	} else {
		if resp, err := t.api.MeituPrize(); err != nil {
			issues = append(issues, fmt.Errorf("美图备份领奖: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "美图备份奖励已领取")
		} else if responseCodeIs(resp, 411, 604, 404, 602) {
			parts = append(parts, "美图备份奖励已领、非移动号或不在活动期")
		} else {
			issues = append(issues, rewardResponseError("美图备份领奖", resp))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

func (t *MeituBackupTask) readAuthStatus() (int, int, bool) {
	resp, err := t.api.MeituAuthStatus()
	if err != nil || resp == nil || !responseCodeIs(resp, 0) {
		return 0, 0, false
	}
	return api.ParseMeituAuthStatus(resp)
}

func (t *MeituBackupTask) readBackupStatus() (int, int, bool) {
	resp, err := t.api.MeituBackupState()
	if err != nil || resp == nil || !responseCodeIs(resp, 0) {
		return 0, 0, false
	}
	return api.ParseMeituBackupStatus(resp)
}

func (t *MeituBackupTask) readPrizeCount() (int, int, bool) {
	resp, err := t.api.MeituPrizeCount()
	if err != nil || resp == nil || !responseCodeIs(resp, 0) {
		return 0, 0, false
	}
	return api.ParseMeituPrizeCount(resp)
}
