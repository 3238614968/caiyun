package tasks

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// PosterTask 校园海报·AI体验活动（National_PlayAISpecial，newyear 接口族）。
// 海报经预签名上传完成；AI 相机执行真实识图和对话。
type PosterTask struct {
	*activityActions
	lastMessage string
	pending     bool
}

func NewPosterTask(client *http.Client, log *logger.Logger) *PosterTask {
	return &PosterTask{activityActions: newActivityActions(client, log)}
}

func (t *PosterTask) SetStorage(store Storage) *PosterTask {
	t.setStorage(store)
	return t
}

func (t *PosterTask) SetAccountContext(phone, authToken string) *PosterTask {
	t.setAccountContext(phone, authToken)
	return t
}

const posterMaxLotteries = 5

func (t *PosterTask) Run() error {
	t.pending = false
	t.logger.Start("------【校园海报活动】------")
	t.api.PrepareActivitySession(api.PosterMarketName)

	tasks, err := t.api.NewYearTaskList(api.PosterMarketName)
	if err != nil {
		t.logger.Error("获取校园海报任务列表失败", err)
		return err
	}

	initialPending := make(map[int]bool)
	for _, task := range tasks {
		if !strings.EqualFold(task.State, activityStateFinish) {
			initialPending[task.ID] = true
		}
	}
	manualNames := make(map[string]bool)
	executed := 0
	failedActions := 0
	blocked := make(map[int]bool)
	var actionIssues []string
	acted := make(map[string]bool)
	for round := 0; round < 3; round++ {
		if round > 0 {
			refreshed, err := t.api.NewYearTaskList(api.PosterMarketName)
			if err != nil {
				t.logger.Debug(fmt.Sprintf("刷新校园海报任务列表失败: %v", err))
				break
			}
			tasks = refreshed
		}
		pending := false
		for _, task := range tasks {
			if strings.EqualFold(task.State, activityStateFinish) {
				continue
			}
			if blocked[task.ID] {
				continue
			}
			key := task.StepKey()
			if key == "" {
				key = "click"
			}
			stepIdentity := fmt.Sprintf("%d:%s", task.ID, key)
			if acted[stepIdentity] {
				continue
			}
			switch {
			case task.ID == 33 || task.Flag == "makingPoster":
				pending = true
				acted[stepIdentity] = true
				if err := t.api.CompletePosterTask(); err != nil {
					failedActions++
					blocked[task.ID] = true
					actionIssues = append(actionIssues, fmt.Sprintf("海报任务%d: %v", task.ID, err))
					t.logger.Warn(fmt.Sprintf("生成校园海报失败: %v", err))
					continue
				}
				executed++
				// Some server versions finish on upload completion; others still
				// need the activity step registered once afterward.
				if latest, err := t.api.NewYearTaskList(api.PosterMarketName); err == nil && !posterTaskFinished(latest, task.ID) {
					if clickErr := t.api.NewYearStepClick(api.PosterMarketName, task.ID, key); clickErr != nil {
						t.logger.Debug(fmt.Sprintf("海报任务步骤登记失败: %v", clickErr))
					}
				}
			case task.ID == 34 || strings.Contains(task.Name, "AI相机"):
				pending = true
				acted[stepIdentity] = true
				if err := t.completePosterCameraTask(task.ID, key); err != nil {
					failedActions++
					blocked[task.ID] = true
					actionIssues = append(actionIssues, fmt.Sprintf("AI任务%d: %v", task.ID, err))
					t.logger.Warn(fmt.Sprintf("AI相机体验任务失败: %v", err))
				} else {
					executed++
				}
			default:
				manualNames[task.Name] = true
			}
		}
		if !pending {
			break
		}
		if round < 2 {
			time.Sleep(350 * time.Millisecond)
		}
	}

	chances, err := t.api.NewYearLotteryCount(api.PosterMarketName)
	if err != nil {
		t.logger.Debug(fmt.Sprintf("查询校园海报抽奖次数失败: %v", err))
	}
	prizes := t.runPosterLotteries(chances)

	confirmed := 0
	if latest, err := t.api.NewYearTaskList(api.PosterMarketName); err == nil {
		for _, task := range latest {
			if !strings.EqualFold(task.State, activityStateFinish) {
				t.pending = true
			}
			if initialPending[task.ID] && strings.EqualFold(task.State, activityStateFinish) {
				confirmed++
			}
		}
	} else {
		return fmt.Errorf("校园海报状态确认失败: %w", err)
	}
	parts := make([]string, 0, 4)
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("执行海报/AI动作%d次，服务端新确认%d项", executed, confirmed))
	}
	if len(prizes) > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", len(prizes), strings.Join(prizes, "、")))
	}
	if len(manualNames) > 0 {
		var names []string
		for name := range manualNames {
			names = append(names, name)
		}
		sort.Strings(names)
		parts = append(parts, "待完成: "+strings.Join(names, "、"))
	}
	if failedActions > 0 {
		parts = append(parts, fmt.Sprintf("%d次自动任务执行失败: %s", failedActions, strings.Join(actionIssues, "、")))
	}
	if t.pending {
		parts = append(parts, "仍有任务待完成或待服务端确认")
	}
	if len(parts) == 0 {
		parts = append(parts, "任务均已完成")
	}
	t.lastMessage = strings.Join(parts, "; ")
	if failedActions > 0 {
		return fmt.Errorf("校园海报活动未完成: %s", t.lastMessage)
	}
	t.logger.Success("校园海报活动: " + t.lastMessage)
	return nil
}

func (t *PosterTask) Pending() bool { return t.pending }

func posterTaskFinished(items []api.ActivityTask, taskID int) bool {
	for _, task := range items {
		if task.ID == taskID {
			return strings.EqualFold(task.State, activityStateFinish)
		}
	}
	return false
}

// completePosterCameraTask performs AI recognition before registering the step.
func (t *PosterTask) completePosterCameraTask(taskID int, key string) error {
	if err := t.api.CompleteAICameraTask(); err != nil {
		return err
	}
	if err := t.api.NewYearStepClick(api.PosterMarketName, taskID, key); err != nil {
		return fmt.Errorf("登记 AI 相机任务步骤失败: %w", err)
	}
	t.logger.Success("校园海报活动完成AI相机体验任务")
	return nil
}

func (t *PosterTask) runPosterLotteries(chances int) []string {
	if chances > posterMaxLotteries {
		chances = posterMaxLotteries
	}
	prizes := make([]string, 0, chances)
	for round := 0; round < chances; round++ {
		name, err := t.api.NewYearLottery(api.PosterMarketName)
		if err != nil {
			t.logger.Debug(fmt.Sprintf("校园海报活动抽奖失败: %v", err))
			break
		}
		if strings.TrimSpace(name) == "" {
			name = "谢谢参与"
		}
		prizes = append(prizes, name)
		time.Sleep(time.Second)
	}
	return prizes
}

func (t *PosterTask) Message() string {
	return strings.TrimSpace(t.lastMessage)
}
