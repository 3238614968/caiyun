//go:build cgo
// +build cgo

package dbmigrate

import (
	"context"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestApplyMigrationStatementsRecordsVersionOnSuccess(t *testing.T) {
	db := newMigrationUnitDB(t)
	ctx := context.Background()
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		t.Fatalf("ensureSchemaMigrations error: %v", err)
	}

	statements := []string{
		`CREATE TABLE tx_success (id INTEGER PRIMARY KEY, value TEXT)`,
		`INSERT INTO tx_success (id, value) VALUES (1, 'ok')`,
	}
	if err := applyMigrationStatements(ctx, db, "999_unit_success", "unit success", statements); err != nil {
		t.Fatalf("applyMigrationStatements success error: %v", err)
	}

	if !db.Migrator().HasTable("tx_success") {
		t.Fatalf("tx_success table was not created")
	}
	var value string
	if err := db.Raw(`SELECT value FROM tx_success WHERE id = 1`).Scan(&value).Error; err != nil {
		t.Fatalf("query tx_success error: %v", err)
	}
	if value != "ok" {
		t.Fatalf("tx_success value = %q, want ok", value)
	}
	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", "999_unit_success").Count(&count).Error; err != nil {
		t.Fatalf("count migration record error: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration record count = %d, want 1", count)
	}
}

func TestApplyMigrationStatementsLeavesPartialDDLUnrecordedOnError(t *testing.T) {
	db := newMigrationUnitDB(t)
	ctx := context.Background()
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		t.Fatalf("ensureSchemaMigrations error: %v", err)
	}

	statements := []string{
		`CREATE TABLE partial_failure (id INTEGER PRIMARY KEY, value TEXT)`,
		`INSERT INTO partial_failure (id, value) VALUES (1, 'persisted')`,
		`INSERT INTO missing_table (id) VALUES (1)`,
	}
	err := applyMigrationStatements(ctx, db, "999_unit_failure", "unit failure", statements)
	if err == nil {
		t.Fatalf("applyMigrationStatements expected error, got nil")
	}
	if !strings.Contains(err.Error(), "999_unit_failure") {
		t.Fatalf("error = %v, want contains version", err)
	}

	// This intentionally models MySQL DDL auto-commit semantics: successful
	// statements before the failure remain applied and must be idempotent.
	if !db.Migrator().HasTable("partial_failure") {
		t.Fatalf("partial_failure table should remain after a later statement fails")
	}
	var value string
	if err := db.Raw(`SELECT value FROM partial_failure WHERE id = 1`).Scan(&value).Error; err != nil {
		t.Fatalf("query partial_failure error: %v", err)
	}
	if value != "persisted" {
		t.Fatalf("partial_failure value = %q, want persisted", value)
	}

	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", "999_unit_failure").Count(&count).Error; err != nil {
		t.Fatalf("count migration record error: %v", err)
	}
	if count != 0 {
		t.Fatalf("failed migration record count = %d, want 0", count)
	}
}

func newMigrationUnitDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite migration unit db: %v", err)
	}
	return db
}
