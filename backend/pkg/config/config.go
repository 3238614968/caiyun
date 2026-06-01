package config

import (
	"fmt"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// Config 全局配置
type Config struct {
	mu sync.RWMutex

	// 服务器配置
	Server ServerConfig

	// 数据库配置
	Database DatabaseConfig

	// Redis 配置
	Redis RedisConfig

	// JWT 配置
	JWT JWTConfig

	// 任务配置
	Task TaskConfig

	// 兑换中心配置
	Exchange ExchangeConfig

	// 日志配置
	Log LogConfig
}

// ServerConfig 服务器配置
type ServerConfig struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
}

// DatabaseConfig 数据库配置
type DatabaseConfig struct {
	Host     string
	Port     string
	User     string
	Password string
	DBName   string
	MaxIdle  int
	MaxOpen  int
	Lifetime time.Duration
	IdleTime time.Duration
}

// RedisConfig Redis 配置
type RedisConfig struct {
	Host     string
	Port     string
	Password string
	DB       int
	PoolSize int
}

// JWTConfig JWT 配置
type JWTConfig struct {
	Secret  string
	Expiry  time.Duration
	Refresh time.Duration
}

// TaskConfig 任务配置
type TaskConfig struct {
	Concurrency int
	Schedule    string
}

// ExchangeConfig 兑换中心配置
type ExchangeConfig struct {
	Concurrency          int
	ScheduleTime1        string
	ScheduleTime2        string
	AutoUpdateProducts   bool
	UpdateTime           string
	EnablePriority       bool
	DefaultTimeout       int
	MaxGlobalConcurrency int
	AutoRetryFailed      bool
	LogRetentionDays     int
}

// LogConfig 日志配置
type LogConfig struct {
	Level      string
	OutputPath string
	MaxSize    int64
	MaxBackups int
	MaxAge     int
	Compress   bool
	JSONFormat bool
}

// defaultConfig 返回默认配置
func defaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port:         "8080",
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Database: DatabaseConfig{
			Host:     "localhost",
			Port:     "3306",
			User:     "root",
			Password: "",
			DBName:   "caiyun",
			MaxIdle:  20,
			MaxOpen:  100,
			Lifetime: time.Hour,
			IdleTime: 10 * time.Minute,
		},
		Redis: RedisConfig{
			Host:     "localhost",
			Port:     "6379",
			Password: "",
			DB:       0,
			PoolSize: 50,
		},
		JWT: JWTConfig{
			Secret:  "",
			Expiry:  7 * 24 * time.Hour,
			Refresh: 24 * time.Hour,
		},
		Task: TaskConfig{
			Concurrency: 20,
			Schedule:    "0 8 * * *",
		},
		Exchange: ExchangeConfig{
			Concurrency:          10,
			ScheduleTime1:        "10:00",
			ScheduleTime2:        "16:00",
			AutoUpdateProducts:   true,
			UpdateTime:           "08:00",
			EnablePriority:       true,
			DefaultTimeout:       30,
			MaxGlobalConcurrency: 50,
			AutoRetryFailed:      true,
			LogRetentionDays:     30,
		},
		Log: LogConfig{
			Level:      "info",
			OutputPath: "",
			MaxSize:    100,
			MaxBackups: 5,
			MaxAge:     30,
			Compress:   false,
			JSONFormat: false,
		},
	}
}

// Load 加载配置
func Load(envFile string) (*Config, error) {
	// 尝试加载.env 文件
	if envFile != "" {
		if err := godotenv.Load(envFile); err != nil {
			// .env 文件不存在是允许的
			fmt.Printf("提示：未找到.env 文件：%v，使用默认配置和环境变量\n", err)
		}
	}

	cfg := defaultConfig()

	// 从环境变量加载配置
	cfg.loadServerConfig()
	cfg.loadDatabaseConfig()
	cfg.loadRedisConfig()
	cfg.loadJWTConfig()
	cfg.loadTaskConfig()
	cfg.loadExchangeConfig()
	cfg.loadLogConfig()

	// 验证配置
	if err := cfg.validate(); err != nil {
		return nil, fmt.Errorf("配置验证失败：%w", err)
	}

	return cfg, nil
}

func (c *Config) loadServerConfig() {
	if port := os.Getenv("PORT"); port != "" {
		c.Server.Port = port
	}
}

func (c *Config) loadDatabaseConfig() {
	if host := os.Getenv("DB_HOST"); host != "" {
		c.Database.Host = host
	}
	if port := os.Getenv("DB_PORT"); port != "" {
		c.Database.Port = port
	}
	if user := os.Getenv("DB_USER"); user != "" {
		c.Database.User = user
	}
	if password := os.Getenv("DB_PASSWORD"); password != "" {
		c.Database.Password = password
	}
	if dbname := os.Getenv("DB_NAME"); dbname != "" {
		c.Database.DBName = dbname
	}
}

func (c *Config) loadRedisConfig() {
	if host := os.Getenv("REDIS_HOST"); host != "" {
		c.Redis.Host = host
	}
	if port := os.Getenv("REDIS_PORT"); port != "" {
		c.Redis.Port = port
	}
	if password := os.Getenv("REDIS_PASSWORD"); password != "" {
		c.Redis.Password = password
	}
	if db := os.Getenv("REDIS_DB"); db != "" {
		if dbInt, err := strconv.Atoi(db); err == nil {
			c.Redis.DB = dbInt
		}
	}
}

func (c *Config) loadJWTConfig() {
	if secret := os.Getenv("JWT_SECRET"); secret != "" {
		c.JWT.Secret = secret
	}
}

func (c *Config) loadTaskConfig() {
	if concurrency := os.Getenv("TASK_CONCURRENCY"); concurrency != "" {
		if val, err := strconv.Atoi(concurrency); err == nil && val > 0 {
			c.Task.Concurrency = val
		}
	}
	if schedule := os.Getenv("TASK_SCHEDULE"); schedule != "" {
		c.Task.Schedule = schedule
	}
}

func (c *Config) loadExchangeConfig() {
	if concurrency := os.Getenv("EXCHANGE_CONCURRENCY"); concurrency != "" {
		if val, err := strconv.Atoi(concurrency); err == nil && val > 0 {
			c.Exchange.Concurrency = val
		}
	}
	if time1 := os.Getenv("EXCHANGE_SCHEDULE_TIME_1"); time1 != "" {
		c.Exchange.ScheduleTime1 = time1
	}
	if time2 := os.Getenv("EXCHANGE_SCHEDULE_TIME_2"); time2 != "" {
		c.Exchange.ScheduleTime2 = time2
	}
	if auto := os.Getenv("EXCHANGE_AUTO_UPDATE_PRODUCTS"); auto != "" {
		c.Exchange.AutoUpdateProducts = auto == "true"
	}
	if update := os.Getenv("EXCHANGE_UPDATE_TIME"); update != "" {
		c.Exchange.UpdateTime = update
	}
	if priority := os.Getenv("EXCHANGE_ENABLE_PRIORITY"); priority != "" {
		c.Exchange.EnablePriority = priority == "true"
	}
	if timeout := os.Getenv("EXCHANGE_DEFAULT_TIMEOUT"); timeout != "" {
		if t, err := strconv.Atoi(timeout); err == nil && t > 0 {
			c.Exchange.DefaultTimeout = t
		}
	}
	if max := os.Getenv("EXCHANGE_MAX_GLOBAL_CONCURRENCY"); max != "" {
		if m, err := strconv.Atoi(max); err == nil && m > 0 {
			c.Exchange.MaxGlobalConcurrency = m
		}
	}
	if retry := os.Getenv("EXCHANGE_AUTO_RETRY_FAILED"); retry != "" {
		c.Exchange.AutoRetryFailed = retry == "true"
	}
	if days := os.Getenv("EXCHANGE_LOG_RETENTION_DAYS"); days != "" {
		if d, err := strconv.Atoi(days); err == nil && d > 0 {
			c.Exchange.LogRetentionDays = d
		}
	}
}

func (c *Config) loadLogConfig() {
	if level := os.Getenv("LOG_LEVEL"); level != "" {
		c.Log.Level = level
	}
	if path := os.Getenv("LOG_PATH"); path != "" {
		c.Log.OutputPath = path
	}
}

// validate 验证配置
func (c *Config) validate() error {
	// 验证必要的环境变量
	if c.JWT.Secret == "" {
		return fmt.Errorf("JWT_SECRET 不能为空")
	}
	if c.JWT.Secret == "your-secret-key-change-in-production" || len(c.JWT.Secret) < 16 {
		return fmt.Errorf("JWT_SECRET 使用弱值或默认值")
	}
	if c.Database.Password == "" {
		return fmt.Errorf("DB_PASSWORD 不能为空")
	}
	if len(c.Database.Password) < 16 {
		return fmt.Errorf("DB_PASSWORD 使用弱值")
	}

	// 验证端口
	if port := c.Server.Port; port == "" {
		return fmt.Errorf("服务器端口不能为空")
	}

	// 验证数据库配置
	if c.Database.Host == "" || c.Database.User == "" || c.Database.DBName == "" {
		return fmt.Errorf("数据库配置不完整")
	}

	// 验证 Redis 配置
	if c.Redis.Host == "" || c.Redis.Port == "" {
		return fmt.Errorf("Redis 配置不完整")
	}

	// 验证并发数
	if c.Task.Concurrency <= 0 {
		return fmt.Errorf("任务并发数必须大于 0")
	}
	if c.Exchange.Concurrency <= 0 {
		return fmt.Errorf("兑换并发数必须大于 0")
	}

	return nil
}

// Get 获取配置值（通用方法）
func (c *Config) Get(key string, defaultValue string) string {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// Reload 重新加载配置
func (c *Config) Reload(envFile string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	newCfg, err := Load(envFile)
	if err != nil {
		return err
	}

	c.Server = newCfg.Server
	c.Database = newCfg.Database
	c.Redis = newCfg.Redis
	c.JWT = newCfg.JWT
	c.Task = newCfg.Task
	c.Exchange = newCfg.Exchange
	c.Log = newCfg.Log
	return nil
}

// globalConfig 全局配置实例
var globalConfig *Config
var once sync.Once

// Global 返回全局配置实例
func Global() *Config {
	once.Do(func() {
		var err error
		globalConfig, err = Load("")
		if err != nil {
			panic(fmt.Sprintf("加载配置失败：%v", err))
		}
	})
	return globalConfig
}

// SetGlobal 设置全局配置（用于测试）
func SetGlobal(cfg *Config) {
	globalConfig = cfg
}
