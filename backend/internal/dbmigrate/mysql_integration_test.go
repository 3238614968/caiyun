package dbmigrate

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"caiyun/internal/repository"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestRunEmbeddedMySQLIntegrationAppliesAndTracksVersions(t *testing.T) {
	if os.Getenv("CAIYUN_MYSQL_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_MYSQL_INTEGRATION=1 to run MySQL integration tests")
	}

	adminDB := openMySQLIntegrationDB(t, mysqlIntegrationDSN(t, ""))
	testDBName := fmt.Sprintf("caiyun_it_%d", time.Now().UnixNano())
	quotedDBName := "`" + strings.ReplaceAll(testDBName, "`", "") + "`"
	if err := adminDB.Exec("CREATE DATABASE " + quotedDBName + " CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatalf("create test database error: %v", err)
	}
	t.Cleanup(func() {
		_ = adminDB.Exec("DROP DATABASE IF EXISTS " + quotedDBName).Error
		if sqlDB, err := adminDB.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	db := openMySQLIntegrationDB(t, mysqlIntegrationDSN(t, testDBName))
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})

	logger := log.New(io.Discard, "", 0)
	ctx := context.Background()
	if err := RunEmbedded(ctx, db, logger); err != nil {
		t.Fatalf("RunEmbedded first error: %v", err)
	}
	versions := loadMigrationVersions(t, db)
	wantVersions := embeddedMigrationVersions(t)
	if !reflect.DeepEqual(versions, wantVersions) {
		t.Fatalf("schema_migrations versions = %#v, want %#v", versions, wantVersions)
	}

	schemaRepo := repository.NewSchemaRepository(db)
	if err := schemaRepo.ValidateCriticalSchema(); err != nil {
		t.Fatalf("ValidateCriticalSchema error: %v", err)
	}
	if err := schemaRepo.WithContext(context.Background()).ValidateCriticalSchema(); err != nil {
		t.Fatalf("ValidateCriticalSchema with context error: %v", err)
	}

	var descriptions []string
	if err := db.Table("schema_migrations").Order("version ASC").Pluck("description", &descriptions).Error; err != nil {
		t.Fatalf("load descriptions error: %v", err)
	}
	for idx, description := range descriptions {
		if strings.TrimSpace(description) == "" {
			t.Fatalf("description for version %s is empty", versions[idx])
		}
	}

	var firstCount int64
	if err := db.Table("schema_migrations").Count(&firstCount).Error; err != nil {
		t.Fatalf("count schema_migrations after first run error: %v", err)
	}
	if err := RunEmbedded(ctx, db, logger); err != nil {
		t.Fatalf("RunEmbedded second error: %v", err)
	}
	var secondCount int64
	if err := db.Table("schema_migrations").Count(&secondCount).Error; err != nil {
		t.Fatalf("count schema_migrations after second run error: %v", err)
	}
	if firstCount != secondCount {
		t.Fatalf("schema_migrations count changed after rerun: first=%d second=%d", firstCount, secondCount)
	}
	if versionsAfter := loadMigrationVersions(t, db); !reflect.DeepEqual(versionsAfter, wantVersions) {
		t.Fatalf("schema_migrations after rerun = %#v, want %#v", versionsAfter, wantVersions)
	}
}

func mysqlIntegrationDSN(t *testing.T, dbName string) string {
	t.Helper()
	addr := getenvOrDefault("CAIYUN_TEST_MYSQL_ADDR", "127.0.0.1:3306")
	user := getenvOrDefault("CAIYUN_TEST_MYSQL_USER", "root")
	password := os.Getenv("CAIYUN_TEST_MYSQL_PASSWORD")
	dsn := fmt.Sprintf("%s:%s@tcp(%s)/", user, password, addr)
	if dbName != "" {
		dsn += dbName
	}
	return dsn + "?charset=utf8mb4&parseTime=True&loc=UTC&multiStatements=true"
}

func openMySQLIntegrationDB(t *testing.T, dsn string) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{
		Logger:      logger.Default.LogMode(logger.Silent),
		PrepareStmt: true,
	})
	if err != nil {
		t.Fatalf("open mysql integration db error: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB error: %v", err)
	}
	sqlDB.SetConnMaxLifetime(time.Minute)
	sqlDB.SetConnMaxIdleTime(time.Minute)
	sqlDB.SetMaxOpenConns(4)
	sqlDB.SetMaxIdleConns(4)
	if err := sqlDB.Ping(); err != nil {
		t.Fatalf("ping mysql integration db error: %v", err)
	}
	return db
}

func embeddedMigrationVersions(t *testing.T) []string {
	t.Helper()
	entries, err := migrationFS.ReadDir("sql")
	if err != nil {
		t.Fatalf("ReadDir embedded sql error: %v", err)
	}
	versions := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		versions = append(versions, strings.TrimSuffix(entry.Name(), ".sql"))
	}
	sort.Strings(versions)
	return versions
}

func loadMigrationVersions(t *testing.T, db *gorm.DB) []string {
	t.Helper()
	var versions []string
	if err := db.Table("schema_migrations").Order("version ASC").Pluck("version", &versions).Error; err != nil {
		t.Fatalf("load migration versions error: %v", err)
	}
	return versions
}

func getenvOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
