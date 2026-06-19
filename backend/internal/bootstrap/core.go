package bootstrap

import (
	"errors"
	"fmt"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/constants"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/repository"
	"caiyun/internal/services"
	"caiyun/pkg/database"

	"gorm.io/gorm"
)

type Core struct {
	DB         *gorm.DB
	Redis      *cache.RedisCache
	Auth       *auth.Auth
	TaskStore  *cache.RedisStorage
	Repository Repositories
}

// Close 统一释放 Core 持有的外部连接资源。
func (c *Core) Close() error {
	if c == nil {
		return nil
	}
	var closeErr error
	if c.Redis != nil {
		closeErr = errors.Join(closeErr, c.Redis.Close())
	}
	if c.DB != nil {
		sqlDB, err := c.DB.DB()
		if err != nil {
			closeErr = errors.Join(closeErr, err)
		} else {
			closeErr = errors.Join(closeErr, sqlDB.Close())
		}
	}
	return closeErr
}

type Repositories struct {
	User            *repository.UserRepository
	Account         *repository.AccountRepository
	TaskLog         *repository.TaskLogRepository
	CloudStats      *repository.CloudStatsRepository
	TaskConfig      *repository.TaskConfigRepository
	Product         *repository.ProductRepository
	ExchangeAccount *repository.ExchangeAccountRepository
	ExchangeTask    *repository.ExchangeTaskRepository
	ExchangeRecord  *repository.ExchangeRecordRepository
	SystemConfig    *repository.SystemConfigRepository
	AuditLog        *repository.AuditLogRepository
	Announcement    *repository.AnnouncementRepository
	WSMessage       *repository.WSMessageRepository
	Schema          *repository.SchemaRepository
}

func InitCore() (*Core, error) {
	db, err := database.NewMySQL(database.Config{
		Host: GetEnv("DB_HOST", "localhost"),
		Port: GetEnv("DB_PORT", "3306"),
		User: GetEnv("DB_USER", "caiyun_app"),
		// 数据库密码是外部服务凭据，不应套用 JWT/HMAC 的 32 字符密钥长度规则；
		// 是否允许短密码由 MySQL 自身账号策略决定。
		Password:        GetEnv("DB_PASSWORD", ""),
		DBName:          GetEnv("DB_NAME", "caiyun"),
		MaxIdleConns:    GetIntEnv("DB_MAX_IDLE_CONNS", 20),
		MaxOpenConns:    GetIntEnv("DB_MAX_OPEN_CONNS", 100),
		ConnMaxLifetime: GetDurationEnv("DB_CONN_MAX_LIFETIME", time.Hour),
		ConnMaxIdleTime: GetDurationEnv("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	redisCache, err := cache.NewRedisCache(cache.RedisConfig{
		Host: GetEnv("REDIS_HOST", "localhost"),
		Port: GetEnv("REDIS_PORT", "6379"),
		// Redis 本地部署通常不设置密码，REDIS_PASSWORD= 空值是合法配置。
		// 不能用 GetSecretEnv，否则 ALLOW_INSECURE_DEFAULTS=false 时会把空密码当成启动致命错误。
		Password: GetEnv("REDIS_PASSWORD", ""),
		DB:       GetIntEnv("REDIS_DB", 0),
	})
	if err != nil {
		_ = closeGormDB(db)
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}
	if redisCache == nil {
		_ = closeGormDB(db)
		return nil, fmt.Errorf("Redis 连接失败: 缓存实例为空")
	}

	repos := Repositories{
		User:            repository.NewUserRepository(db),
		Account:         repository.NewAccountRepository(db),
		TaskLog:         repository.NewTaskLogRepository(db),
		CloudStats:      repository.NewCloudStatsRepository(db),
		TaskConfig:      repository.NewTaskConfigRepository(db),
		Product:         repository.NewProductRepository(db),
		ExchangeAccount: repository.NewExchangeAccountRepository(db),
		ExchangeTask:    repository.NewExchangeTaskRepository(db),
		ExchangeRecord:  repository.NewExchangeRecordRepository(db),
		SystemConfig:    repository.NewSystemConfigRepository(db),
		AuditLog:        repository.NewAuditLogRepository(db),
		Announcement:    repository.NewAnnouncementRepository(db),
		WSMessage:       repository.NewWSMessageRepository(db),
		Schema:          repository.NewSchemaRepository(db),
	}

	if err := repos.Schema.ValidateCriticalSchema(); err != nil {
		_ = redisCache.Close()
		_ = closeGormDB(db)
		return nil, fmt.Errorf("数据库结构校验失败: %w", err)
	}
	// 注意：曾经在此处自动执行 ALTER TABLE 补齐缺失列，但运行时改 schema 在生产环境
	// 与 DBA 流程冲突且难以审计。现在仅做校验：缺列会直接启动失败并提示执行 migrations。
	// TaskConfig 同步属于业务数据而非 DDL，仍可在启动时进行。
	if err := repos.TaskConfig.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
		_ = redisCache.Close()
		_ = closeGormDB(db)
		return nil, fmt.Errorf("任务配置同步失败: %w", err)
	}

	return &Core{
		DB:         db,
		Redis:      redisCache,
		Auth:       auth.NewAuth(corehttp.NewClient()),
		TaskStore:  cache.NewRedisStorage(redisCache, constants.RedisNamespaceTask),
		Repository: repos,
	}, nil
}

func closeGormDB(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
