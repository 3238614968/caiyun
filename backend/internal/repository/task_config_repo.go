package repository

import (
	"context"
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

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *TaskConfigRepository) WithContext(ctx context.Context) *TaskConfigRepository {
	if ctx == nil {
		return r
	}
	return &TaskConfigRepository{db: r.db.WithContext(ctx)}
}

// AutoMigrate creates the task_configs table if not exists.
// 若表已存在且扩展列迁移失败，则保持兼容运行，避免因历史库结构导致启动失败。
func (r *TaskConfigRepository) AutoMigrate() error {
	err := r.db.AutoMigrate(&models.TaskConfig{})
	if err == nil {
		return nil
	}
	if r.db.Migrator().HasTable(&models.TaskConfig{}) {
		return nil
	}
	return err
}

// SyncDefinitions 将代码中的任务注册表同步到数据库。
// 已存在任务保留管理员设置的 is_enabled，仅刷新描述、排序和批次标记。
func (r *TaskConfigRepository) SyncDefinitions(defs []models.TaskConfig) error {
	hasDescription := r.db.Migrator().HasColumn(&models.TaskConfig{}, "description")
	hasRunInBatch := r.db.Migrator().HasColumn(&models.TaskConfig{}, "run_in_batch")

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
			createData := map[string]interface{}{
				"task_type":  def.TaskType,
				"task_name":  def.TaskName,
				"is_enabled": def.IsEnabled,
				"sort_order": def.SortOrder,
			}
			if hasDescription {
				createData["description"] = def.Description
			}
			if hasRunInBatch {
				createData["run_in_batch"] = def.RunInBatch
			}
			if err := r.db.Model(&models.TaskConfig{}).Create(createData).Error; err != nil {
				return err
			}
			continue
		}

		updates := map[string]interface{}{
			"task_name":  def.TaskName,
			"sort_order": def.SortOrder,
			"updated_at": time.Now(),
			"deleted_at": nil,
		}
		if hasDescription {
			updates["description"] = def.Description
		}
		if hasRunInBatch {
			updates["run_in_batch"] = def.RunInBatch
		}
		if err := r.db.Unscoped().Model(&models.TaskConfig{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
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
