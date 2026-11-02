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
	lastMessage       string
	pending           bool
	unavailablePrizes map[int]bool
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
		if task.ID == 11 {
			actions = append(actions, tokenPKAction{Task: task, Kind: tokenPKActionRun, Key: "smartAlbum"})
			continue
		}
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
	t.unavailablePrizes = make(map[int]bool)
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
				if !t.deferUnavailablePrize(action.Task.ID, err) {
					issues = append(issues, err)
				}
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
	t.pending = len(attempted) > 0 || len(t.unavailablePrizes) > 0

	usedToken, stages, progressErr := t.api.TokenPKProgressQueryHome()
	if progressErr != nil {
		issues = append(issues, progressErr)
		t.logger.Debug(fmt.Sprintf("查询算力进度失败: %v", progressErr))
	}

	parts := make([]string, 0, 6)
	if progressErr == nil {
		count, err := t.receiveProgressRewards(usedToken, stages)
		if err != nil {
			issues = append(issues, err)
		}
		if count > 0 {
			parts = append(parts, fmt.Sprintf("阶段奖励领取%d项", count))
		}
	}
	chanceReward, chanceErr := t.api.TokenPKAutoReceiveLotteryChance()
	if chanceErr != nil {
		if t.deferUnavailablePrize(-1, chanceErr) {
			t.pending = true
		} else {
			issues = append(issues, chanceErr)
		}
		t.logger.Debug(fmt.Sprintf("自动领取抽奖次数失败: %v", chanceErr))
	}

	month := time.Now().In(time.FixedZone("CST", 8*3600)).Format("200601")
	if rank, err := t.api.TokenPKRank(month); err != nil {
		issues = append(issues, err)
	} else if err := rewardResponseError("算力榜单", rank); err != nil {
		issues = append(issues, err)
	} else if result, ok := rank.Result.(map[string]interface{}); ok {
		parts = append(parts, fmt.Sprintf("榜单%s：排名%v、算力%v", month, result["myRank"], result["myToken"]))
	}
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
	if len(attempted) > 0 {
		parts = append(parts, fmt.Sprintf("%d项待服务端确认或周期结算", len(attempted)))
	}
	if len(t.unavailablePrizes) > 0 {
		parts = append(parts, fmt.Sprintf("%d项奖励暂不可领（奖品发放失败/已耗尽），保留待领取状态", len(t.unavailablePrizes)))
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
			delete(t.unavailablePrizes, task.ID)
		}
		if strings.EqualFold(task.State, activityStateSuccess) {
			if t.unavailablePrizes[task.ID] {
				continue
			}
			if err := t.receiveTokenPKPrize(task); err == nil {
				claimed++
				delete(attempted, task.ID)
			} else {
				if !t.deferUnavailablePrize(task.ID, err) {
					issues = append(issues, err)
				}
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
	case "smartAlbum":
		return t.api.TokenPKSmartAlbumOpen()
	case "backup":
		err = t.performAlbumBackup()
	case "uploadPhoto":
		_, err = t.uploadPhotoFile("tokenpk_photo")
	case "aiCamera":
		err = t.performAICamera()
	case "aiAssistant":
		// 新版13号任务需要带图对话，纯文本请求不能推进该任务状态。
		if action.Task.ID == 13 {
			err = t.performAICamera()
		} else {
			err = t.api.CompleteLingxiChat()
		}
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

// isTokenPKPrizeUnavailable 判断领奖失败是否为 9001“奖品发放失败”。
// 该码既表示当日奖池/库存耗尽，也会在任务已 FINISH 重复领取时返回；
// 语义是“当前无可领”，不是任务失败，保留待领状态等次日刷新即可。
func isTokenPKPrizeUnavailable(err error) bool {
	var businessErr *api.ActivityBusinessError
	return errors.As(err, &businessErr) && businessErr.Code == "9001"
}

func (t *TokenPKTask) deferUnavailablePrize(id int, err error) bool {
	if !isTokenPKPrizeUnavailable(err) {
		return false
	}
	if t.unavailablePrizes == nil {
		t.unavailablePrizes = make(map[int]bool)
	}
	t.unavailablePrizes[id] = true
	return true
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
