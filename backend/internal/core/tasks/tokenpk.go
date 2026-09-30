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

// TokenPKTask 算力大作战（National_TokenPK）。
type TokenPKTask struct {
	*activityActions
	lastMessage string
	pending     bool
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
		// Completed rewards stay claimable even after the monthly cap is reached.
		if task.MonthlyLimit > 0 && task.MonthlyCompleted >= task.MonthlyLimit {
			continue
		}
		key := task.StepKey()
		if key == "inviteFriend" {
			// The cross-account mutual_assist task handles this step.
			continue
		}
		switch key {
		case "backup", "uploadPhoto", "aiCamera", "aiAssistant", "createNote", "shareFile", "openUrl", "loginPc":
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
	t.pending = false
	var issues []error
	attempted := make(map[int]bool)
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
	initialClaimed := 0

	for _, action := range actions {
		switch action.Kind {
		case tokenPKActionReceive:
			if err := t.receiveTokenPKPrize(action.Task); err != nil {
				issues = append(issues, err)
			} else {
				initialClaimed++
			}
		case tokenPKActionRun:
			if err := t.executeTokenPKStep(action); err != nil {
				failures++
				issues = append(issues, err)
				t.logger.Debug(fmt.Sprintf("算力大作战任务 %s 执行失败: %v", action.Task.Name, err))
			} else {
				executed++
				attempted[action.Task.ID] = true
			}
		case tokenPKActionReserve:
			if err := t.api.TokenPKStepReserve(action.Task.ID, action.Key); err != nil {
				failures++
				issues = append(issues, err)
				t.logger.Debug(fmt.Sprintf("算力大作战任务 %s 预约失败: %v", action.Task.Name, err))
			} else {
				executed++
				attempted[action.Task.ID] = true
				t.logger.Success(fmt.Sprintf("算力大作战已预约: %s", action.Task.Name))
			}
		case tokenPKActionManual:
			manualTasks = append(manualTasks, fmt.Sprintf("%s(%s)", action.Task.Name, action.Key))
		}
	}

	// 执行动作后服务端需要时间刷新任务状态，重新拉取并领取新完成的任务。
	time.Sleep(3 * time.Second)
	claimed, claimErr := t.receiveFinishedPrizes(attempted)
	claimed += initialClaimed
	if claimErr != nil {
		issues = append(issues, claimErr)
	}
	t.pending = len(attempted) > 0

	chanceReward, chanceErr := t.api.TokenPKAutoReceiveLotteryChance()
	if chanceErr != nil {
		issues = append(issues, chanceErr)
		t.logger.Debug(fmt.Sprintf("自动领取抽奖次数失败: %v", chanceErr))
	}

	usedToken, _, progressErr := t.api.TokenPKProgressQueryHome()
	if progressErr != nil {
		issues = append(issues, progressErr)
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
	if failures > 0 {
		parts = append(parts, fmt.Sprintf("%d项步骤失败", failures))
	}
	if t.pending {
		parts = append(parts, fmt.Sprintf("%d项待服务端确认或跨月完成", len(attempted)))
	}
	if len(parts) == 0 {
		parts = append(parts, "本轮无可执行步骤（已完成、达到月上限或待好友互助）")
	}
	t.lastMessage = strings.Join(parts, "; ")
	if err := errors.Join(issues...); err != nil {
		return err
	}
	t.logger.Success("算力大作战: " + t.lastMessage)
	return nil
}

func (t *TokenPKTask) receiveFinishedPrizes(attempted map[int]bool) (int, error) {
	tasks, err := t.api.TokenPKTaskList()
	if err != nil {
		t.logger.Debug(fmt.Sprintf("复查算力大作战任务列表失败: %v", err))
		return 0, err
	}
	claimed := 0
	var issues []error
	for _, task := range tasks {
		if strings.EqualFold(task.State, activityStateFinish) {
			delete(attempted, task.ID)
		}
		if strings.EqualFold(task.State, activityStateSuccess) {
			if err := t.receiveTokenPKPrize(task); err == nil {
				claimed++
				delete(attempted, task.ID)
			} else {
				issues = append(issues, err)
			}
		}
	}
	return claimed, errors.Join(issues...)
}

func (t *TokenPKTask) receiveTokenPKPrize(task api.ActivityTask) error {
	if err := t.api.TokenPKReceivePrize(task.ID); err != nil {
		t.logger.Debug(fmt.Sprintf("领取算力大作战奖励失败(%s): %v", task.Name, err))
		return fmt.Errorf("算力领奖/%d: %w", task.ID, err)
	}
	prize := task.PrizeSummary()
	if prize == "" {
		prize = task.Name
	}
	t.logger.Success(fmt.Sprintf("算力大作战领取奖励: %s -> %s", task.Name, prize))
	return nil
}

// executeTokenPKStep performs the real action before registering its server step.
func (t *TokenPKTask) executeTokenPKStep(action tokenPKAction) error {
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
	case "loginPc":
		_, _, err = t.uploadTextFileWithChannel("tokenpk_pc", "10200153")
	case "openUrl":
		time.Sleep(time.Second)
	default:
		return fmt.Errorf("未知任务步骤 key: %s", action.Key)
	}
	if err != nil {
		return err
	}
	if err := t.api.TokenPKStepClick(action.Task.ID, action.Key); err != nil {
		return fmt.Errorf("登记任务步骤失败: %w", err)
	}
	t.logger.Success(fmt.Sprintf("算力大作战完成任务动作: %s", action.Task.Name))
	return nil
}

func (t *TokenPKTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}

func (t *TokenPKTask) Pending() bool { return t.pending }

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
