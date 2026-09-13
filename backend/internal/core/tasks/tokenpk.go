package tasks

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// TokenPKTask 算力大作战（National_TokenPK）。
type TokenPKTask struct {
	*activityActions
	lastMessage string
}

func NewTokenPKTask(client *http.Client, log *logger.Logger) *TokenPKTask {
	return &TokenPKTask{activityActions: newActivityActions(client, log)}
}

func (t *TokenPKTask) SetStorage(store Storage) *TokenPKTask {
	t.setStorage(store)
	return t
}

func (t *TokenPKTask) SetAccountContext(phone, authToken string) *TokenPKTask {
	t.setAccountContext(phone, authToken)
	return t
}

const (
	activityStateSuccess = "SUCCESS"
	activityStateFinish  = "FINISH"

	tokenPKActionReceive = "receive"
	tokenPKActionRun     = "run"
	tokenPKActionReserve = "reserve"
	tokenPKActionManual  = "manual"
)

// tokenPKManualReasons 无法通过接口自动化完成的任务按钮（key）。
var tokenPKManualReasons = map[string]string{
	"loginPc":      "需要登录PC客户端",
	"inviteFriend": "需要好友真实参与",
}

type tokenPKAction struct {
	Task api.ActivityTask
	Kind string
	Key  string
}

// planTokenPKActions 将任务列表归类为：领奖 / 可自动执行 / 预约 / 需手动。
func planTokenPKActions(tasks []api.ActivityTask) []tokenPKAction {
	actions := make([]tokenPKAction, 0, len(tasks))
	for _, task := range tasks {
		switch strings.ToUpper(strings.TrimSpace(task.State)) {
		case activityStateSuccess:
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionReceive})
			continue
		case activityStateFinish:
			continue
		}
		key := task.StepKey()
		if reason, manual := tokenPKManualReasons[key]; manual {
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionManual, Key: reason})
			continue
		}
		switch key {
		case "backup", "uploadPhoto", "aiCamera", "aiAssistant", "createNote", "shareFile", "openUrl":
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionRun, Key: key})
		case "reserveLogin":
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionReserve, Key: key})
		default:
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionManual, Key: "暂不支持的自动化步骤"})
		}
	}
	return actions
}

func (t *TokenPKTask) Run() error {
	t.logger.Start("------【算力大作战】------")
	t.api.PrepareActivitySession(api.TokenPKMarketName)

	tasks, err := t.api.TokenPKTaskList()
	if err != nil {
		t.logger.Error("获取算力大作战任务列表失败", err)
		return err
	}

	actions := planTokenPKActions(tasks)
	manualTasks := make([]string, 0, len(actions))
	failures := 0
	executed := 0

	for _, action := range actions {
		switch action.Kind {
		case tokenPKActionReceive:
			t.receiveTokenPKPrize(action.Task)
		case tokenPKActionRun:
			if err := t.executeTokenPKStep(action); err != nil {
				failures++
				t.logger.Debug(fmt.Sprintf("算力大作战任务 %s 执行失败: %v", action.Task.Name, err))
			} else {
				executed++
			}
		case tokenPKActionReserve:
			if err := t.api.TokenPKStepReserve(action.Task.ID, action.Key); err != nil {
				failures++
				t.logger.Debug(fmt.Sprintf("算力大作战任务 %s 预约失败: %v", action.Task.Name, err))
			} else {
				executed++
				t.logger.Success(fmt.Sprintf("算力大作战已预约: %s", action.Task.Name))
			}
		case tokenPKActionManual:
			manualTasks = append(manualTasks, fmt.Sprintf("%s(%s)", action.Task.Name, action.Key))
		}
	}

	// 执行动作后服务端需要时间刷新任务状态，重新拉取并领取新完成的任务。
	time.Sleep(3 * time.Second)
	claimed := t.receiveFinishedPrizes()

	chanceReward, chanceErr := t.api.TokenPKAutoReceiveLotteryChance()
	if chanceErr != nil {
		t.logger.Debug(fmt.Sprintf("自动领取抽奖次数失败: %v", chanceErr))
	}

	usedToken, _, progressErr := t.api.TokenPKProgressQueryHome()
	if progressErr != nil {
		t.logger.Debug(fmt.Sprintf("查询算力进度失败: %v", progressErr))
	}

	parts := make([]string, 0, 6)
	if claimed > 0 {
		parts = append(parts, fmt.Sprintf("领取奖励%d项", claimed))
	}
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("执行任务%d项", executed))
	}
	if chanceReward > 0 {
		parts = append(parts, fmt.Sprintf("抽奖次数+%d", chanceReward))
	}
	if usedToken > 0 {
		parts = append(parts, fmt.Sprintf("累计算力%s", formatTokenCount(usedToken)))
	}
	if len(manualTasks) > 0 {
		parts = append(parts, fmt.Sprintf("需手动: %s", strings.Join(manualTasks, "、")))
	}
	if failures > 0 && claimed == 0 && executed == 0 {
		t.lastMessage = strings.Join(parts, "; ")
		return fmt.Errorf("算力大作战执行失败(%d 个动作报错)", failures)
	}
	if len(parts) == 0 {
		parts = append(parts, "任务均已完成")
	}
	t.lastMessage = strings.Join(parts, "; ")
	t.logger.Success("算力大作战: " + t.lastMessage)
	return nil
}

func (t *TokenPKTask) receiveFinishedPrizes() int {
	tasks, err := t.api.TokenPKTaskList()
	if err != nil {
		t.logger.Debug(fmt.Sprintf("复查算力大作战任务列表失败: %v", err))
		return 0
	}
	claimed := 0
	for _, task := range tasks {
		if strings.EqualFold(task.State, activityStateSuccess) {
			if t.receiveTokenPKPrize(task) {
				claimed++
			}
		}
	}
	return claimed
}

func (t *TokenPKTask) receiveTokenPKPrize(task api.ActivityTask) bool {
	if err := t.api.TokenPKReceivePrize(task.ID); err != nil {
		t.logger.Debug(fmt.Sprintf("领取算力大作战奖励失败(%s): %v", task.Name, err))
		return false
	}
	prize := task.PrizeSummary()
	if prize == "" {
		prize = task.Name
	}
	t.logger.Success(fmt.Sprintf("算力大作战领取奖励: %s -> %s", task.Name, prize))
	return true
}

// executeTokenPKStep 先上报“去完成”点击，再执行真实动作（与 App 行为一致）。
func (t *TokenPKTask) executeTokenPKStep(action tokenPKAction) error {
	if err := t.api.TokenPKStepClick(action.Task.ID, action.Key); err != nil {
		return fmt.Errorf("上报任务点击失败: %w", err)
	}

	var err error
	switch action.Key {
	case "backup":
		err = t.performAlbumBackup()
	case "uploadPhoto":
		_, err = t.uploadPhotoFile("tokenpk_photo")
	case "aiCamera":
		err = t.performAICamera()
	case "aiAssistant":
		err = t.api.CompleteLingxiChat()
	case "createNote":
		err = t.createTempNote()
	case "shareFile":
		err = t.shareNewFile()
	case "openUrl":
		time.Sleep(time.Second)
	default:
		return fmt.Errorf("未知任务步骤 key: %s", action.Key)
	}
	if err != nil {
		return err
	}
	t.logger.Success(fmt.Sprintf("算力大作战完成任务动作: %s", action.Task.Name))
	return nil
}

func (t *TokenPKTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func formatTokenCount(tokens int64) string {
	switch {
	case tokens >= 100000000:
		return fmt.Sprintf("%.1f亿", float64(tokens)/100000000)
	case tokens >= 10000:
		return fmt.Sprintf("%d万", tokens/10000)
	default:
		return fmt.Sprintf("%d", tokens)
	}
}
