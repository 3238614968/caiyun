package services

import (
	"time"

	"caiyun/internal/core/tasks"
)

// 本文件承载「活动覆盖度补齐」新增的任务执行入口。所有任务都复用账号级
// HTTP 客户端，因此天然支持多账号隔离与既有的熔断/重试/链路追踪语义。

// runNoticeSwitchTask 上报 APP 通知状态并开启邮箱短信通知开关。
func (r *TaskRunner) runNoticeSwitchTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewNoticeSwitchTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("notice_switch", startTime, err, task.Message(), "通知与开关状态已同步")
}

// runStudentPerksTask 同步并查询学生认证福利状态。
func (r *TaskRunner) runStudentPerksTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewStudentPerksTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("student_perks", startTime, err, task.Message(), "学生认证福利状态已同步")
}

// runPrizeCenterTask 盘点领奖专区未领奖品。
func (r *TaskRunner) runPrizeCenterTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewPrizeCenterTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("prize_center", startTime, err, task.Message(), "领奖专区盘点完成")
}

// runMCloudDayTask 执行会员日领礼与盲盒抽奖。
func (r *TaskRunner) runMCloudDayTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewMCloudDayTask(r.httpClient, r.logger)
	err := task.Run()
	return activityTaskResult("mcloud_day", startTime, err, task.Message(), "会员日任务执行完成")
}

// runMeituBackupTask 执行美图授权备份领好礼。
func (r *TaskRunner) runMeituBackupTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewMeituBackupTask(r.httpClient, r.logger)
	err := task.Run()
	return activityTaskResult("meitu_backup", startTime, err, task.Message(), "美图备份任务执行完成")
}

// runRedInviteTask 执行红包邀请（生成/接受邀请码）。
func (r *TaskRunner) runRedInviteTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewRedInviteTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("red_invite", startTime, err, task.Message(), "红包邀请任务执行完成")
}

// runUnloading1TTask 探测 1T 新礼活动状态。
func (r *TaskRunner) runUnloading1TTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewUnloading1TTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("unloading_1t", startTime, err, task.Message(), "1T 新礼状态已探测")
}

// runRafflecodeTask 查询抽奖码场次与记录。
func (r *TaskRunner) runRafflecodeTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewRafflecodeTask(r.httpClient, r.logger)
	err := task.Run()
	return activityTaskResult("rafflecode", startTime, err, task.Message(), "抽奖码状态已查询")
}

// runFamilyCircleTask 查询家庭圈任务状态。
func (r *TaskRunner) runFamilyCircleTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewFamilyCircleTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("family_circle", startTime, err, task.Message(), "家庭圈任务状态已查询")
}

// runAIStoreTask 探测 AI Store 授权并在配置时保存作品。
func (r *TaskRunner) runAIStoreTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewAIStoreTask(r.httpClient, r.logger)
	task.SetAccountPhone(r.account.Phone)
	err := task.Run()
	return activityTaskResult("ai_store", startTime, err, task.Message(), "AI Store 任务执行完成")
}

// runAlbumBackupReportTask 查询相册备份状态，并在配置了真实开关值时上报。
func (r *TaskRunner) runAlbumBackupReportTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewAlbumBackupReportTask(r.httpClient, r.logger)
	if r.account != nil {
		task.SetAccountContext(r.account.Phone, r.getRawAccountToken())
	}
	err := task.Run()
	return activityTaskResult("album_backup_report", startTime, err, task.Message(), "相册备份状态已同步")
}

// runUpgradeGiftTask 读取焕新权益奖池并激活已获得的权益。
func (r *TaskRunner) runUpgradeGiftTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewUpgradeGiftTask(r.httpClient, r.logger)
	err := task.Run()
	return activityTaskResult("upgrade_gift", startTime, err, task.Message(), "焕新权益任务执行完成")
}

// runFunAIMailTask 执行趣玩AI 邮箱版的登记与抽奖。
func (r *TaskRunner) runFunAIMailTask() *TaskResult {
	startTime := time.Now()
	task := tasks.NewFunAIMailTask(r.httpClient, r.logger)
	err := task.Run()
	return activityTaskResult("fun_ai_mail", startTime, err, task.Message(), "趣玩AI 邮箱版任务执行完成")
}
