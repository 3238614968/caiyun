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

var allowedArchiveTablePairs = map[string]string{
	"task_logs":        "task_logs_archive",
	"exchange_records": "exchange_records_archive",
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
	if err := validateArchiveTables(sourceTable, archiveTable); err != nil {
		return 0, err
	}

	var moved int64
	err := r.db.Transaction(func(tx *gorm.DB) error {
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
		insertSQL := fmt.Sprintf("INSERT IGNORE INTO `%s` SELECT * FROM `%s` WHERE id IN (%s)", archiveTable, sourceTable, placeholders)
		if err := tx.Exec(insertSQL, args...).Error; err != nil {
			return err
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

func validateArchiveTables(sourceTable, archiveTable string) error {
	expectedArchive, ok := allowedArchiveTablePairs[sourceTable]
	if !ok || expectedArchive != archiveTable {
		return fmt.Errorf("unsupported archive table pair: %s -> %s", sourceTable, archiveTable)
	}
	return nil
}
