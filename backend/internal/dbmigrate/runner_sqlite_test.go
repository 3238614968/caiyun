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
	var record schemaMigrationRecord
	if err := db.Where("version = ?", "999_unit_success").Take(&record).Error; err != nil {
		t.Fatalf("load migration record error: %v", err)
	}
	if record.Dirty {
		t.Fatal("successful migration must clear dirty state")
	}
	if len(record.Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(record.Checksum))
	}
}

func TestApplyMigrationStatementsLeavesPartialDDLMarkedDirtyOnError(t *testing.T) {
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

	var record schemaMigrationRecord
	if err := db.Where("version = ?", "999_unit_failure").Take(&record).Error; err != nil {
		t.Fatalf("load failed migration record error: %v", err)
	}
	if !record.Dirty {
		t.Fatal("partially applied migration must remain dirty")
	}
	if len(record.Checksum) != 64 {
		t.Fatalf("checksum length = %d, want 64", len(record.Checksum))
	}
}

func TestValidateAppliedMigrationRejectsChecksumMismatchAndBackfillsLegacyRecord(t *testing.T) {
	db := newMigrationUnitDB(t)
	ctx := context.Background()
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		t.Fatalf("ensureSchemaMigrations error: %v", err)
	}

	checksum := migrationChecksum([]byte("migration content"))
	record := schemaMigrationRecord{Version: "999_checksum", Description: "legacy"}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create legacy migration record: %v", err)
	}
	if err := validateAppliedMigration(ctx, db, &record, record.Version, "checksum", checksum); err != nil {
		t.Fatalf("backfill legacy checksum: %v", err)
	}
	var backfilled schemaMigrationRecord
	if err := db.Where("version = ?", record.Version).Take(&backfilled).Error; err != nil {
		t.Fatalf("load backfilled migration record: %v", err)
	}
	if backfilled.Checksum != checksum || backfilled.Dirty {
		t.Fatalf("backfilled record = %#v, want checksum=%s and clean", backfilled, checksum)
	}

	if err := validateAppliedMigration(ctx, db, &backfilled, backfilled.Version, "checksum", migrationChecksum([]byte("changed content"))); err == nil || !strings.Contains(err.Error(), "checksum mismatch") {
		t.Fatalf("checksum mismatch error = %v, want mismatch", err)
	}
}

func TestValidateAppliedMigrationRejectsDirtyRecord(t *testing.T) {
	db := newMigrationUnitDB(t)
	ctx := context.Background()
	if err := ensureSchemaMigrations(ctx, db); err != nil {
		t.Fatalf("ensureSchemaMigrations error: %v", err)
	}
	record := schemaMigrationRecord{Version: "999_dirty", Description: "dirty", Checksum: migrationChecksum([]byte("dirty")), Dirty: true}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create dirty migration record: %v", err)
	}
	if err := validateAppliedMigration(ctx, db, &record, record.Version, record.Description, record.Checksum); err == nil || !strings.Contains(err.Error(), "marked dirty") {
		t.Fatalf("dirty migration error = %v, want marked dirty", err)
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
