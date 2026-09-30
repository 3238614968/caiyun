package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// MCloudDayTask（National_MCloudDay 全网移动云盘会员日）。
//
// 活动开放状态有三个独立开关：主活动 online、额外礼包 extGiftOnline、
// 盲盒 blindboxOnline。「主活动关、额外礼包开」是常见组合，因此逐项判断，
// 并以各业务接口的实际回执为准。
type MCloudDayTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewMCloudDayTask 创建会员日任务。
func NewMCloudDayTask(client *http.Client, log *logger.Logger) *MCloudDayTask {
	return &MCloudDayTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *MCloudDayTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 查询会员日状态并在开放时尝试领礼与盲盒抽奖。
func (t *MCloudDayTask) Run() error {
	var issues []error
	var parts []string

	infoResp, err := t.api.MCloudDayActivityInfo()
	if err != nil {
		return fmt.Errorf("会员日状态查询: %w", err)
	}
	if err := rewardResponseError("会员日状态查询", infoResp); err != nil {
		return err
	}
	info, ok := api.ParseMCloudDayActivityInfo(infoResp)
	if !ok {
		return fmt.Errorf("会员日活动状态响应缺少有效 result")
	}

	hasStock := false
	if giftResp, err := t.api.MCloudDayGiftList(); err != nil {
		issues = append(issues, fmt.Errorf("会员日礼品列表: %w", err))
	} else {
		if err := rewardResponseError("会员日礼品列表", giftResp, 602); err != nil {
			issues = append(issues, err)
		} else if responseCodeIs(giftResp, 0) {
			hasStock = api.MCloudDayGiftHasStock(giftResp)
		}
	}
	parts = append(parts, api.MCloudDayMessage(info, hasStock))

	if info.ExtGiftOnline && hasStock {
		if resp, err := t.api.MCloudDayGiftVerify(); err != nil {
			issues = append(issues, fmt.Errorf("会员日礼品校验: %w", err))
		} else if responseCodeIs(resp, 0) {
			if received, err := t.api.MCloudDayGiftReceive(); err != nil {
				issues = append(issues, fmt.Errorf("会员日礼品领取: %w", err))
			} else if responseCodeIs(received, 0) {
				parts = append(parts, "会员日礼品已领取")
			} else {
				issues = append(issues, rewardResponseError("会员日礼品领取", received, 411, 602))
			}
		} else {
			issues = append(issues, rewardResponseError("会员日礼品校验", resp, 602))
		}
	}

	if info.BlindboxOnline {
		if resp, err := t.api.MCloudDayBlindboxLottery(); err != nil {
			issues = append(issues, fmt.Errorf("会员日盲盒抽奖: %w", err))
		} else if responseCodeIs(resp, 0) {
			parts = append(parts, "会员日盲盒已抽奖")
		} else if skipErr := rewardResponseError("会员日盲盒抽奖", resp, 602); skipErr != nil {
			// 次数耗尽属于正常终止态。
			if !isNoLotteryChanceError(skipErr) {
				issues = append(issues, skipErr)
			} else {
				parts = append(parts, "会员日盲盒次数已用完")
			}
		} else {
			parts = append(parts, "会员日盲盒未开放")
		}
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
