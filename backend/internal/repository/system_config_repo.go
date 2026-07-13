package repository

import (
	"caiyun/internal/models"
	"context"
	"time"

	"gorm.io/gorm"
)

type SystemConfigRepository struct {
	db *gorm.DB
}

// SystemConfigUpdate 表示一次配置写入。
type SystemConfigUpdate struct {
	Key         string
	Value       string
	Description string
}

func NewSystemConfigRepository(db *gorm.DB) *SystemConfigRepository {
	return &SystemConfigRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *SystemConfigRepository) WithContext(ctx context.Context) *SystemConfigRepository {
	if ctx == nil {
		return r
	}
	return &SystemConfigRepository{db: r.db.WithContext(ctx)}
}

// GetByKey 根据 key 获取配置
func (r *SystemConfigRepository) GetByKey(key string) (*models.SystemConfig, error) {
	var config models.SystemConfig
	err := r.db.Where("key_name = ?", key).First(&config).Error
	if err != nil {
		return nil, err
	}
	return &config, nil
}

// Set 设置配置值
func (r *SystemConfigRepository) Set(key, value string) error {
	config := &models.SystemConfig{
		KeyName:  key,
		KeyValue: value,
	}

	// 尝试更新，如果没有记录则插入
	result := r.db.Model(&models.SystemConfig{}).
		Where("key_name = ?", key).
		Update("key_value", value)

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return r.db.Create(config).Error
	}

	return nil
}

// UpdateByKey 根据 key 更新配置
func (r *SystemConfigRepository) UpdateByKey(key, value, description string) error {
	config := &models.SystemConfig{
		KeyName:     key,
		KeyValue:    value,
		Description: description,
	}

	// 尝试更新，如果没有记录则插入
	result := r.db.Model(&models.SystemConfig{}).
		Where("key_name = ?", key).
		Updates(map[string]interface{}{
			"key_value":   value,
			"description": description,
			"updated_at":  time.Now(),
		})

	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return r.db.Create(config).Error
	}

	return nil
}

// BatchUpdate 在同一事务中批量更新配置，避免校验通过后的部分写入。
func (r *SystemConfigRepository) BatchUpdate(updates []SystemConfigUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		txRepo := &SystemConfigRepository{db: tx}
		for _, update := range updates {
			if err := txRepo.UpdateByKey(update.Key, update.Value, update.Description); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetAll 获取所有配置
func (r *SystemConfigRepository) GetAll() ([]*models.SystemConfig, error) {
	var configs []*models.SystemConfig
	err := r.db.Find(&configs).Error
	return configs, err
}

// Delete 删除配置
func (r *SystemConfigRepository) Delete(key string) error {
	return r.db.Where("key_name = ?", key).Delete(&models.SystemConfig{}).Error
}
