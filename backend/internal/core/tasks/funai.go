package tasks

import (
	"errors"
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
	pending     bool
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

const funaiMaxLotteries = 40

// These task IDs have reusable account actions. Image algorithms use an
// embedded synthetic single-face portrait and fetch live server templates.
var (
	funaiChatTasks   = map[string]bool{"aizhushou": true, "lingxiluxian": true, "zuowenzhushou": true}
	funaiCameraTasks = map[string]bool{"aixiangji": true, "paizhaowenai": true}
	funaiNoteTasks   = map[string]bool{"yunbiji": true}
	funaiImageTasks  = map[string]bool{"003": true, "006": true, "009": true, "034": true, "031": true, "007": true, "008": true, "004": true, "3Drenou": true}
)

func (t *FunAITask) Run() error {
	t.pending = false
	var issues []error
	attempted := make(map[string]bool)
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
		issues = append(issues, fmt.Errorf("获取趣玩 AI 限时模块: %w", timedErr))
	}

	pendingManual := 0
	executed := 0
	failedActions := 0
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			if task.IsComplete == 1 {
				continue
			}
			previousExecuted, previousFailures := executed, failedActions
			switch {
			case task.ID == "011":
				if err := t.api.CompleteFunAIMiaoyunTask(); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("AI 写真: %w", err))
					t.logger.Debug(fmt.Sprintf("趣玩 AI 写真任务失败: %v", err))
				}
			case task.ID == "204":
				if err := t.api.CompleteFunAIScanTask(); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("扫描文档: %w", err))
					t.logger.Debug(fmt.Sprintf("趣玩 AI 扫描文档任务失败: %v", err))
				}
			case task.ID == "ailuyinzhuanxie":
				if err := t.api.FunaiTimedTranscriptionCompleted(module.ModuleID, task.ID); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("录音转写: %w", err))
					t.logger.Debug(fmt.Sprintf("趣玩 AI 录音转写任务失败(%s): %v", task.TaskName, err))
				}
			case funaiChatTasks[task.ID]:
				if err := t.api.CompleteLingxiChat(); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("AI 对话/%s: %w", task.ID, err))
					t.logger.Debug(fmt.Sprintf("趣玩AI对话任务失败(%s): %v", task.TaskName, err))
				}
			case funaiCameraTasks[task.ID]:
				if err := t.api.CompleteAICameraTask(); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("AI 相机/%s: %w", task.ID, err))
					t.logger.Debug(fmt.Sprintf("趣玩AI相机任务失败(%s): %v", task.TaskName, err))
				}
			case funaiNoteTasks[task.ID]:
				if err := t.createTempNote(); err == nil {
					executed++
					// 抓包确认云笔记任务需要 page/register 登记。
					if err := t.api.FunaiRegister(module.ModuleID, task.ID); err != nil {
						failedActions++
						issues = append(issues, fmt.Errorf("趣玩 AI 笔记登记: %w", err))
						t.logger.Debug(fmt.Sprintf("趣玩AI任务登记失败(%s): %v", task.TaskName, err))
					}
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("趣玩 AI 笔记: %w", err))
					t.logger.Debug(fmt.Sprintf("趣玩AI笔记任务失败(%s): %v", task.TaskName, err))
				}
			case funaiImageTasks[task.ID]:
				if err := t.api.CompleteFunAIImageTask(task.ID); err == nil {
					executed++
				} else {
					failedActions++
					issues = append(issues, fmt.Errorf("趣玩 AI 图像/%s: %w", task.ID, err))
					t.logger.Debug(fmt.Sprintf("趣玩 AI 图像任务失败(%s): %v", task.TaskName, err))
				}
			default:
				pendingManual++
			}
			if executed > previousExecuted && failedActions == previousFailures {
				attempted[module.ModuleID+"/"+task.ID] = true
			}
		}
	}

	time.Sleep(2 * time.Second)
	confirmed := -1
	if latestPage, refreshErr := t.api.FunaiPageModule(); refreshErr == nil {
		latestModules := []api.FunaiModule{*latestPage}
		if latestTimed, timedErr := t.api.FunaiTimedModules(); timedErr == nil {
			latestModules = append(latestModules, latestTimed...)
		} else {
			issues = append(issues, fmt.Errorf("复查趣玩 AI 限时模块: %w", timedErr))
		}
		confirmed = countNewFunAICompletions(modules, latestModules)
		for _, module := range latestModules {
			for _, task := range module.TaskIDs {
				if task.IsComplete == 1 {
					delete(attempted, module.ModuleID+"/"+task.ID)
				}
			}
		}
	} else {
		t.logger.Debug(fmt.Sprintf("复查趣玩 AI 任务状态失败: %v", refreshErr))
		issues = append(issues, fmt.Errorf("复查趣玩 AI 主模块: %w", refreshErr))
	}
	t.pending = len(attempted) > 0
	prizes, lotteryErr := t.runLotteries(pageModule.ModuleID)

	parts := make([]string, 0, 4)
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("执行任务动作%d项", executed))
		if confirmed >= 0 {
			parts = append(parts, fmt.Sprintf("服务端确认新完成%d项", confirmed))
		}
	}
	if len(prizes) > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", len(prizes), strings.Join(prizes, "、")))
	}
	if pendingManual > 0 {
		parts = append(parts, fmt.Sprintf("%d项任务需 App 内交互、活动资格或服务端支持", pendingManual))
	}
	if failedActions > 0 {
		parts = append(parts, fmt.Sprintf("%d项自动任务执行失败", failedActions))
	}
	if lotteryErr != nil {
		// 抽奖次数耗尽属于正常终止态，不视为任务失败；仅在真正的接口异常时返回错误。
		if !isNoLotteryChanceError(lotteryErr) {
			issues = append(issues, lotteryErr)
		} else if len(prizes) == 0 {
			parts = append(parts, "暂无可用抽奖次数")
		}
	}
	if t.pending {
		parts = append(parts, fmt.Sprintf("%d项动作已提交，服务端尚未确认完成", len(attempted)))
	}
	if len(parts) == 0 {
		parts = append(parts, "暂无可用抽奖次数")
	}
	t.lastMessage = strings.Join(parts, "; ")
	if err := errors.Join(issues...); err != nil {
		return err
	}
	t.logger.Success("趣玩AI: " + t.lastMessage)
	return nil
}

func countNewFunAICompletions(before, after []api.FunaiModule) int {
	pending := make(map[string]bool)
	for _, module := range before {
		for _, task := range module.TaskIDs {
			if task.IsComplete != 1 {
				pending[module.ModuleID+"/"+task.ID] = true
			}
		}
	}
	count := 0
	for _, module := range after {
		for _, task := range module.TaskIDs {
			if task.IsComplete == 1 && pending[module.ModuleID+"/"+task.ID] {
				count++
				delete(pending, module.ModuleID+"/"+task.ID)
			}
		}
	}
	return count
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

func (t *FunAITask) Pending() bool { return t.pending }

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
