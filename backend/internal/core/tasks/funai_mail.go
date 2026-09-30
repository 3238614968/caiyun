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

// FunAIMailTask（National_playAI139mail 趣玩AI 邮箱版）。
//
// 与云盘版共用同一后端，任务表也一致；邮箱版只开放
// `page/register`、`page/lottery`、`invite/*` 三个动作。
// 因此这里做「登记 + 抽奖 + 邀请码」三件事，AI 工具的动作由 fun_ai 任务负责。
type FunAIMailTask struct {
	api         *api.CaiyunAPI
	logger      *logger.Logger
	lastMessage string
}

// NewFunAIMailTask 创建趣玩AI 邮箱版任务。
func NewFunAIMailTask(client *http.Client, log *logger.Logger) *FunAIMailTask {
	return &FunAIMailTask{api: api.NewCaiyunAPI(client), logger: log}
}

// Message 返回最近一次执行的结果描述。
func (t *FunAIMailTask) Message() string { return strings.TrimSpace(t.lastMessage) }

// Run 登记未完成任务并抽奖。
func (t *FunAIMailTask) Run() error {
	var issues []error
	var parts []string

	t.api.PrepareActivitySession(api.FunAIMailMarketName)

	module, err := t.api.FunaiPageModuleForMarket(api.FunAIMailMarketName)
	if err != nil {
		return fmt.Errorf("获取趣玩AI邮箱版模块: %w", err)
	}

	registered := 0
	for _, task := range module.TaskIDs {
		if task.IsComplete == 1 || task.TaskSign != 0 {
			continue
		}
		if err := t.api.FunaiRegisterForMarket(api.FunAIMailMarketName, module.ModuleID, task.ID); err != nil {
			issues = append(issues, fmt.Errorf("趣玩AI邮箱版登记(%s): %w", task.ID, err))
			continue
		}
		registered++
	}
	parts = append(parts, fmt.Sprintf("邮箱版登记%d项", registered))

	// 邮箱版共用同一份抽奖次数，连续抽到次数用尽为止。
	drew := 0
	prizes := make([]string, 0, 4)
	for round := 0; round < funaiMaxLotteries; round++ {
		prize, err := t.api.FunaiLotteryForMarket(api.FunAIMailMarketName, module.ModuleID)
		if err != nil {
			if !isNoLotteryChanceError(err) {
				issues = append(issues, fmt.Errorf("趣玩AI邮箱版抽奖: %w", err))
			}
			break
		}
		name := strings.TrimSpace(prize.PrizeName)
		if name == "" {
			name = "谢谢参与"
		}
		prizes = append(prizes, name)
		drew++
		time.Sleep(time.Second)
	}
	if drew > 0 {
		parts = append(parts, fmt.Sprintf("抽奖%d次: %s", drew, strings.Join(prizes, "、")))
	} else {
		parts = append(parts, "暂无可用抽奖次数")
	}

	t.lastMessage = strings.Join(parts, "；")
	if t.logger != nil {
		t.logger.Info(t.lastMessage)
	}
	return errors.Join(issues...)
}
