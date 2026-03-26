package constants

// 抢兑系统常量定义

// 默认配置值
const (
	// DefaultMonthlyCardPrizeID 默认月卡商品ID
	DefaultMonthlyCardPrizeID = "1001"

	// DefaultConcurrency 默认并发数
	DefaultConcurrency = 10

	// DefaultMaxRetries 默认最大重试次数
	DefaultMaxRetries = 3

	// DefaultExchangeTime 默认兑换时间
	DefaultExchangeTime = "09:00"
)

// 时间相关常量
const (
	// TokenRefreshInterval Token刷新间隔（小时）
	TokenRefreshInterval = 1

	// TokenExpireBuffer Token过期缓冲时间（小时）
	TokenExpireBuffer = 1

	// RetryBackoffBase 重试退避基础时间（秒）
	RetryBackoffBase = 2

	// MaxBatchExecuteTasks 最大批量执行任务数
	MaxBatchExecuteTasks = 50

	// MaxExportRecords 最大导出记录数
	MaxExportRecords = 10000
)

// 兑换时间窗口
const (
	// MorningExchangeHour 上午兑换小时（10点）
	MorningExchangeHour = 10

	// MorningExchangeMinute 上午兑换分钟
	MorningExchangeMinute = 0

	// EveningExchangeHour 晚上兑换小时（16点）
	EveningExchangeHour = 16

	// EveningExchangeMinute 晚上兑换分钟
	EveningExchangeMinute = 0

	// ExchangeTimeWindowMinutes 兑换时间窗口（分钟）
	ExchangeTimeWindowMinutes = 5

	// ExchangePreInitSeconds 提前初始化秒数（提前30秒准备 JWT 与队列）
	ExchangePreInitSeconds = 30
)

// 定时任务配置
const (
	// AutoUpdateProductsCron 自动更新商品定时任务表达式（每天凌晨3点）
	AutoUpdateProductsCron = "0 3 * * *"

	// AccountHealthCheckCron 账号健康检查定时任务表达式（每6小时）
	AccountHealthCheckCron = "0 */6 * * *"

	// StatsRetentionDays 统计数据保留天数
	StatsRetentionDays = 30

	// AuditLogRetentionDays 审计日志保留天数
	AuditLogRetentionDays = 90

	// WSMessageRetentionDays WebSocket消息保留天数
	WSMessageRetentionDays = 7
)

// HTTP客户端配置
const (
	// HTTPTimeout HTTP请求超时时间（秒）
	HTTPTimeout = 30

	// HTTPMaxRetries HTTP请求最大重试次数
	HTTPMaxRetries = 3

	// HTTPRetryDelay HTTP请求重试延迟（秒）
	HTTPRetryDelay = 1
)

// WebSocket配置
const (
	// WSReadBufferSize WebSocket读取缓冲区大小
	WSReadBufferSize = 1024

	// WSWriteBufferSize WebSocket写入缓冲区大小
	WSWriteBufferSize = 1024

	// WSPingPeriod WebSocket ping周期（秒）
	WSPingPeriod = 30

	// WSPongWait WebSocket pong等待时间（秒）
	WSPongWait = 60

	// WSWriteTimeout WebSocket写入超时时间（秒）
	WSWriteTimeout = 10
)

// 限流配置
const (
	// DefaultGlobalRate 默认全局限流速率（每秒请求数）
	DefaultGlobalRate = 100

	// DefaultGlobalBurst 默认全局限流突发容量
	DefaultGlobalBurst = 150

	// DefaultUserRate 默认用户限流速率（每秒请求数）
	DefaultUserRate = 30

	// DefaultUserBurst 默认用户限流突发容量
	DefaultUserBurst = 50

	// ExchangeTaskRate 兑换任务限流速率（每秒请求数）
	ExchangeTaskRate = 5

	// ExchangeTaskBurst 兑换任务限流突发容量
	ExchangeTaskBurst = 10

	// BatchExecuteRate 批量执行限流速率（每秒请求数）
	BatchExecuteRate = 2

	// BatchExecuteBurst 批量执行限流突发容量
	BatchExecuteBurst = 5

	// ExportRate 导出限流速率（每秒请求数）
	ExportRate = 1

	// ExportBurst 导出限流突发容量
	ExportBurst = 3
)

// 分页配置
const (
	// DefaultPageSize 默认每页大小
	DefaultPageSize = 20

	// MaxPageSize 最大每页大小
	MaxPageSize = 100

	// DefaultPage 默认页码
	DefaultPage = 1
)

// 系统配置键名
const (
	// ConfigKeyExchangeEnabled 抢兑功能启用配置键
	ConfigKeyExchangeEnabled = "exchange_enabled"

	// ConfigKeyExchangeConcurrency 抢兑并发数配置键
	ConfigKeyExchangeConcurrency = "exchange_concurrency"

	// ConfigKeyExchangeTime 抢兑时间配置键
	ConfigKeyExchangeTime = "exchange_time"

	// ConfigKeyExchangeMonthlyEnabled 月卡兑换启用配置键
	ConfigKeyExchangeMonthlyEnabled = "exchange_monthly_enabled"

	// ConfigKeyExchangeMonthlyPrizeID 月卡商品ID配置键
	ConfigKeyExchangeMonthlyPrizeID = "exchange_monthly_prize_id"

	// ConfigKeyAutoUpdateProducts 自动更新商品配置键
	ConfigKeyAutoUpdateProducts = "exchange_auto_update_products"
)
