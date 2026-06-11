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
			Description:    "执行摇一摇抽奖任务",
			SortOrder:      70,
			DefaultEnabled: true,
			RunInBatch:     true,
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
			Description:    "执行合成 1T 云朵游戏任务",
			SortOrder:      140,
			DefaultEnabled: true,
			RunInBatch:     true,
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
			Description:    "执行云手机红包派对签到流程",
			SortOrder:      160,
			DefaultEnabled: true,
			RunInBatch:     true,
			Aliases:        []string{"cloud_phone_redpack"},
			execute: func(r *TaskRunner) *TaskResult {
				return r.runCloudPhoneTask()
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
		if def.RunInBatch {
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
	sorted = append(sorted, configs...)
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
