package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// UpgradeGiftTask（National_v13gift 全网云盘焕新权益免费领）。
//
// 奖池通过 `getPrizePool` 读取；激活走活动侧提供的免验证码官方通道
// `getPrize?smsCode=innerActivation`。无领奖记录的账号回
// `404 未找到领奖记录`（不在白名单），属正常结果，不视为失败。
type UpgradeGiftTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewUpgradeGiftTask 创建焕新权益任务。
func NewUpgradeGiftTask(client *http.Client, log *logger.Logger) *UpgradeGiftTask {
	return &UpgradeGiftTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *UpgradeGiftTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 读取奖池与领奖记录，并在有记录时激活。
func (t *UpgradeGiftTask) Run() error {
	var issues []error
	var parts []string

	if resp, err := t.api.UpgradeGiftPrizePool(); err != nil {
		issues = append(issues, fmt.Errorf("焕新权益奖池: %w", err))
	} else if responseCodeIs(resp, 0) {
		pool := api.ParseUpgradeGiftPrizePool(resp)
		if len(pool) > 0 {
			top := pool[0]
			parts = append(parts, fmt.Sprintf("奖池%d项，首位：%s（剩余%d）", len(pool), top.PrizeName, top.DailyRemain))
		} else {
			parts = append(parts, "焕新权益奖池为空")
		}
	} else {
		issues = append(issues, rewardResponseError("焕新权益奖池", resp, 404, 602))
	}

	record, err := t.api.UpgradeGiftPrizeRecord()
	if err != nil {
		issues = append(issues, fmt.Errorf("焕新权益领奖记录: %w", err))
	} else if !responseCodeIs(record, 0) {
		issues = append(issues, rewardResponseError("焕新权益领奖记录", record, 404, 602))
	} else if len(resultItems(record)) == 0 {
		parts = append(parts, "无领奖记录（不在白名单）")
	} else {
		if resp, err := t.api.ActivateUpgradeGift(); err != nil {
			issues = append(issues, fmt.Errorf("焕新权益激活: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "焕新权益已激活")
		} else if responseCodeIs(resp, 404, 411, 602) {
			parts = append(parts, "焕新权益无需激活或已领取")
		} else {
			issues = append(issues, rewardResponseError("焕新权益激活", resp))
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
