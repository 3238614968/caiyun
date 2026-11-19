package services

import (
	"fmt"
	"time"

	"caiyun/internal/core/tasks"
)

func miguTaskDefinitions() []TaskDefinition {
	var definitions []TaskDefinition
	for i, item := range []struct{ code, name, description string }{
		{"migu_login", "咪咕登录即送", "使用云盘账号换取独立咪咕会话，触发登录接入并复查完成状态，奖励由活动自动结算"},
		{"migu_signin", "咪咕每日签到", "按账号选择常规或七日新人礼签到，查询今日状态，未签时提交并复查"},
		{"migu_new_user", "咪咕新人礼", "符合资格时领取七日新人礼，已领或非新人账号自动跳过"},
		{"migu_community", "咪咕浏览社区", "执行当前金币中心社区频道任务并复查完成次数"},
		{"migu_mobile", "咪咕前往移动APP", "执行当前金币中心中移动APP任务并复查完成次数"},
		{"migu_noon", "咪咕饭点打卡", "北京时间12:00–13:00执行饭点打卡，窗口外保留待完成状态"},
		{"migu_night", "咪咕晚安打卡", "北京时间22:00–23:00执行晚安打卡，窗口外保留待完成状态"},
		{"migu_song", "咪咕听歌与领奖", "按参考协议补报至目标时长，复查后领取所有已达标SONG档位的AI音符"},
		{"migu_assets", "咪咕金币与音符", "核对AI音符余额并读取奖励提示；提示数非累计资产，不计入云朵收益"},
	} {
		code := item.code
		definitions = append(definitions, TaskDefinition{
			Code: code, Name: item.name, Description: item.description,
			SortOrder: 180 + i, DefaultEnabled: true, RunInBatch: true,
			execute: func(r *TaskRunner) *TaskResult { return r.runMiguTask(code) },
		})
	}
	return definitions
}

func (r *TaskRunner) runMiguTask(code string) *TaskResult {
	started := time.Now()
	if r.account == nil {
		return activityTaskResult(code, started, fmt.Errorf("咪咕任务缺少账号上下文"), "", "")
	}
	if r.miguTask == nil {
		r.miguTask = tasks.NewMiguTask(r.httpClient).SetStorage(r.storage).SetAccountContext(r.account.Phone, r.account.Auth)
	}
	err := r.miguTask.Run(code)
	result := activityTaskResult(code, started, err, r.miguTask.Message(), "咪咕任务执行完成")
	result.MiguNotesGained = r.miguTask.NotesGained()
	if err == nil && r.miguTask.Pending() {
		result.Status = "pending"
	}
	return result
}
