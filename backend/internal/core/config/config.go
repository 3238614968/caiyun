package config

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Config 配置结构
type Config struct {
	// 账号配置（直接位于根级）
	Auth            string         `yaml:"auth"`
	Phone           string         `yaml:"phone"`
	Token           string         `yaml:"token"`
	Platform        string         `yaml:"platform"`
	Expire          int64          `yaml:"expire"`
	UserID          string         `yaml:"user_id"`
	EnableAICloud   bool           `yaml:"enable_ai_cloud"`
	EnableRedPacket bool           `yaml:"enable_red_packet"`
	RefreshDays     int            `yaml:"refresh_days_before_expire"`
	Features        FeaturesConfig `yaml:"features,omitempty"`
}

// FeaturesConfig 功能开关配置
type FeaturesConfig struct {
	GardenCheckin       bool `yaml:"garden_checkin,omitempty"`
	BackupGift          bool `yaml:"backup_gift,omitempty"`
	Blindbox            bool `yaml:"blindbox,omitempty"`
	CloudPhoneParty     bool `yaml:"cloud_phone_party,omitempty"`
	MessagePushReward   bool `yaml:"message_push_reward,omitempty"`
	PrintTodayCloud     bool `yaml:"print_today_cloud,omitempty"`
	InviteFriends       bool `yaml:"invite_friends,omitempty"`
	ExchangeMonthlyCard bool `yaml:"exchange_monthly_card,omitempty"`
	Shake               bool `yaml:"shake,omitempty"`
	CloudBattle         bool `yaml:"cloud_battle,omitempty"`
}

// Load 加载配置文件
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("解析配置文件失败: %w", err)
	}

	// 当配置文件未提供 auth 时，从环境变量回退读取
	if cfg.Auth == "" {
		cfg.Auth = os.Getenv("CAIYUN_AUTH")
	}

	// 设置默认值
	if cfg.RefreshDays == 0 {
		cfg.RefreshDays = 30
	}

	return &cfg, nil
}

// Save 保存配置文件
func (c *Config) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("序列化配置失败: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("写入配置文件失败: %w", err)
	}

	return nil
}

// NeedsRefresh 检查 Token 是否需要刷新
func (c *Config) NeedsRefresh() bool {
	if c.Expire == 0 {
		return false
	}

	currentTime := time.Now().UnixMilli()
	daysRemaining := (c.Expire - currentTime) / (24 * 60 * 60 * 1000)

	return daysRemaining <= int64(c.RefreshDays)
}
