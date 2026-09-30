package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// AlbumBackupReportTask（相册自动备份开关状态上报）。
//
// 该端点的语义是**客户端上报本机真实开关值**（APK 的
// `ImageBackupAutoOpenUtil.reportAutoBackupOpenOrClose`），因此：
//
//   - 默认只查询服务端记录的状态，不上报；
//   - 只有在显式配置 `CAIYUN_ALBUM_BACKUP_STATUS=0|1`（表示设备实际值）时才上报。
//
// 在未真正开启备份时提交 status=1 属于向活动方提交虚假状态，本实现不提供
// 「强制置 1」的入口。补对平台头后服务端还会做号码白名单校验（`407`）。
type AlbumBackupReportTask struct {
	*activityActions
	lastMessage string
}

// NewAlbumBackupReportTask 创建相册备份上报任务。
func NewAlbumBackupReportTask(client *http.Client, log *logger.Logger) *AlbumBackupReportTask {
	return &AlbumBackupReportTask{activityActions: newActivityActions(client, log)}
}

// SetAccountContext 注入账号上下文（手机号用于加密上送）。
func (t *AlbumBackupReportTask) SetAccountContext(phone, authToken string) *AlbumBackupReportTask {
	t.setAccountContext(phone, authToken)
	return t
}

// Message 返回最近一次执行的结果描述。
func (t *AlbumBackupReportTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询备份状态；配置了真实开关值时才上报。
func (t *AlbumBackupReportTask) Run() error {
	var issues []error
	var parts []string

	if resp, err := t.api.AlbumAutoBackupStatus(); err != nil {
		issues = append(issues, fmt.Errorf("相册备份状态查询: %w", err))
	} else if responseCodeIs(resp, 0) {
		parts = append(parts, "相册备份状态已获取")
	} else {
		parts = append(parts, fmt.Sprintf("相册备份状态不可用：code=%v %s", resp.Code, resp.MessageText()))
		issues = append(issues, rewardResponseError("相册备份状态查询", resp, 407, 404, 604))
	}

	statusRaw := accountTaskSetting("CAIYUN_ALBUM_BACKUP_STATUS", t.phone)
	if statusRaw == "" {
		parts = append(parts, "未配置 CAIYUN_ALBUM_BACKUP_STATUS，仅查询不上报")
	} else if statusRaw != "0" && statusRaw != "1" {
		issues = append(issues, fmt.Errorf("相册备份真实状态必须为0或1"))
	} else {
		status := 0
		if statusRaw == "1" {
			status = 1
		}
		resp, err := t.api.ReportAlbumAutoBackupStatus(t.phone, status)
		if err != nil {
			issues = append(issues, fmt.Errorf("相册备份状态上报: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, fmt.Sprintf("已上报相册备份状态=%d", status))
		} else {
			// 407 号码不在营销号码库、604 仅限移动号等都属于活动侧资格结论。
			parts = append(parts, fmt.Sprintf("相册备份上报返回 code=%v %s", resp.Code, resp.MessageText()))
			issues = append(issues, rewardResponseError("相册备份上报", resp, 407, 604))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
