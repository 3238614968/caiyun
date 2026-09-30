package tasks

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

var hiddenPrizeMarkets = []string{
	"National_NewLoginGif", "NationalMailAppFreeSpace", "NationalMailH5FreeSpace",
	"National_MailGiftNewUser", "National_MCloudDay", "National_OpenLuckybag",
}

// HiddenRewardsTask scans task and prize entries absent from the regular V3 table.
type HiddenRewardsTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

func NewHiddenRewardsTask(client *http.Client, log *logger.Logger) *HiddenRewardsTask {
	return &HiddenRewardsTask{api: api.NewCaiyunAPI(client), logger: log}
}

func (t *HiddenRewardsTask) Message() string { return t.lastMessage }

func (t *HiddenRewardsTask) Run() error {
	var issues []error
	var parts []string
	seen := map[string]bool{}
	for attempt := 0; attempt < 8; attempt++ {
		resp, err := t.api.HiddenSignTasks()
		if err != nil {
			issues = append(issues, fmt.Errorf("隐藏任务盘点: %w", err))
			break
		}
		if !responseCodeIs(resp, 0) {
			issues = append(issues, rewardResponseError("隐藏任务盘点", resp))
			break
		}
		for _, item := range responseArray(resp.Result) {
			if task, ok := item.(map[string]interface{}); ok {
				seen[fmt.Sprint(task["id"])] = true
			}
		}
		if len(seen) >= 7 {
			break
		}
	}
	if len(seen) > 0 {
		parts = append(parts, fmt.Sprintf("App任务视图%d项（隐藏548=%v、550=%v）", len(seen), seen["548"], seen["550"]))
	}

	if info, err := t.api.OpenEmailSMSTaskInfo(); err != nil {
		issues = append(issues, fmt.Errorf("短信通知状态: %w", err))
	} else if !responseCodeIs(info, 0) {
		if infoErr := rewardResponseError("短信通知状态", info, 604, 602); infoErr != nil {
			issues = append(issues, infoErr)
		} else {
			parts = append(parts, "邮箱短信通知奖励不符合号码资格或已结束")
		}
	} else if reward, err := t.api.ClaimOpenEmailSMSReward(); err != nil {
		issues = append(issues, fmt.Errorf("短信通知领奖: %w", err))
	} else if responseCodeIs(reward, 0) {
		parts = append(parts, "邮箱短信通知奖励已领取")
	} else if responseCodeIs(reward, 411, 604) {
		parts = append(parts, "邮箱短信通知奖励已领或不符合号码资格")
	} else {
		issues = append(issues, rewardResponseError("邮箱短信通知领奖", reward, 602))
	}

	if resp, err := t.api.SpringGiftTasks(); err != nil {
		issues = append(issues, fmt.Errorf("月度福袋任务: %w", err))
	} else if responseCodeIs(resp, 0) {
		registered := 0
		for _, item := range responseArray(resp.Result) {
			task, ok := item.(map[string]interface{})
			if !ok || !toBool(task["needRegister"]) || toBool(task["complete"]) {
				continue
			}
			mark := fmt.Sprint(task["id"])
			if mark == "<nil>" || mark == "" {
				continue
			}
			result, registerErr := t.api.RegisterSpringGiftTask(mark)
			if registerErr != nil {
				issues = append(issues, fmt.Errorf("月度福袋任务%s登记: %w", mark, registerErr))
			} else if responseCodeIs(result, 0) {
				registered++
			} else {
				issues = append(issues, rewardResponseError("月度福袋任务登记", result))
			}
		}
		drew := 0
		if count, countErr := t.api.SpringGiftLotteryCount(); countErr != nil {
			issues = append(issues, fmt.Errorf("月度福袋抽奖次数: %w", countErr))
		} else if responseCodeIs(count, 0) {
			for i := 0; i < min(responseNumber(count.Result), 20); i++ {
				result, drawErr := t.api.DrawSpringGift()
				if drawErr != nil {
					issues = append(issues, fmt.Errorf("月度福袋抽奖: %w", drawErr))
					break
				}
				if !responseCodeIs(result, 0) {
					if drawErr := rewardResponseError("月度福袋抽奖", result); !isNoLotteryChanceError(drawErr) {
						issues = append(issues, drawErr)
					}
					break
				}
				drew++
				time.Sleep(400 * time.Millisecond)
			}
		} else {
			issues = append(issues, rewardResponseError("月度福袋抽奖次数", count))
		}
		parts = append(parts, fmt.Sprintf("月度福袋登记%d项、抽奖%d次", registered, drew))
	} else {
		issues = append(issues, rewardResponseError("月度福袋任务", resp, 602))
	}

	if resp, err := t.api.CloudPendingInfo(); err != nil {
		issues = append(issues, fmt.Errorf("云朵中心待领盘点: %w", err))
	} else if responseCodeIs(resp, 0) {
		if result, ok := resp.Result.(map[string]interface{}); ok {
			free, later := 0, 0
			for _, item := range responseArray(result["receiveList"]) {
				entry, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				if responseNumber(entry["cloudType"]) == 0 {
					free += responseNumber(entry["cloudNum"])
				} else {
					later += responseNumber(entry["cloudNum"])
				}
			}
			parts = append(parts, fmt.Sprintf("待领云朵%d（本月%d、下月%d，需App验证）", responseNumber(result["toReceive"]), free, later))
		} else {
			issues = append(issues, fmt.Errorf("云朵中心待领盘点缺少有效 result"))
		}
	} else {
		issues = append(issues, rewardResponseError("云朵中心待领盘点", resp))
	}

	if resp, err := t.api.UpgradeGiftPrizeRecord(); err != nil {
		issues = append(issues, fmt.Errorf("焕新权益记录: %w", err))
	} else if !responseCodeIs(resp, 0) {
		issues = append(issues, rewardResponseError("焕新权益记录", resp, 404, 602))
	} else if len(responseArray(resp.Result)) > 0 {
		activation, activateErr := t.api.ActivateUpgradeGift()
		if activateErr != nil {
			issues = append(issues, fmt.Errorf("焕新权益激活: %w", activateErr))
		} else if responseCodeIs(activation, 0) {
			parts = append(parts, "焕新权益已激活")
		} else {
			issues = append(issues, rewardResponseError("焕新权益激活", activation))
		}
	}

	claimed := 0
	for _, market := range hiddenPrizeMarkets {
		query, err := t.api.QueryUnclaimedPrize(market)
		if err != nil {
			issues = append(issues, fmt.Errorf("%s查询: %w", market, err))
			continue
		}
		if !responseCodeIs(query, 0) {
			issues = append(issues, rewardResponseError(market+"查询", query, 410, 602, 505))
			continue
		}
		accept, err := t.api.AcceptUnclaimedPrize(market)
		if err != nil {
			issues = append(issues, fmt.Errorf("%s领奖: %w", market, err))
		} else if responseCodeIs(accept, 0) {
			claimed++
		} else {
			issues = append(issues, rewardResponseError(market+"领奖", accept, 410))
		}
	}
	parts = append(parts, fmt.Sprintf("未领奖扫描%d项、新领%d项", len(hiddenPrizeMarkets), claimed))
	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}

// Only explicit, endpoint-specific terminal codes can be treated as a skip.
func rewardResponseError(operation string, resp *api.CaiyunResponse, allowed ...int) error {
	if responseCodeIs(resp, 0) || (len(allowed) > 0 && responseCodeIs(resp, allowed...)) {
		return nil
	}
	if resp == nil {
		return fmt.Errorf("%s返回空响应", operation)
	}
	return fmt.Errorf("%s失败: code=%v %s", operation, resp.Code, resp.MessageText())
}

func responseCodeIs(resp *api.CaiyunResponse, codes ...int) bool {
	if resp == nil {
		return false
	}
	code, err := strconv.Atoi(fmt.Sprint(resp.Code))
	if err != nil {
		return false
	}
	for _, candidate := range codes {
		if code == candidate {
			return true
		}
	}
	return false
}

func responseArray(value interface{}) []interface{} {
	items, _ := value.([]interface{})
	return items
}

func responseNumber(value interface{}) int {
	switch v := value.(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := strconv.Atoi(v.String())
		return n
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}
