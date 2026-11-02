package services

import (
	"caiyun/internal/core/tasks"
	"time"
)

func autumnTaskDefinitions() []TaskDefinition {
	defs := []TaskDefinition{}
	for i, item := range []struct{ code, name, description string }{
		{"annual_bill", "年度账单活动", "浏览年度报告、回忆相册、智能相册及可用次数抽奖；问卷使用账号配置答案"},
		{"luckybag", "每日开福袋", "查询号码资格和可用次数，开福袋后复查剩余次数"},
		{"redcavalry", "红色骑兵闯关", "阅读卡片、完成互动与答题，复查徽章后完成回顾"},
		{"fun_ai_timed_lottery", "趣玩AI限时抽奖", "读取当前限时模块，为已完成任务领取抽奖机会"},
	} {
		code := item.code
		defs = append(defs, TaskDefinition{Code: code, Name: item.name, Description: item.description, SortOrder: 161 + i, DefaultEnabled: true, RunInBatch: true, execute: func(r *TaskRunner) *TaskResult { return r.runAutumnTask(code) }})
	}
	defs = append(defs, TaskDefinition{Code: "tasklist_gp", Name: "GP渠道任务中心", Description: "独立签到并推进GP网关两个渠道的任务，按服务端状态记录延迟结算", SortOrder: 164, DefaultEnabled: true, RunInBatch: true, execute: func(r *TaskRunner) *TaskResult { return r.runGPTaskList() }})
	defs = append(defs, TaskDefinition{Code: "redeem_code", Name: "活动兑换码", Description: "使用账号提供的兑换码领取权益，默认关闭", SortOrder: 164, DefaultEnabled: false, RunInBatch: false, execute: func(r *TaskRunner) *TaskResult { return r.runAutumnTask("redeem_code") }})
	return defs
}
func (r *TaskRunner) runAutumnTask(code string) *TaskResult {
	started := time.Now()
	task := tasks.NewAutumnTask(r.httpClient, code)
	if r.account != nil {
		task.SetAccountPhone(r.account.Phone)
	}
	err := task.Run()
	result := activityTaskResult(code, started, err, task.Message(), "活动执行完成")
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}
func (r *TaskRunner) runGPTaskList() *TaskResult {
	started := time.Now()
	task := tasks.NewGPTaskListTask(r.httpClient, r.logger).SetStorage(r.storage)
	if r.account != nil {
		task.SetAccountContext(r.account.Phone, r.getRawAccountToken())
	}
	err := task.Run()
	result := activityTaskResult("tasklist_gp", started, err, task.Message(), "GP渠道巡检完成")
	if err == nil && task.Pending() {
		result.Status = "pending"
	}
	return result
}
