package dbmigrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"fmt"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

//go:embed sql/*.sql
var migrationFS embed.FS

type schemaMigrationRecord struct {
	Version     string `gorm:"primaryKey;size:64"`
	Description string `gorm:"size:255;not null;default:''"`
	// Checksum is the SHA-256 of the embedded migration file.  It prevents an
	// already deployed migration from being silently changed in a later build.
	// Empty values are only expected on installations created before checksum
	// tracking was introduced and are backfilled once on the next successful
	// migration run.
	Checksum string `gorm:"size:64;not null;default:''"`
	// Dirty is set before executing a migration and cleared only after every
	// statement succeeds.  This is required because MySQL DDL implicitly
	// commits, so a failed migration can leave a partially changed schema.
	Dirty     bool      `gorm:"not null;default:false"`
	AppliedAt time.Time `gorm:"autoCreateTime"`
}

func (schemaMigrationRecord) TableName() string {
	return "schema_migrations"
}

// RunEmbedded executes all embedded SQL migrations in lexical order.
// Each migration is tracked in schema_migrations by its file name without the .sql suffix.
func RunEmbedded(ctx context.Context, db *gorm.DB, logger *log.Logger) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if logger == nil {
		logger = log.Default()
	}

	if err := ensureSchemaMigrations(ctx, db); err != nil {
		return err
	}
	releaseLock, err := acquireMigrationLock(ctx, db)
	if err != nil {
		return err
	}
	defer func() {
		if releaseErr := releaseLock(); releaseErr != nil {
			logger.Printf("释放数据库迁移锁失败: %v", releaseErr)
		}
	}()

	entries, err := migrationFS.ReadDir("sql")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		version := strings.TrimSuffix(name, filepath.Ext(name))
		content, err := migrationFS.ReadFile("sql/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		checksum := migrationChecksum(content)

		record, err := findMigrationRecord(ctx, db, version)
		if err != nil {
			return err
		}
		if record != nil {
			if err := validateAppliedMigration(ctx, db, record, version, migrationDescription(name), checksum); err != nil {
				return err
			}
			continue
		}

		statements := splitSQLStatements(string(content))
		logger.Printf("数据库迁移开始: %s (%d statements)", version, len(statements))
		if err := applyMigrationStatementsWithChecksum(ctx, db, version, migrationDescription(name), checksum, statements); err != nil {
			return err
		}
		logger.Printf("数据库迁移完成: %s", version)
	}

	return nil
}

func applyMigrationStatements(ctx context.Context, db *gorm.DB, version, description string, statements []string) error {
	return applyMigrationStatementsWithChecksum(ctx, db, version, description, migrationChecksum([]byte(strings.Join(statements, "\n"))), statements)
}

func applyMigrationStatementsWithChecksum(ctx context.Context, db *gorm.DB, version, description, checksum string, statements []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// MySQL implicitly commits around most DDL. Wrapping a migration in a
	// transaction therefore gives a false rollback guarantee: earlier schema
	// changes survive even when a later statement fails.  Record a dirty state
	// before the first statement and clear it only after every statement
	// succeeds, so a subsequent deployment stops instead of re-running a
	// partially applied schema change.
	//
	// Migrations may create or remove stored procedures. MySQL rejects those
	// statements through the binary prepared-statement protocol (Error 1295),
	// so execute the SQL text directly through database/sql rather than GORM's
	// optional prepared-statement cache.
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get migration database handle: %w", err)
	}
	if err := markMigrationDirty(ctx, db, version, description, checksum); err != nil {
		return err
	}
	for idx, stmt := range statements {
		stmt = strings.TrimSpace(stmt)
		if stmt == "" {
			continue
		}
		if _, err := sqlDB.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("apply migration %s statement %d failed: %w; sql=%s", version, idx+1, err, abbreviateSQL(stmt, 240))
		}
	}
	if err := markMigrationComplete(ctx, db, version, description, checksum); err != nil {
		return err
	}
	return nil
}

func acquireMigrationLock(ctx context.Context, db *gorm.DB) (func() error, error) {
	const lockName = "caiyun_schema_migrations"
	if ctx == nil {
		ctx = context.Background()
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("acquire migration lock: get sql db: %w", err)
	}

	// MySQL named locks belong to a physical session. Reserve one connection for
	// the whole migration so RELEASE_LOCK is guaranteed to run on the same session.
	conn, err := sqlDB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("acquire migration lock: reserve connection: %w", err)
	}
	var locked sql.NullInt64
	if err := conn.QueryRowContext(ctx, "SELECT GET_LOCK(?, 60)", lockName).Scan(&locked); err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire migration lock: %w", err)
	}
	if !locked.Valid || locked.Int64 != 1 {
		_ = conn.Close()
		return nil, fmt.Errorf("acquire migration lock: timeout waiting for %s", lockName)
	}

	var once sync.Once
	var releaseErr error
	return func() error {
		once.Do(func() {
			releaseCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			defer func() {
				if err := conn.Close(); err != nil && releaseErr == nil {
					releaseErr = fmt.Errorf("close migration lock connection: %w", err)
				}
			}()

			var released sql.NullInt64
			if err := conn.QueryRowContext(releaseCtx, "SELECT RELEASE_LOCK(?)", lockName).Scan(&released); err != nil {
				releaseErr = fmt.Errorf("release migration lock: %w", err)
				return
			}
			if !released.Valid || released.Int64 != 1 {
				releaseErr = fmt.Errorf("release migration lock: lock %s was not owned by reserved connection", lockName)
			}
		})
		return releaseErr
	}, nil
}

func ensureSchemaMigrations(ctx context.Context, db *gorm.DB) error {
	return db.WithContext(ctx).AutoMigrate(&schemaMigrationRecord{})
}

func findMigrationRecord(ctx context.Context, db *gorm.DB, version string) (*schemaMigrationRecord, error) {
	var record schemaMigrationRecord
	err := db.WithContext(ctx).Where("version = ?", version).Take(&record).Error
	if err == nil {
		return &record, nil
	}
	if err == gorm.ErrRecordNotFound {
		return nil, nil
	}
	return nil, fmt.Errorf("check migration %s: %w", version, err)
}

func migrationApplied(ctx context.Context, db *gorm.DB, version string) (bool, error) {
	record, err := findMigrationRecord(ctx, db, version)
	if err != nil {
		return false, err
	}
	return record != nil && !record.Dirty, nil
}

func validateAppliedMigration(ctx context.Context, db *gorm.DB, record *schemaMigrationRecord, version, description, checksum string) error {
	if record == nil {
		return fmt.Errorf("migration %s record is nil", version)
	}
	if record.Dirty {
		return fmt.Errorf("migration %s is marked dirty; inspect and repair the partial schema change before retrying", version)
	}
	if record.Checksum == "" {
		// Existing deployments predate migration content tracking.  Preserve
		// their applied history and establish a checksum baseline now; all
		// later runs will reject content changes deterministically.
		if err := db.WithContext(ctx).Model(&schemaMigrationRecord{}).
			Where("version = ? AND checksum = '' AND dirty = ?", version, false).
			Updates(map[string]interface{}{"checksum": checksum, "description": description}).Error; err != nil {
			return fmt.Errorf("backfill migration %s checksum: %w", version, err)
		}
		return nil
	}
	if record.Checksum != checksum {
		return fmt.Errorf("migration %s checksum mismatch: applied=%s embedded=%s; do not modify an applied migration", version, record.Checksum, checksum)
	}
	return nil
}

func markMigrationDirty(ctx context.Context, db *gorm.DB, version, description, checksum string) error {
	record := &schemaMigrationRecord{
		Version:     version,
		Description: description,
		Checksum:    checksum,
		Dirty:       true,
	}
	result := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(record)
	if result.Error != nil {
		return fmt.Errorf("mark migration %s dirty: %w", version, result.Error)
	}
	if result.RowsAffected == 1 {
		return nil
	}

	existing, err := findMigrationRecord(ctx, db, version)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("mark migration %s dirty: record was not persisted", version)
	}
	if existing.Dirty {
		return fmt.Errorf("migration %s is already marked dirty; inspect and repair the partial schema change before retrying", version)
	}
	return fmt.Errorf("migration %s is already recorded as applied", version)
}

func markMigrationComplete(ctx context.Context, db *gorm.DB, version, description, checksum string) error {
	result := db.WithContext(ctx).Model(&schemaMigrationRecord{}).
		Where("version = ? AND dirty = ?", version, true).
		Updates(map[string]interface{}{
			"description": description,
			"checksum":    checksum,
			"dirty":       false,
			"applied_at":  time.Now().UTC(),
		})
	if result.Error != nil {
		return fmt.Errorf("complete migration %s: %w", version, result.Error)
	}
	if result.RowsAffected != 1 {
		return fmt.Errorf("complete migration %s: dirty migration record was not found", version)
	}
	return nil
}

func migrationChecksum(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func migrationDescription(name string) string {
	version := strings.TrimSuffix(name, filepath.Ext(name))
	parts := strings.SplitN(version, "_", 2)
	if len(parts) == 2 {
		return strings.ReplaceAll(parts[1], "_", " ")
	}
	return version
}

func splitSQLStatements(sql string) []string {
	delimiter := ";"
	var buf strings.Builder
	var statements []string

	for _, line := range strings.Split(sql, "\n") {
		trimmed := strings.TrimSpace(line)
		upper := strings.ToUpper(trimmed)
		if strings.HasPrefix(upper, "DELIMITER ") {
			newDelimiter := strings.TrimSpace(trimmed[len("DELIMITER "):])
			if newDelimiter != "" {
				delimiter = newDelimiter
			}
			continue
		}

		buf.WriteString(line)
		buf.WriteByte('\n')
		current := buf.String()
		if strings.HasSuffix(strings.TrimSpace(current), delimiter) {
			stmt := trimStatementDelimiter(current, delimiter)
			if strings.TrimSpace(stmt) != "" {
				statements = append(statements, stmt)
			}
			buf.Reset()
		}
	}

	if rest := strings.TrimSpace(buf.String()); rest != "" {
		statements = append(statements, rest)
	}
	return statements
}

func trimStatementDelimiter(stmt, delimiter string) string {
	trimmedRight := strings.TrimRight(stmt, " \t\r\n")
	if strings.HasSuffix(trimmedRight, delimiter) {
		trimmedRight = strings.TrimRight(strings.TrimSuffix(trimmedRight, delimiter), " \t\r\n")
	}
	return trimmedRight
}

func abbreviateSQL(stmt string, max int) string {
	stmt = strings.Join(strings.Fields(stmt), " ")
	if max <= 0 || len(stmt) <= max {
		return stmt
	}
	return stmt[:max] + "..."
}
