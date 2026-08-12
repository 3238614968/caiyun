package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	defaultArchiveBatchSize = 1000
	maxArchiveBatchSize     = 10000
)

type archiveTableSpec struct {
	archiveTable string
	columns      []string
}

// Archive columns are intentionally explicit.  `INSERT ... SELECT *` made the
// archive operation depend on two independently evolving table definitions;
// adding exchange_rule_id to live exchange_records after the archive table had
// been created with CREATE TABLE LIKE caused positional copies to fail.  Keep
// this list synchronized with migration 021's archive-parity DDL.
var archiveTableSpecs = map[string]archiveTableSpec{
	"task_logs": {
		archiveTable: "task_logs_archive",
		columns: []string{
			"id", "user_id", "account_id", "task_type", "status", "message",
			"cloud_gained", "execution_time", "created_at", "deleted_at",
		},
	},
	"exchange_records": {
		archiveTable: "exchange_records_archive",
		columns: []string{
			"id", "user_id", "exchange_account_id", "exchange_rule_id", "exchange_task_id",
			"product_id", "prize_id", "prize_name", "status", "message", "execution_time_ms", "created_at",
		},
	},
}

// HistoryArchiveRepository 负责将冷热数据从主表归档到 archive 表。
type HistoryArchiveRepository struct {
	db *gorm.DB
}

func NewHistoryArchiveRepository(db *gorm.DB) *HistoryArchiveRepository {
	return &HistoryArchiveRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应取消与超时。
func (r *HistoryArchiveRepository) WithContext(ctx context.Context) *HistoryArchiveRepository {
	if ctx == nil {
		return r
	}
	return &HistoryArchiveRepository{db: r.db.WithContext(ctx)}
}

func (r *HistoryArchiveRepository) ArchiveTaskLogsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return r.WithContext(ctx).archiveBatch("task_logs", "task_logs_archive", cutoff, batchSize)
}

func (r *HistoryArchiveRepository) ArchiveExchangeRecordsBefore(ctx context.Context, cutoff time.Time, batchSize int) (int64, error) {
	return r.WithContext(ctx).archiveBatch("exchange_records", "exchange_records_archive", cutoff, batchSize)
}

func (r *HistoryArchiveRepository) archiveBatch(sourceTable, archiveTable string, cutoff time.Time, batchSize int) (int64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("archive repository 未初始化")
	}
	batchSize = normalizeArchiveBatchSize(batchSize)
	spec, err := archiveTableSpecFor(sourceTable, archiveTable)
	if err != nil {
		return 0, err
	}

	var moved int64
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var ids []uint
		if err := tx.Table(sourceTable).
			Select("id").
			Where("created_at < ?", cutoff).
			Order("id ASC").
			Limit(batchSize).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			moved = 0
			return nil
		}

		placeholders, args := archiveIDPlaceholders(ids)
		// A duplicate archive primary key must abort this transaction.  INSERT
		// IGNORE followed by DELETE could silently drop the source row when a
		// previous partial/manual archive had already copied that ID.
		insertSQL := buildArchiveInsertSQL(sourceTable, archiveTable, spec.columns, placeholders)
		insertResult := tx.Exec(insertSQL, args...)
		if insertResult.Error != nil {
			return insertResult.Error
		}
		if insertResult.RowsAffected != int64(len(ids)) {
			return fmt.Errorf("archive %s -> %s inserted %d rows, expected %d", sourceTable, archiveTable, insertResult.RowsAffected, len(ids))
		}

		deleteSQL := fmt.Sprintf("DELETE FROM `%s` WHERE id IN (%s)", sourceTable, placeholders)
		result := tx.Exec(deleteSQL, args...)
		if result.Error != nil {
			return result.Error
		}
		moved = result.RowsAffected
		return nil
	})
	if err != nil {
		return 0, err
	}
	return moved, nil
}

func normalizeArchiveBatchSize(batchSize int) int {
	if batchSize <= 0 || batchSize > maxArchiveBatchSize {
		return defaultArchiveBatchSize
	}
	return batchSize
}

func archiveIDPlaceholders(ids []uint) (string, []interface{}) {
	parts := make([]string, 0, len(ids))
	args := make([]interface{}, 0, len(ids))
	for _, id := range ids {
		parts = append(parts, "?")
		args = append(args, id)
	}
	return strings.Join(parts, ", "), args
}

func archiveTableSpecFor(sourceTable, archiveTable string) (archiveTableSpec, error) {
	spec, ok := archiveTableSpecs[sourceTable]
	if !ok || spec.archiveTable != archiveTable {
		return archiveTableSpec{}, fmt.Errorf("unsupported archive table pair: %s -> %s", sourceTable, archiveTable)
	}
	return spec, nil
}

func buildArchiveInsertSQL(sourceTable, archiveTable string, columns []string, placeholders string) string {
	quotedColumns := make([]string, 0, len(columns))
	for _, column := range columns {
		quotedColumns = append(quotedColumns, "`"+column+"`")
	}
	columnList := strings.Join(quotedColumns, ", ")
	return fmt.Sprintf(
		"INSERT INTO `%s` (%s) SELECT %s FROM `%s` WHERE `id` IN (%s)",
		archiveTable,
		columnList,
		columnList,
		sourceTable,
		placeholders,
	)
}
