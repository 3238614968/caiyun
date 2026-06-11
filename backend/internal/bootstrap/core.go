package bootstrap

import (
	"fmt"

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
		Host:     GetEnv("DB_HOST", "localhost"),
		Port:     GetEnv("DB_PORT", "3306"),
		User:     GetEnv("DB_USER", "caiyun_app"),
		Password: GetSecretEnv("DB_PASSWORD", "local-development-password"),
		DBName:   GetEnv("DB_NAME", "caiyun"),
	})
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}

	redisCache, err := cache.NewRedisCache(cache.RedisConfig{
		Host:     GetEnv("REDIS_HOST", "localhost"),
		Port:     GetEnv("REDIS_PORT", "6379"),
		Password: GetEnv("REDIS_PASSWORD", ""),
		DB:       0,
	})
	if err != nil {
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
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
		return nil, fmt.Errorf("数据库结构校验失败: %w", err)
	}
	if err := repos.TaskConfig.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
		_ = redisCache.Close()
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
