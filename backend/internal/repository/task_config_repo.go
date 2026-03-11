package repository

import (
	"strings"
	"time"

	"caiyun/internal/models"

	"gorm.io/gorm"
)

type TaskConfigRepository struct {
	db *gorm.DB
}

func NewTaskConfigRepository(db *gorm.DB) *TaskConfigRepository {
	return &TaskConfigRepository{db: db}
}

// AutoMigrate creates the task_configs table if not exists.
func (r *TaskConfigRepository) AutoMigrate() error {
	return r.db.AutoMigrate(&models.TaskConfig{})
}

// InitDefaults 仅保留兼容入口，新的默认任务定义请使用 SyncDefinitions。
func (r *TaskConfigRepository) InitDefaults() error {
	return nil
}

// SyncDefinitions 将代码中的任务注册表同步到数据库。
// 已存在任务保留管理员设置的 is_enabled，仅刷新描述、排序和批次标记。
func (r *TaskConfigRepository) SyncDefinitions(defs []models.TaskConfig) error {
	activeCodes := make([]string, 0, len(defs))
	seen := make(map[string]struct{}, len(defs))

	for _, def := range defs {
		def.TaskType = normalizeTaskCode(def.TaskType)
		if def.TaskType == "" {
			continue
		}
		if _, ok := seen[def.TaskType]; ok {
			continue
		}
		seen[def.TaskType] = struct{}{}
		activeCodes = append(activeCodes, def.TaskType)

		var existing models.TaskConfig
		query := r.db.Unscoped().Where("task_type = ?", def.TaskType).Limit(1).Find(&existing)
		if query.Error != nil {
			return query.Error
		}

		if query.RowsAffected == 0 {
			if err := r.db.Create(&def).Error; err != nil {
				return err
			}
			continue
		}

		existing.TaskName = def.TaskName
		existing.Description = def.Description
		existing.SortOrder = def.SortOrder
		existing.RunInBatch = def.RunInBatch
		existing.UpdatedAt = time.Now()
		existing.DeletedAt.Valid = false
		if err := r.db.Unscoped().Save(&existing).Error; err != nil {
			return err
		}
	}

	if len(activeCodes) > 0 {
		if err := r.db.Where("task_type NOT IN ?", activeCodes).Delete(&models.TaskConfig{}).Error; err != nil {
			return err
		}
	}

	return nil
}

// List returns all task configs ordered by sort_order.
func (r *TaskConfigRepository) List() ([]*models.TaskConfig, error) {
	var configs []*models.TaskConfig
	err := r.db.Order("sort_order ASC, id ASC").Find(&configs).Error
	return configs, err
}

// FindByTaskType finds a task config by task type.
func (r *TaskConfigRepository) FindByTaskType(taskType string) (*models.TaskConfig, error) {
	var config models.TaskConfig
	err := r.db.Where("task_type = ?", normalizeTaskCode(taskType)).First(&config).Error
	return &config, err
}

// UpdateEnabled toggles a task's enabled status.
func (r *TaskConfigRepository) UpdateEnabled(taskType string, isEnabled bool) error {
	return r.db.Model(&models.TaskConfig{}).
		Where("task_type = ?", normalizeTaskCode(taskType)).
		Update("is_enabled", isEnabled).Error
}

// GetDisabledTaskTypes returns a set of disabled task type strings.
func (r *TaskConfigRepository) GetDisabledTaskTypes() (map[string]bool, error) {
	var configs []*models.TaskConfig
	err := r.db.Where("is_enabled = ?", false).Find(&configs).Error
	if err != nil {
		return nil, err
	}

	result := make(map[string]bool, len(configs))
	for _, c := range configs {
		result[normalizeTaskCode(c.TaskType)] = true
	}
	return result, nil
}

func normalizeTaskCode(code string) string {
	return strings.ToLower(strings.TrimSpace(code))
}
