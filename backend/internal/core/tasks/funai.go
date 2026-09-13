package tasks

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// FunAITask 趣玩AI赢好礼（National_playAI）：通过真实 AI 功能调用完成任务
// 获得抽奖次数，并按模块抽奖。
type FunAITask struct {
	*activityActions
	lastMessage string
}

func NewFunAITask(client *http.Client, log *logger.Logger) *FunAITask {
	return &FunAITask{activityActions: newActivityActions(client, log)}
}

func (t *FunAITask) SetStorage(store Storage) *FunAITask {
	t.setStorage(store)
	return t
}

func (t *FunAITask) SetAccountContext(phone, authToken string) *FunAITask {
	t.setAccountContext(phone, authToken)
	return t
}

const funaiMaxLotteries = 5

// funaiChatTasks/funaiCameraTasks/funaiNoteTasks 是任务 id 与可自动化动作的映射。
// 其余任务（智能抠图、AI漫画、表情包等 office 工具入口）接口不在抓包样本内，
// 保持未完成并提示。
var (
	funaiChatTasks   = map[string]bool{"aizhushou": true, "lingxiluxian": true, "zuowenzhushou": true}
	funaiCameraTasks = map[string]bool{"aixiangji": true, "paizhaowenai": true}
	funaiNoteTasks   = map[string]bool{"yunbiji": true}
)

func (t *FunAITask) Run() error {
	t.logger.Start("------【趣玩AI抽奖】------")
	t.api.PrepareActivitySession(api.FunAIMarketName)

	pageModule, err := t.api.FunaiPageModule()
	if err != nil {
		t.logger.Error("获取趣玩AI模块失败", err)
		return err
	}

	modules := []api.FunaiModule{*pageModule}
	if timed, timedErr := t.api.FunaiTimedModules(); timedErr == nil {
		modules = append(modules, timed...)
	} else {
		t.logger.Debug(fmt.Sprintf("获取趣玩AI限时模块失败: %v", timedErr))
	}

	pendingManual := 0
	executed := 0
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			if task.IsComplete == 1 {
				continue
			}
			switch {
			case funaiChatTasks[task.ID]:
				if err := t.api.CompleteLingxiChat(); err == nil {
					executed++
				} else {
					t.logger.Debug(fmt.Sprintf("趣玩AI对话任务失败(%s): %v", task.TaskName, err))
				}
			case funaiCameraTasks[task.ID]:
				if err := t.api.CompleteAICameraTask(); err == nil {
					executed++
				} else {
					t.logger.Debug(fmt.Sprintf("趣玩AI相机任务失败(%s): %v", task.TaskName, err))
				}
			case funaiNoteTasks[task.ID]:
				if err := t.createTempNote(); err == nil {
					executed++
					// 抓包确认云笔记任务需要 page/register 登记。
					if err := t.api.FunaiRegister(module.ModuleID, task.ID); err != nil {
						t.logger.Debug(fmt.Sprintf("趣玩AI任务登记失败(%s): %v", task.TaskName, err))
					}
				} else {
					t.logger.Debug(fmt.Sprintf("趣玩AI笔记任务失败(%s): %v", task.TaskName, err))
				}
			default:
				pendingManual++
			}
		}
	}

	time.Sleep(2 * time.Second)
	prizes, lotteryErr := t.runLotteries(pageModule.ModuleID)

	parts := make([]string, 0, 4)
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("新完成任务%d项", executed))
	}
	if len(prizes) > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", len(prizes), strings.Join(prizes, "、")))
	}
	if pendingManual > 0 {
		parts = append(parts, fmt.Sprintf("%d项AI工具体验任务需手动", pendingManual))
	}
	if lotteryErr != nil && len(prizes) == 0 && executed == 0 {
		// 抽奖次数耗尽属于正常终止态，不视为任务失败；仅在真正的接口异常时返回错误。
		if !isNoLotteryChanceError(lotteryErr) {
			t.lastMessage = strings.Join(parts, "; ")
			return lotteryErr
		}
		parts = append(parts, "暂无可用抽奖次数")
	}
	if len(parts) == 0 {
		parts = append(parts, "暂无可用抽奖次数")
	}
	t.lastMessage = strings.Join(parts, "; ")
	t.logger.Success("趣玩AI: " + t.lastMessage)
	return nil
}

// runLotteries 连续抽奖直到没有可用次数，最多 funaiMaxLotteries 次。
func (t *FunAITask) runLotteries(moduleID string) ([]string, error) {
	prizes := make([]string, 0, funaiMaxLotteries)
	var lastErr error
	for round := 0; round < funaiMaxLotteries; round++ {
		prize, err := t.api.FunaiLottery(moduleID)
		if err != nil {
			lastErr = err
			break
		}
		name := strings.TrimSpace(prize.PrizeName)
		if name == "" {
			name = "谢谢参与"
		}
		prizes = append(prizes, name)
		time.Sleep(time.Second)
	}
	return prizes, lastErr
}

func (t *FunAITask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

// isNoLotteryChanceError 判断抽奖接口返回是否为“次数已用完”这类正常终止提示，
// 而非真正的服务异常。
func isNoLotteryChanceError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, marker := range []string{"次数用完", "次数已用完", "没有可用次数", "无可用次数", "次数不足", "没有抽奖次数"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
