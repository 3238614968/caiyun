package tasks

import (
	"fmt"
	"strings"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// PosterTask 校园海报·AI体验活动（National_PlayAISpecial，newyear 接口族）。
// “体验AI相机”可全自动；“生成校园海报”需要用户在海报页真实生成，
// 抓包中未见海报生成接口，只能完成点击并提示。
type PosterTask struct {
	*activityActions
	lastMessage string
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
	t.logger.Start("------【校园海报活动】------")
	t.api.PrepareActivitySession(api.PosterMarketName)

	tasks, err := t.api.NewYearTaskList(api.PosterMarketName)
	if err != nil {
		t.logger.Error("获取校园海报任务列表失败", err)
		return err
	}

	manualNames := make([]string, 0, len(tasks))
	executed := 0
	for _, task := range tasks {
		if strings.EqualFold(task.State, activityStateFinish) {
			continue
		}
		key := task.StepKey()
		if key == "" {
			key = "click"
		}
		switch {
		case task.Flag == "makingPoster" || strings.Contains(task.Name, "海报"):
			if clickErr := t.api.NewYearStepClick(api.PosterMarketName, task.ID, key); clickErr != nil {
				t.logger.Debug(fmt.Sprintf("海报任务点击失败: %v", clickErr))
			}
			manualNames = append(manualNames, task.Name+"(需在海报页生成)")
		case strings.Contains(task.Name, "AI相机"):
			if err := t.completePosterCameraTask(task.ID, key); err != nil {
				t.logger.Debug(fmt.Sprintf("AI相机体验任务失败: %v", err))
			} else {
				executed++
			}
		case strings.EqualFold(task.State, activityStateSuccess):
			manualNames = append(manualNames, task.Name+"(待领取)")
		default:
			if clickErr := t.api.NewYearStepClick(api.PosterMarketName, task.ID, key); clickErr != nil {
				t.logger.Debug(fmt.Sprintf("校园海报任务点击失败(%s): %v", task.Name, clickErr))
			}
		}
	}

	time.Sleep(2 * time.Second)
	chances, err := t.api.NewYearLotteryCount(api.PosterMarketName)
	if err != nil {
		t.logger.Debug(fmt.Sprintf("查询校园海报抽奖次数失败: %v", err))
	}
	prizes := t.runPosterLotteries(chances)

	parts := make([]string, 0, 4)
	if executed > 0 {
		parts = append(parts, fmt.Sprintf("完成AI体验任务%d项", executed))
	}
	if len(prizes) > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", len(prizes), strings.Join(prizes, "、")))
	}
	if len(manualNames) > 0 {
		parts = append(parts, "待完成: "+strings.Join(manualNames, "、"))
	}
	if len(parts) == 0 {
		parts = append(parts, "任务均已完成")
	}
	t.lastMessage = strings.Join(parts, "; ")
	t.logger.Success("校园海报活动: " + t.lastMessage)
	return nil
}

// completePosterCameraTask 点击任务入口后走一遍真实的 AI 相机识别+对话。
func (t *PosterTask) completePosterCameraTask(taskID int, key string) error {
	if err := t.api.NewYearStepClick(api.PosterMarketName, taskID, key); err != nil {
		return fmt.Errorf("上报任务点击失败: %w", err)
	}
	if err := t.api.CompleteAICameraTask(); err != nil {
		return err
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
