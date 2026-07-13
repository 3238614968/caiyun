package migrator

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/dbmigrate"
	"caiyun/internal/repository"
	"caiyun/internal/security"
	"caiyun/internal/services"
	"caiyun/internal/version"
	"caiyun/pkg/database"
)

// Run executes the dedicated database migration command. API and worker
// processes never call this path implicitly in production.
func Run(ctx context.Context, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	bootstrap.LoadEnvFile()
	if err := security.ValidateFieldCryptoConfig(); err != nil {
		return fmt.Errorf("数据加密配置校验失败: %w", err)
	}
	closeLogger := bootstrap.ConfigureStandardLogger("migrator")
	defer closeLogger()

	flags := flag.NewFlagSet("caiyun migrate", flag.ContinueOnError)
	validateOnly := flags.Bool("validate-only", false, "仅校验数据库结构，不执行迁移")
	skipTaskConfigSync := flags.Bool("skip-task-config-sync", bootstrap.GetBoolEnv("MIGRATOR_SKIP_TASK_CONFIG_SYNC", false), "跳过任务配置定义同步")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("migrate 不支持位置参数: %s", strings.Join(flags.Args(), " "))
	}

	log.Printf("启动 caiyun migrate: %+v", version.Get())
	db, err := database.NewMySQL(database.Config{
		Host:            bootstrap.GetEnv("DB_HOST", "localhost"),
		Port:            bootstrap.GetEnv("DB_PORT", "3306"),
		User:            bootstrap.GetEnv("DB_USER", "caiyun_app"),
		Password:        bootstrap.GetEnv("DB_PASSWORD", ""),
		DBName:          bootstrap.GetEnv("DB_NAME", "caiyun"),
		MaxIdleConns:    bootstrap.GetIntEnv("DB_MAX_IDLE_CONNS", 5),
		MaxOpenConns:    bootstrap.GetIntEnv("DB_MAX_OPEN_CONNS", 10),
		ConnMaxLifetime: bootstrap.GetDurationEnv("DB_CONN_MAX_LIFETIME", time.Hour),
		ConnMaxIdleTime: bootstrap.GetDurationEnv("DB_CONN_MAX_IDLE_TIME", 10*time.Minute),
	})
	if err != nil {
		return fmt.Errorf("数据库连接失败: %w", err)
	}
	defer func() {
		sqlDB, sqlErr := db.DB()
		if sqlErr != nil {
			log.Printf("获取底层数据库句柄失败: %v", sqlErr)
			return
		}
		if closeErr := sqlDB.Close(); closeErr != nil {
			log.Printf("关闭数据库连接失败: %v", closeErr)
		}
	}()

	if *validateOnly {
		log.Println("validate-only 模式：跳过迁移执行，仅校验关键数据库结构")
	} else if err := dbmigrate.RunEmbedded(ctx, db, log.Default()); err != nil {
		return fmt.Errorf("数据库迁移失败: %w", err)
	}

	schemaRepo := repository.NewSchemaRepository(db)
	if err := schemaRepo.ValidateCriticalSchema(); err != nil {
		return fmt.Errorf("数据库结构校验失败: %w", err)
	}
	log.Println("数据库关键结构校验通过")

	if *skipTaskConfigSync {
		log.Println("已跳过任务配置定义同步")
		return nil
	}

	taskConfigRepo := repository.NewTaskConfigRepository(db)
	if err := taskConfigRepo.SyncDefinitions(services.DefaultTaskConfigs()); err != nil {
		return fmt.Errorf("任务配置定义同步失败: %w", err)
	}
	log.Println("数据库迁移与任务配置同步完成")
	return nil
}
