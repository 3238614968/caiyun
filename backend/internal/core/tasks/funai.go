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
	wait        func(time.Duration)
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
// the authorized fixed reference portrait and fetch live style parameters.
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
	knowledgeAttempted := false
	var knowledgeErr error
	nineGridAttempted := false
	var nineGridErr error
	t.logger.Start("------【趣玩AI抽奖】------")
	t.api.PrepareActivitySession(api.FunAIMarketName)

	pageModule, err := t.api.FunaiPageModule()
	modules := []api.FunaiModule{}
	if err != nil {
		t.logger.Error("获取趣玩AI模块失败", err)
		issues = append(issues, fmt.Errorf("获取趣玩 AI 主模块: %w", err))
	} else {
		modules = append(modules, *pageModule)
	}
	if timed, timedErr := t.api.FunaiTimedModules(); timedErr == nil {
		modules = append(modules, timed...)
	} else {
		t.logger.Debug(fmt.Sprintf("获取趣玩AI限时模块失败: %v", timedErr))
		issues = append(issues, fmt.Errorf("获取趣玩 AI 限时模块: %w", timedErr))
	}
	// The official activity registers each entry before opening the feature.
	// Register both modules before a shared knowledge/ninegrid action, otherwise the
	// second module is not participating when that action takes place.
	knowledgeEntries := make(map[string]error)
	nineGridEntries := make(map[string]error)
	var knowledgeModuleIDs []string
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			if task.ID == "aijiugongge" && task.IsComplete != 1 {
				if _, seen := nineGridEntries[module.ModuleID]; !seen {
					nineGridEntries[module.ModuleID] = t.api.FunaiRegister(module.ModuleID, task.ID)
				}
			}
			if task.ID == "zhishiku" && task.IsComplete != 1 {
				if _, seen := knowledgeEntries[module.ModuleID]; !seen {
					knowledgeEntries[module.ModuleID] = t.api.FunaiRegister(module.ModuleID, task.ID)
					if knowledgeEntries[module.ModuleID] == nil {
						knowledgeModuleIDs = append(knowledgeModuleIDs, module.ModuleID)
					}
				}
			}
		}
	}

	pendingManual := 0
	var manualDetails []string
	executed := 0
	failedActions := 0
	imageResults := map[string]error{}
	imageFailures := map[string]struct {
		id  string
		err error
	}{}
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			if task.IsComplete == 1 {
				continue
			}
			previousExecuted, previousFailures := executed, failedActions
			switch {
			case task.ID == "zhishiku":
				if entryErr := knowledgeEntries[module.ModuleID]; entryErr != nil {
					failedActions++
					issues = append(issues, fmt.Errorf("知识库入口登记/%s: %w", module.ModuleID, entryErr))
					break
				}
				if !knowledgeAttempted {
					knowledgeAttempted = true
					// The App creation flow includes the official attributed click
					// and a real private library, then asynchronous activity readback.
					actionErr := t.api.CreateFunAIKnowledgeBase(knowledgeModuleIDs...)
					knowledgeErr = actionErr
					if actionErr != nil {
						failedActions++
						issues = append(issues, fmt.Errorf("知识库创建: %w", actionErr))
						break
					}
				}
				if knowledgeErr != nil {
					break
				}
				executed++
			case task.ID == "aijiugongge":
				if entryErr := nineGridEntries[module.ModuleID]; entryErr != nil {
					failedActions++
					issues = append(issues, fmt.Errorf("九宫格入口登记/%s: %w", module.ModuleID, entryErr))
					break
				}
				if !nineGridAttempted {
					nineGridAttempted = true
					_, nineGridErr = t.api.CompleteNineGridTask()
				}
				if nineGridErr != nil {
					failedActions++
					issues = append(issues, fmt.Errorf("九宫格作品: %w", nineGridErr))
				} else {
					executed++
				}
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
				actionErr, tried := imageResults[task.ID]
				if !tried {
					actionErr = t.api.CompleteFunAIImageTask(task.ID)
					imageResults[task.ID] = actionErr
					if actionErr == nil {
						executed++
					}
				}
				key := module.ModuleID + "/" + task.ID
				if actionErr == nil {
					attempted[key] = true
				} else {
					imageFailures[key] = struct {
						id  string
						err error
					}{task.ID, actionErr}
					t.logger.Debug(fmt.Sprintf("趣玩 AI 图像提交异常(%s)，先复查任务完成位: %v", task.TaskName, actionErr))
				}
			default:
				pendingManual++
				manualDetails = append(manualDetails, module.ModuleID+"/"+task.ID+" "+task.TaskName)
			}
			if executed > previousExecuted && failedActions == previousFailures {
				attempted[module.ModuleID+"/"+task.ID] = true
			}
		}
	}

	wait := t.wait
	if wait == nil {
		wait = time.Sleep
	}
	wait(2 * time.Second)
	confirmed := -1
	refresh := func() ([]api.FunaiModule, error) {
		var latest []api.FunaiModule
		var refreshIssues []error
		if page, err := t.api.FunaiPageModule(); err == nil {
			latest = append(latest, *page)
		} else {
			refreshIssues = append(refreshIssues, fmt.Errorf("复查趣玩 AI 主模块: %w", err))
		}
		if timed, err := t.api.FunaiTimedModules(); err == nil {
			latest = append(latest, timed...)
		} else {
			refreshIssues = append(refreshIssues, fmt.Errorf("复查趣玩 AI 限时模块: %w", err))
		}
		return latest, errors.Join(refreshIssues...)
	}
	latestModules, refreshErr := refresh()
	// The real create event is credited asynchronously (about seven seconds in
	// the authorized live check). Poll records only, never repeat the action.
	for retry := 0; ((knowledgeAttempted && knowledgeErr == nil) || (nineGridAttempted && nineGridErr == nil)) && refreshErr == nil && retry < 5 && pendingFunAIRecordedActions(attempted, latestModules); retry++ {
		wait(3 * time.Second)
		latestModules, refreshErr = refresh()
	}
	if refreshErr != nil {
		issues = append(issues, refreshErr)
	}
	if len(latestModules) > 0 {
		confirmed = countNewFunAICompletions(modules, latestModules)
		for _, module := range latestModules {
			for _, task := range module.TaskIDs {
				if task.IsComplete == 1 {
					delete(attempted, module.ModuleID+"/"+task.ID)
					delete(imageFailures, module.ModuleID+"/"+task.ID)
				}
			}
		}
	}
	// The same image action can be credited to multiple modules. Submit it once;
	// a rendering error only fails the task if its server completion bit remains
	// unset. Unknown confirmations and unrelated failures are never discarded.
	reportedImages := map[string]bool{}
	for _, failure := range imageFailures {
		if !reportedImages[failure.id] {
			reportedImages[failure.id] = true
			failedActions++
			issues = append(issues, fmt.Errorf("趣玩 AI 图像/%s: %w", failure.id, failure.err))
		}
	}
	t.pending = len(attempted) > 0 || pendingManual > 0
	var prizes []string
	var lotteryErr error
	if pageModule != nil {
		prizes, lotteryErr = t.runLotteries(pageModule.ModuleID)
	}
	zhangtingMessage := t.runZhangtingLottery()

	parts := make([]string, 0, 5)
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("执行任务动作%d项", executed))
	}
	if confirmed >= 0 {
		parts = append(parts, fmt.Sprintf("服务端确认新完成%d项", confirmed))
	}
	if len(prizes) > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", len(prizes), strings.Join(prizes, "、")))
	}
	if zhangtingMessage != "" {
		parts = append(parts, zhangtingMessage)
	}
	if pendingManual > 0 {
		parts = append(parts, fmt.Sprintf("%d项任务待处理：%s（需 App 内交互、活动资格或尚未适配，未记为完成）", pendingManual, strings.Join(manualDetails, "、")))
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
	if len(attempted) > 0 {
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

// runZhangtingLottery 掌厅每日抽奖一次（不消耗模块抽奖次数）。
// 属于附加每日机会：今日已抽或接口异常只记入消息，不影响主任务结果。
func (t *FunAITask) runZhangtingLottery() string {
	prize, err := t.api.FunaiZhangtingLottery()
	if err != nil {
		t.logger.Debug(fmt.Sprintf("趣玩AI掌厅抽奖不可用: %v", err))
		return "掌厅抽奖未执行（今日已抽或接口异常）"
	}
	if prize.IsWin == 1 {
		name := strings.TrimSpace(prize.PrizeName)
		if name == "" {
			name = "未知奖品"
		}
		return "掌厅抽奖中奖: " + name
	}
	return "掌厅抽奖未中奖"
}

func (t *FunAITask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func (t *FunAITask) Pending() bool { return t.pending }

func pendingFunAIRecordedActions(attempted map[string]bool, modules []api.FunaiModule) bool {
	pending := make(map[string]bool)
	for key := range attempted {
		if strings.HasSuffix(key, "/zhishiku") || strings.HasSuffix(key, "/aijiugongge") {
			pending[key] = true
		}
	}
	for _, module := range modules {
		for _, task := range module.TaskIDs {
			if (task.ID == "zhishiku" || task.ID == "aijiugongge") && task.IsComplete == 1 {
				delete(pending, module.ModuleID+"/"+task.ID)
			}
		}
	}
	return len(pending) > 0
}

// isNoLotteryChanceError 判断抽奖接口返回是否为“次数已用完”这类正常终止提示，
// 而非真正的服务异常。
func isNoLotteryChanceError(err error) bool {
	if err == nil {
		return false
	}
	message := err.Error()
	for _, marker := range []string{"次数用完", "次数已用完", "没有可用次数", "无可用次数", "次数不足", "没有抽奖次数", "暂无刮奖机会", "暂无抽奖机会"} {
		if strings.Contains(message, marker) {
			return true
		}
	}
	return false
}
