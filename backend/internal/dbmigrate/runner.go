package dbmigrate

import (
	"context"
	"database/sql"
	"embed"
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
	Version     string    `gorm:"primaryKey;size:64"`
	Description string    `gorm:"size:255;not null;default:''"`
	AppliedAt   time.Time `gorm:"autoCreateTime"`
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
		applied, err := migrationApplied(ctx, db, version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}

		content, err := migrationFS.ReadFile("sql/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}
		statements := splitSQLStatements(string(content))
		logger.Printf("数据库迁移开始: %s (%d statements)", version, len(statements))
		if err := applyMigrationStatements(ctx, db, version, migrationDescription(name), statements); err != nil {
			return err
		}
		logger.Printf("数据库迁移完成: %s", version)
	}

	return nil
}

func applyMigrationStatements(ctx context.Context, db *gorm.DB, version, description string, statements []string) error {
	if db == nil {
		return fmt.Errorf("db is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	// MySQL implicitly commits around most DDL. Wrapping a migration in a
	// transaction therefore gives a false rollback guarantee: earlier schema
	// changes survive even when a later statement fails. Execute the migration's
	// idempotent statements in order and record its version only after every
	// statement succeeds. A failed partial migration remains unrecorded and is
	// safe to retry after the underlying problem is fixed.
	//
	// Migrations may create or remove stored procedures. MySQL rejects those
	// statements through the binary prepared-statement protocol (Error 1295),
	// so execute the SQL text directly through database/sql rather than GORM's
	// optional prepared-statement cache.
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("get migration database handle: %w", err)
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
	if err := recordMigration(ctx, db, version, description); err != nil {
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

func migrationApplied(ctx context.Context, db *gorm.DB, version string) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&schemaMigrationRecord{}).Where("version = ?", version).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return count > 0, nil
}

func recordMigration(ctx context.Context, db *gorm.DB, version, description string) error {
	record := &schemaMigrationRecord{Version: version, Description: description}
	if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(record).Error; err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	return nil
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
