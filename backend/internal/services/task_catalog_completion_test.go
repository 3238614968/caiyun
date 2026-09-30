package services

import "testing"

// 校验「活动覆盖度补齐」新增任务已注册，并符合预期的默认启停策略：
// 只读/前置类任务默认进批次；涉及设备状态上报或活动侧风控的写操作默认关闭。
func TestNewCompletionTasksDefaults(t *testing.T) {
	catalog := NewTaskCatalog()

	enabledInBatch := []string{"notice_switch", "prize_center", "student_perks"}
	for _, code := range enabledInBatch {
		def, ok := catalog.Get(code)
		if !ok {
			t.Fatalf("task %q is not registered", code)
		}
		if !def.DefaultEnabled || !def.RunInBatch {
			t.Fatalf("task %q should default to enabled+in-batch, got %+v", code, def)
		}
	}

	disabled := []string{
		"mcloud_day",
		"meitu_backup",
		"red_invite",
		"unloading_1t",
		"rafflecode",
		"family_circle",
		"ai_store",
		"album_backup_report",
		"upgrade_gift",
		"fun_ai_mail",
	}
	for _, code := range disabled {
		def, ok := catalog.Get(code)
		if !ok {
			t.Fatalf("task %q is not registered", code)
		}
		if def.DefaultEnabled || def.RunInBatch {
			t.Fatalf("task %q should default to disabled+out-of-batch, got %+v", code, def)
		}
	}
}

// 别名应能解析到规范任务码，保证历史配置与前端传参兼容。
func TestCompletionTaskAliases(t *testing.T) {
	catalog := NewTaskCatalog()
	aliases := map[string]string{
		"getprize":     "prize_center",
		"studentperks": "student_perks",
		"v13gift":      "upgrade_gift",
		"familycircle": "family_circle",
		"meitu":        "meitu_backup",
		"redinvite":    "red_invite",
		"newgifts1t":   "unloading_1t",
		"aistore":      "ai_store",
		"albumbackup":  "album_backup_report",
		"mcloudday":    "mcloud_day",
		"funai_mail":   "fun_ai_mail",
		"appnotice":    "notice_switch",
	}
	for alias, want := range aliases {
		if got := catalog.Normalize(alias); got != want {
			t.Fatalf("Normalize(%q) = %q, want %q", alias, got, want)
		}
	}
}

// 每个新增任务都必须有可执行的 executor，避免注册表里出现空实现。
func TestCompletionTasksHaveExecutors(t *testing.T) {
	catalog := NewTaskCatalog()
	codes := []string{
		"notice_switch", "fun_ai_mail", "prize_center", "upgrade_gift",
		"student_perks", "family_circle", "meitu_backup", "red_invite",
		"unloading_1t", "rafflecode", "ai_store", "album_backup_report", "mcloud_day",
	}
	for _, code := range codes {
		def, ok := catalog.Get(code)
		if !ok {
			t.Fatalf("task %q is not registered", code)
		}
		if def.execute == nil {
			t.Fatalf("task %q has no executor", code)
		}
		if def.Name == "" || def.Description == "" {
			t.Fatalf("task %q is missing name/description: %+v", code, def)
		}
	}
}
