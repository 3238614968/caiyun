package tasks

import (
	"errors"
	"fmt"
	"strings"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// PrizeCenterTask（National_Getprize 全网领奖专区）。
//
// 默认行为是「盘点」：列出未领取奖品并按到期时间排序，方便账号持有人
// 在 App 内逐个领取 —— 领奖专区按单个奖品（oid）发码，且发码前必须过滑块，
// 服务端另有每日短信上限，因此不做批量自动领取。
//
// 只有在按账号提供明确的奖品 OID 与该奖品已收到的验证码时才领取。
// 发送短信须先经官方滑块流程完成，领取任务不再重新发码。
type PrizeCenterTask struct {
	accountTaskSettings
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewPrizeCenterTask 创建领奖专区任务。
func NewPrizeCenterTask(client *http.Client, log *logger.Logger) *PrizeCenterTask {
	return &PrizeCenterTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *PrizeCenterTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 盘点未领取奖品，并在配置齐备时领取其中一件。
func (t *PrizeCenterTask) Run() error {
	var issues []error
	var parts []string

	entries, err := t.api.PrizeCenterAllEntries()
	if err != nil {
		return fmt.Errorf("领奖专区奖品盘点: %w", err)
	}

	unclaimed := api.PrizeCenterUnclaimed(entries)
	parts = append(parts, fmt.Sprintf("奖品记录%d条、未领取%d条", len(entries), len(unclaimed)))
	if gift, err := t.api.PrizeCenterSpaceGiftInfo(); err != nil {
		issues = append(issues, fmt.Errorf("空间礼包查询: %w", err))
	} else if responseCodeIs(gift, 0) {
		if info, ok := gift.Result.(map[string]interface{}); ok {
			if status, present := info["status"]; present && responseNumber(status) == 0 {
				parts = append(parts, "无空间礼包")
			} else {
				parts = append(parts, "空间礼包状态需在App核实")
			}
		}
	} else {
		issues = append(issues, rewardResponseError("空间礼包查询", gift, 404, 602))
	}
	if len(unclaimed) > 0 {
		preview := make([]string, 0, 3)
		for _, entry := range unclaimed {
			if len(preview) >= 3 {
				break
			}
			name := strings.TrimSpace(entry.PrizeName)
			if name == "" {
				name = entry.OID
			}
			expire := strings.TrimSpace(entry.ExpireTime)
			if len(expire) > 10 {
				expire = expire[:10]
			}
			preview = append(preview, fmt.Sprintf("%s(到期%s)", name, expire))
		}
		parts = append(parts, "待领："+strings.Join(preview, "、"))
	}

	claimErr := t.claimFirstUnclaimed(unclaimed, &parts)
	if claimErr != nil {
		issues = append(issues, claimErr)
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

func (t *PrizeCenterTask) claimFirstUnclaimed(unclaimed []api.PrizeCenterEntry, parts *[]string) error {
	smsCode := t.setting("CAIYUN_PRIZE_SMS_CODE")
	oid := t.setting("CAIYUN_PRIZE_OID")
	if len(unclaimed) == 0 {
		return nil
	}
	if smsCode == "" || oid == "" {
		*parts = append(*parts, "未提供此账号的奖品 OID 与已收到的验证码，仅盘点")
		return nil
	}
	var target *api.PrizeCenterEntry
	for i := range unclaimed {
		if unclaimed[i].OID == oid {
			target = &unclaimed[i]
			break
		}
	}
	if target == nil {
		return fmt.Errorf("此账号的待领奖品中找不到指定 OID")
	}
	// The code is for this exact OID. Sending another SMS here would invalidate
	// the code the user has already supplied.
	resp, err := t.api.PrizeCenterAccept(target.OID, smsCode)
	if err != nil {
		return fmt.Errorf("领奖专区领取(%s): %w", target.OID, err)
	}
	if responseCodeIs(resp, 0) {
		*parts = append(*parts, "已领取："+target.PrizeName)
		return nil
	}
	return rewardResponseError("领奖专区领取", resp, 411, 602)
}
