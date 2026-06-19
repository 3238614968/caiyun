package repository

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type SchemaRepository struct {
	db *gorm.DB
}

func NewSchemaRepository(db *gorm.DB) *SchemaRepository {
	return &SchemaRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *SchemaRepository) WithContext(ctx context.Context) *SchemaRepository {
	if ctx == nil {
		return r
	}
	return &SchemaRepository{db: r.db.WithContext(ctx)}
}

func (r *SchemaRepository) ValidateCriticalSchema() error {
	if err := r.EnsureUserSessionSchema(); err != nil {
		return err
	}
	if err := r.EnsureTaskConfigSchema(); err != nil {
		return err
	}

	checks := map[string][]string{
		"users":             {"token_version"},
		"task_configs":      {"task_type", "task_name", "description", "is_enabled", "sort_order", "run_in_batch", "updated_at", "deleted_at"},
		"accounts":          {"is_active", "jwt_error_count"},
		"products":          {"prize_id", "prize_name", "image_url", "stock_status", "is_active", "is_deleted"},
		"exchange_accounts": {"exchange_time_1", "exchange_time_2", "is_active"},
		"exchange_tasks":    {"task_type", "status", "priority", "task_group", "timeout_seconds", "max_retries", "retry_count", "last_retry_at", "last_result", "success_count", "fail_count", "deleted_at"},
	}

	for table, requiredCols := range checks {
		if err := r.validateColumns(table, requiredCols); err != nil {
			return err
		}
	}

	return nil
}

// EnsureUserSessionSchema 仅校验 users 表存在与 token_version 列存在，不再运行时 ALTER TABLE。
func (r *SchemaRepository) EnsureUserSessionSchema() error {
	exists, err := r.tableExists("users")
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("缺少 users 表，请先执行 backend/migrations/init.sql")
	}
	return nil
}

// EnsureTaskConfigSchema 仅校验 task_configs 表存在，不再运行时 ALTER TABLE。
func (r *SchemaRepository) EnsureTaskConfigSchema() error {
	exists, err := r.tableExists("task_configs")
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("缺少 task_configs 表，请先执行 backend/migrations/init.sql")
	}
	return nil
}

func (r *SchemaRepository) validateColumns(table string, requiredCols []string) error {
	columns, err := r.getColumnSet(table)
	if err != nil {
		return err
	}

	missing := make([]string, 0)
	for _, column := range requiredCols {
		if !columns[column] {
			missing = append(missing, column)
		}
	}
	if len(missing) == 0 {
		return nil
	}

	sort.Strings(missing)
	return fmt.Errorf("表 %s 缺少字段: %s，请执行 backend/migrations/init.sql 或补齐迁移", table, strings.Join(missing, ", "))
}

func (r *SchemaRepository) tableExists(table string) (bool, error) {
	var count int64
	err := r.db.Raw(`
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = DATABASE() AND table_name = ?
	`, table).Scan(&count).Error
	return count > 0, err
}

func (r *SchemaRepository) getColumnSet(table string) (map[string]bool, error) {
	exists, err := r.tableExists(table)
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, fmt.Errorf("缺少数据表 %s，请先执行 backend/migrations/init.sql", table)
	}

	var rows []struct {
		ColumnName string `gorm:"column:COLUMN_NAME"`
	}
	if err := r.db.Raw(`
		SELECT COLUMN_NAME
		FROM information_schema.columns
		WHERE table_schema = DATABASE() AND table_name = ?
	`, table).Scan(&rows).Error; err != nil {
		return nil, err
	}

	columns := make(map[string]bool, len(rows))
	for _, row := range rows {
		columns[row.ColumnName] = true
	}
	return columns, nil
}
