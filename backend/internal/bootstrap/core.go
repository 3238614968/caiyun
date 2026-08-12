package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/constants"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/dbmigrate"
	"caiyun/internal/observability"
	"caiyun/internal/repository"
	"caiyun/internal/services"
	"caiyun/pkg/database"

	"gorm.io/gorm"
)

// CoreConfig contains every infrastructure setting consumed while constructing
// Core. It is the single structured configuration boundary for DB, Redis and
// dependency clock validation.
type CoreConfig struct {
	Database            database.Config
	Redis               cache.RedisConfig
	ClockSkewMax        time.Duration
	SyncTaskDefinitions bool
}

// LoadCoreConfig resolves the Core configuration once from the bootstrap
// environment. Application code receives Core/its dependencies rather than
// reading these keys again.
func LoadCoreConfig() (CoreConfig, error) {
	config := CoreConfig{
		Database: database.Config{
			Host:            GetEnv("DB_HOST", "localhost"),
			Port:            GetEnv("DB_PORT", "3306"),
			User:            GetEnv("DB_USER", "caiyun_app"),
			Password:        GetEnv("DB_PASSWORD", ""),
			DBName:          GetEnv("DB_NAME", "caiyun"),
			MaxIdleConns:    GetIntEnv("DB_MAX_IDLE_CONNS", 20),
			MaxOpenConns:    GetIntEnv("DB_MAX_OPEN_CONNS", 100),
			ConnMaxLifetime: GetDurationEnv("DB_CONN_MAX_LIFETIME", time.Hour),
			ConnMaxIdleTime: GetDurationEnv("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
		},
		Redis: cache.RedisConfig{
			Host:     GetEnv("REDIS_HOST", "localhost"),
			Port:     GetEnv("REDIS_PORT", "6379"),
			Password: GetEnv("REDIS_PASSWORD", ""),
			DB:       GetIntEnv("REDIS_DB", 0),
		},
		ClockSkewMax:        GetDurationEnv("CLOCK_SKEW_MAX", 30*time.Second),
		SyncTaskDefinitions: GetBoolEnv("TASK_CONFIG_SYNC_ON_STARTUP", false),
	}
	if config.Database.MaxIdleConns < 0 || config.Database.MaxOpenConns < 1 {
		return CoreConfig{}, fmt.Errorf("数据库连接池配置无效")
	}
	if config.ClockSkewMax <= 0 {
		config.ClockSkewMax = 30 * time.Second
	}
	return config, nil
}

type Core struct {
	Config     CoreConfig
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
	RefreshSession  *repository.RefreshSessionRepository
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
	Operation       *repository.OperationRepository
	Schema          *repository.SchemaRepository
}

// InitCore resolves configuration at the process boundary. New callers that
// already validated configuration should use InitCoreWithConfig.
func InitCore() (*Core, error) {
	config, err := LoadCoreConfig()
	if err != nil {
		return nil, fmt.Errorf("基础设施配置无效: %w", err)
	}
	return InitCoreWithConfig(config)
}

// InitCoreWithConfig constructs dependencies from an explicit configuration.
// It makes startup tests and alternate process assemblers independent from
// mutable environment state after the configuration has been loaded.
func InitCoreWithConfig(config CoreConfig) (*Core, error) {
	autoMigrate, err := resolveEmbeddedMigrationPolicy()
	if err != nil {
		return nil, fmt.Errorf("数据库迁移配置无效: %w", err)
	}

	db, err := database.NewMySQL(config.Database)
	if err != nil {
		return nil, fmt.Errorf("数据库连接失败: %w", err)
	}
	if err := observability.InstallGORMTracing(db); err != nil {
		_ = closeGormDB(db)
		return nil, fmt.Errorf("数据库追踪初始化失败: %w", err)
	}

	if autoMigrate {
		if err := dbmigrate.RunEmbedded(context.Background(), db, log.Default()); err != nil {
			_ = closeGormDB(db)
			return nil, fmt.Errorf("数据库自动迁移失败: %w", err)
		}
	} else {
		log.Println("API/Worker 已跳过嵌入式数据库迁移，仅进行结构校验；生产迁移必须使用专用 migrator")
	}

	redisCache, err := cache.NewRedisCache(config.Redis)
	if err != nil {
		_ = closeGormDB(db)
		return nil, fmt.Errorf("Redis 连接失败: %w", err)
	}
	if redisCache == nil {
		_ = closeGormDB(db)
		return nil, fmt.Errorf("Redis 连接失败: 缓存实例为空")
	}
	if err := validateDependencyClocks(db, redisCache, config.ClockSkewMax); err != nil {
		_ = redisCache.Close()
		_ = closeGormDB(db)
		return nil, err
	}

	repos := Repositories{
		User:            repository.NewUserRepository(db),
		RefreshSession:  repository.NewRefreshSessionRepository(db),
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
		Operation:       repository.NewOperationRepository(db),
		Schema:          repository.NewSchemaRepository(db),
	}

	if err := repos.Schema.ValidateCriticalSchema(); err != nil {
		_ = redisCache.Close()
		_ = closeGormDB(db)
		return nil, fmt.Errorf("数据库结构校验失败: %w", err)
	}
	if config.SyncTaskDefinitions {
		// This is an explicit development/bootstrap escape hatch.  Production
		// API and Worker processes remain read-only with respect to schema and
		// task definitions; the dedicated migrator owns those release writes.
		if err := repos.TaskConfig.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
			_ = redisCache.Close()
			_ = closeGormDB(db)
			return nil, fmt.Errorf("任务配置同步失败: %w", err)
		}
	} else {
		log.Println("API/Worker 已跳过任务配置同步；发布写入由专用 migrator 负责")
	}

	return &Core{
		Config:     config,
		DB:         db,
		Redis:      redisCache,
		Auth:       auth.NewAuth(corehttp.NewClient()),
		TaskStore:  cache.NewRedisStorage(redisCache, constants.RedisNamespaceTask),
		Repository: repos,
	}, nil
}

func validateDependencyClocks(db *gorm.DB, redisCache *cache.RedisCache, maxSkew time.Duration) error {
	if maxSkew <= 0 {
		maxSkew = 30 * time.Second
	}
	now := time.Now().UTC()

	var dbNow time.Time
	if err := db.Raw("SELECT UTC_TIMESTAMP(6)").Scan(&dbNow).Error; err != nil {
		return fmt.Errorf("数据库时钟校验失败: %w", err)
	}
	dbNow = dbNow.UTC()
	if skew := absoluteDuration(now.Sub(dbNow)); skew > maxSkew {
		return fmt.Errorf("数据库时钟偏差过大: app=%s db=%s skew=%s max=%s，请先同步主机/MySQL时钟",
			now.Format(time.RFC3339Nano), dbNow.Format(time.RFC3339Nano), skew, maxSkew)
	}

	redisNow, err := redisCache.ServerTime()
	if err != nil {
		return fmt.Errorf("Redis 时钟校验失败: %w", err)
	}
	redisNow = redisNow.UTC()
	if skew := absoluteDuration(now.Sub(redisNow)); skew > maxSkew {
		return fmt.Errorf("Redis 时钟偏差过大: app=%s redis=%s skew=%s max=%s，请先同步主机/Redis时钟",
			now.Format(time.RFC3339Nano), redisNow.Format(time.RFC3339Nano), skew, maxSkew)
	}
	return nil
}

func absoluteDuration(value time.Duration) time.Duration {
	if value < 0 {
		return -value
	}
	return value
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
