package services

import (
	"fmt"
	"sort"
	"strings"

	"caiyun/internal/models"
)

type taskExecutor func(*TaskRunner) *TaskResult

type TaskDefinition struct {
	Code           string
	Name           string
	Description    string
	SortOrder      int
	DefaultEnabled bool
	RunInBatch     bool
	Aliases        []string
	execute        taskExecutor
}

type TaskCatalog struct {
	definitions map[string]TaskDefinition
	aliases     map[string]string
}

var defaultTaskCatalog = NewTaskCatalog()

func NewTaskCatalog() *TaskCatalog {
	defs := []TaskDefinition{
		{
			Code:           "signin",
			Name:           "每日签到",
			Description:    "执行移动云盘每日签到",
			SortOrder:      10,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"daily_checkin"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runSignInTask()
			},
		},
		{
			Code:           "task_expansion_reward",
			Name:           "备份翻倍奖励",
			Description:    "检查并领取签到后的备份翻倍奖励",
			SortOrder:      20,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"taskexpansion", "task_expansion"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runTaskExpansionRewardTask()
			},
		},
		{
			Code:           "cloud_multiple",
			Name:           "云朵翻倍",
			Description:    "领取新版签到页云朵翻倍奖励",
			SortOrder:      25,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"cloudmultiple", "multiple"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runCloudMultipleTask()
			},
		},
		{
			Code:           "wechat",
			Name:           "微信签到",
			Description:    "执行微信公众号签到任务",
			SortOrder:      30,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"wechat_checkin"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runWeChatTask()
			},
		},
		{
			Code:           "wxdraw",
			Name:           "微信抽奖",
			Description:    "执行微信公众号抽奖任务",
			SortOrder:      40,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"lottery"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runWxDrawTask()
			},
		},
		{
			Code:           "tasklist",
			Name:           "任务中心巡检",
			Description:    "执行任务中心自动点击、上传、分享、笔记和领奖流程",
			SortOrder:      50,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"task_list_sync"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runTaskListTask()
			},
		},
		{
			Code:           "mail_mutual",
			Name:           "139邮箱账号互发",
			Description:    "同一用户下的活跃账号两两互发邮件，每个有序账号对每月最多一次",
			SortOrder:      55,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"mail139_mutual"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMailMutualTask()
			},
		},
		{
			Code:           "invitefriends",
			Name:           "邀请好友看电影",
			Description:    "执行分享邀请并领取对应云朵奖励",
			SortOrder:      60,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"share_find"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runInviteFriendsTask()
			},
		},
		{
			Code:           "shake",
			Name:           "摇一摇",
			Description:    "活动接口已下线，默认不再纳入任务批次",
			SortOrder:      70,
			DefaultEnabled: false,
			RunInBatch:     false,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runShakeTask()
			},
		},
		{
			Code:           "receive",
			Name:           "领取云朵",
			Description:    "领取当前账号可领取的云朵奖励",
			SortOrder:      80,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"receive_cloud"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runReceiveTask()
			},
		},
		{
			Code:           "messagepush",
			Name:           "消息推送奖励",
			Description:    "检查并领取消息推送奖励",
			SortOrder:      90,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"msg_push_reward"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMessagePushTask()
			},
		},
		{
			Code:           "revivalreward",
			Name:           "复活卡奖励",
			Description:    "检查并领取复活卡奖励",
			SortOrder:      95,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"revival_reward", "receive_revival_reward"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runRevivalRewardTask()
			},
		},

		{
			Code:           "backupgift",
			Name:           "备份礼包",
			Description:    "执行备份礼包奖励领取流程",
			SortOrder:      100,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"backup_gift"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runBackupGiftTask()
			},
		},
		{
			Code:           "garden",
			Name:           "果园",
			Description:    "果园活动已下架，默认不再纳入任务批次",
			SortOrder:      110,
			DefaultEnabled: false,
			RunInBatch:     false,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runGardenTask()
			},
		},
		{
			Code:           "redpacket",
			Name:           "AI红包",
			Description:    "活动已下架，默认不再纳入任务批次",
			SortOrder:      120,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"ai_redpack"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runRedPacketTask()
			},
		},
		{
			Code:           "aicloud",
			Name:           "AI云朵",
			Description:    "活动已下架，默认不再纳入任务批次",
			SortOrder:      130,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"ai_cloud"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runAiCloudTask()
			},
		},
		{
			Code:           "cloudbattle",
			Name:           "云朵大作战",
			Description:    "合成 1T 接口当前不可达，默认不再纳入任务批次",
			SortOrder:      140,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"hecheng1t"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runCloudBattleTask()
			},
		},
		{
			Code:           "blindbox",
			Name:           "盲盒",
			Description:    "活动已下架，默认不再纳入任务批次",
			SortOrder:      150,
			DefaultEnabled: false,
			RunInBatch:     false,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runBlindBoxTask()
			},
		},
		{
			Code:           "cloudphone",
			Name:           "云手机红包",
			Description:    "活动接口当前拒绝授权，默认不再纳入任务批次",
			SortOrder:      160,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"cloud_phone_redpack"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runCloudPhoneTask()
			},
		},
		{
			Code:           "token_pk",
			Name:           "算力大作战",
			Description:    "自动完成算力大作战任务并领取Token奖励",
			SortOrder:      152,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"tokenpk", "suanli"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runTokenPKTask()
			},
		},
		{
			Code:           "make_wish",
			Name:           "许愿赢好礼",
			Description:    "每月许愿并完成任务累积抽奖码（可用CAIYUN_MAKEWISH_PRIZE指定愿望关键字）",
			SortOrder:      154,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"makewish", "wish"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMakeWishTask()
			},
		},
		{
			Code:           "makewish_exchange",
			Name:           "许愿 AI 豆兑换",
			Description:    "可兑换时消耗 300 AI 豆，需手动启用或单独执行",
			SortOrder:      155,
			DefaultEnabled: false,
			RunInBatch:     false,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMakeWishExchangeTask()
			},
		},
		{
			Code:           "fun_ai",
			Name:           "趣玩AI抽奖",
			Description:    "体验AI功能累积抽奖次数并抽奖",
			SortOrder:      156,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"funai", "playai"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runFunAITask()
			},
		},
		{
			Code:           "poster_activity",
			Name:           "校园海报活动",
			Description:    "完成AI相机体验任务并抽取校园海报活动奖励",
			SortOrder:      158,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"poster", "newyear_special"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runPosterTask()
			},
		},
		{
			Code:           "mutual_assist",
			Name:           "多账号活动互助",
			Description:    "使用同一用户下另一活跃账号助力许愿、算力大作战和趣玩AI邀请活动",
			SortOrder:      159,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"account_mutual_assist"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMutualAssistTask()
			},
		},
		{
			Code:           "hidden_rewards",
			Name:           "隐藏活动奖励",
			Description:    "扫描隐藏任务、领取已达标的短信通知/月度福袋/焕新权益和未领奖品",
			SortOrder:      160,
			DefaultEnabled: true,
			RunInBatch:     true,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runHiddenRewardsTask()
			},
		},
		{
			Code:           "after_task",
			Name:           "收尾清理",
			Description:    "清理任务批次中产生的临时文件和分享链接",
			SortOrder:      170,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"aftertask"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runAfterTaskTask()
			},
		},
		{
			Code:           "todaycloud",
			Name:           "今日云朵统计",
			Description:    "统计账号今日已获得的云朵总量",
			SortOrder:      165,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"today_cloud"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runTodayCloudTask()
			},
		},
		{
			Code:           "notice_switch",
			Name:           "通知开关同步",
			Description:    "按账号提供的真实APP通知状态上报，并开启邮箱短信通知开关",
			SortOrder:      88,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"notice", "appnotice"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runNoticeSwitchTask()
			},
		},
		{
			Code:           "fun_ai_mail",
			Name:           "趣玩AI邮箱版",
			Description:    "趣玩AI邮箱版（National_playAI139mail）的任务登记与抽奖",
			SortOrder:      157,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"funai_mail", "playai139mail"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runFunAIMailTask()
			},
		},
		{
			Code:           "prize_center",
			Name:           "领奖专区盘点",
			Description:    "盘点未领取奖品；仅在按账号提供奖品OID与对应验证码时领取一件",
			SortOrder:      161,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"getprize"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runPrizeCenterTask()
			},
		},
		{
			Code:           "upgrade_gift",
			Name:           "焕新权益",
			Description:    "读取焕新权益奖池并激活已获得权益（hidden_rewards 已含激活，默认不重复执行）",
			SortOrder:      162,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"v13gift"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runUpgradeGiftTask()
			},
		},
		{
			Code:           "student_perks",
			Name:           "学生认证福利",
			Description:    "同步并查询学生认证状态；仅在配置短验码时领奖",
			SortOrder:      163,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"studentperks"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runStudentPerksTask()
			},
		},
		{
			Code:           "family_circle",
			Name:           "家庭圈任务",
			Description:    "查询家庭圈备份与任务状态（需先开通家庭圈群组）",
			SortOrder:      164,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"familycircle"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runFamilyCircleTask()
			},
		},
		{
			Code:           "meitu_backup",
			Name:           "美图备份好礼",
			Description:    "美图授权备份领好礼（仅移动号；前置为设备侧授权与备份开关）",
			SortOrder:      166,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"meitu"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMeituBackupTask()
			},
		},
		{
			Code:           "red_invite",
			Name:           "红包邀请",
			Description:    "红包邀请额度查询与邀请码生成/接受（受活动侧风控约束）",
			SortOrder:      167,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"redinvite", "invitingtask"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runRedInviteTask()
			},
		},
		{
			Code:           "unloading_1t",
			Name:           "1T新礼",
			Description:    "1T 新礼活动探测（服务端当前回 503，未部署）",
			SortOrder:      168,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"newgifts1t"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runUnloading1TTask()
			},
		},
		{
			Code:           "rafflecode",
			Name:           "抽奖码",
			Description:    "抽奖码场次与记录查询（需 jwtToken 头，当前无场次）",
			SortOrder:      169,
			DefaultEnabled: false,
			RunInBatch:     false,
			execute: func(r *TaskRunner) *TaskResult {
				return r.runRafflecodeTask()
			},
		},
		{
			Code:           "ai_store",
			Name:           "AI Store 作品保存",
			Description:    "探测 AI Store 授权；仅在配置模块号时保存作品（朋友圈9图需 App 上下文）",
			SortOrder:      171,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"aistore"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runAIStoreTask()
			},
		},
		{
			Code:           "album_backup_report",
			Name:           "相册备份状态上报",
			Description:    "查询相册备份状态；仅在配置真实开关值时上报",
			SortOrder:      172,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"albumbackup"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runAlbumBackupReportTask()
			},
		},
		{
			Code:           "mcloud_day",
			Name:           "会员日",
			Description:    "移动云盘会员日领礼与盲盒抽奖",
			SortOrder:      173,
			DefaultEnabled: false,
			RunInBatch:     false,
			Aliases:        []string{"mcloudday"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runMCloudDayTask()
			},
		},
	}

	catalog := &TaskCatalog{
		definitions: make(map[string]TaskDefinition, len(defs)),
		aliases:     make(map[string]string, len(defs)*2),
	}

	for _, def := range defs {
		code := normalizeTaskCode(def.Code)
		def.Code = code
		catalog.definitions[code] = def
		catalog.aliases[code] = code
		for _, alias := range def.Aliases {
			normalizedAlias := normalizeTaskCode(alias)
			if normalizedAlias == "" {
				continue
			}
			catalog.aliases[normalizedAlias] = code
		}
	}

	return catalog
}

func DefaultTaskConfigs() []models.TaskConfig {
	return defaultTaskCatalog.ConfigModels()
}

func normalizeTaskCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}

func (c *TaskCatalog) Normalize(code string) string {
	if c == nil {
		return normalizeTaskCode(code)
	}

	normalized := normalizeTaskCode(code)
	if normalized == "" {
		return ""
	}
	if canonical, ok := c.aliases[normalized]; ok {
		return canonical
	}
	return normalized
}

func (c *TaskCatalog) Get(code string) (TaskDefinition, bool) {
	if c == nil {
		return TaskDefinition{}, false
	}
	def, ok := c.definitions[c.Normalize(code)]
	return def, ok
}

func (c *TaskCatalog) List() []TaskDefinition {
	if c == nil {
		return nil
	}

	result := make([]TaskDefinition, 0, len(c.definitions))
	for _, def := range c.definitions {
		result = append(result, def)
	}

	sort.Slice(result, func(i, j int) bool {
		if result[i].SortOrder == result[j].SortOrder {
			return result[i].Code < result[j].Code
		}
		return result[i].SortOrder < result[j].SortOrder
	})

	return result
}

func (c *TaskCatalog) DefaultBatchCodes() []string {
	defs := c.List()
	result := make([]string, 0, len(defs))
	for _, def := range defs {
		if def.DefaultEnabled && def.RunInBatch {
			result = append(result, def.Code)
		}
	}
	return result
}

func (c *TaskCatalog) ResolveBatchCodes(configs []*models.TaskConfig) []string {
	if c == nil {
		return nil
	}

	if len(configs) == 0 {
		return c.DefaultBatchCodes()
	}

	sorted := make([]*models.TaskConfig, 0, len(configs))
	for _, cfg := range configs {
		if cfg != nil {
			sorted = append(sorted, cfg)
		}
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].SortOrder == sorted[j].SortOrder {
			return sorted[i].TaskType < sorted[j].TaskType
		}
		return sorted[i].SortOrder < sorted[j].SortOrder
	})

	result := make([]string, 0, len(sorted))
	seen := make(map[string]bool, len(sorted))
	for _, cfg := range sorted {
		if cfg == nil || !cfg.IsEnabled || !cfg.RunInBatch {
			continue
		}
		code := c.Normalize(cfg.TaskType)
		if code == "" || seen[code] {
			continue
		}
		if _, ok := c.definitions[code]; !ok {
			continue
		}
		seen[code] = true
		result = append(result, code)
	}
	return result
}

func (c *TaskCatalog) ConfigModels() []models.TaskConfig {
	defs := c.List()
	configs := make([]models.TaskConfig, 0, len(defs))
	for _, def := range defs {
		configs = append(configs, models.TaskConfig{
			TaskType:    def.Code,
			TaskName:    def.Name,
			Description: def.Description,
			IsEnabled:   def.DefaultEnabled,
			SortOrder:   def.SortOrder,
			RunInBatch:  def.RunInBatch,
		})
	}
	return configs
}

func (c *TaskCatalog) Execute(r *TaskRunner, code string) (*TaskResult, error) {
	def, ok := c.Get(code)
	if !ok {
		return nil, fmt.Errorf("task not found: %s", code)
	}
	if def.execute == nil {
		return nil, fmt.Errorf("task executor not configured: %s", def.Code)
	}

	result := def.execute(r)
	if result == nil {
		return nil, fmt.Errorf("task result is nil: %s", def.Code)
	}
	if result.TaskType == "" {
		result.TaskType = def.Code
	}
	return result, nil
}
