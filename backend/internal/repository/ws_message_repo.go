package repository

import (
	"caiyun/internal/models"
	"context"
	"encoding/json"
	"time"

	"gorm.io/gorm"
)

// WSMessageRepository WebSocket消息仓库
type WSMessageRepository struct {
	db *gorm.DB
}

// NewWSMessageRepository 创建WebSocket消息仓库
func NewWSMessageRepository(db *gorm.DB) *WSMessageRepository {
	return &WSMessageRepository{db: db}
}

// WithContext 返回绑定到指定 context 的仓库副本，便于数据库操作响应请求取消和超时。
func (r *WSMessageRepository) WithContext(ctx context.Context) *WSMessageRepository {
	if ctx == nil {
		return r
	}
	return &WSMessageRepository{db: r.db.WithContext(ctx)}
}

// Create 创建消息
func (r *WSMessageRepository) Create(message *models.WebSocketMessage) error {
	return r.db.Create(message).Error
}

// GetUnreadMessages 获取用户的未读消息
func (r *WSMessageRepository) GetUnreadMessages(userID uint, limit int) ([]*models.WebSocketMessage, error) {
	var messages []*models.WebSocketMessage
	err := r.db.Where("user_id = ? AND is_read = ?", userID, false).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	return messages, err
}

// GetUndeliveredMessages 获取用户的未送达消息
func (r *WSMessageRepository) GetUndeliveredMessages(userID uint, limit int) ([]*models.WebSocketMessage, error) {
	var messages []*models.WebSocketMessage
	err := r.db.Where("user_id = ? AND is_delivered = ?", userID, false).
		Order("created_at DESC").
		Limit(limit).
		Find(&messages).Error
	return messages, err
}

// MarkAsRead 标记指定用户自己的消息为已读，避免仅凭 messageID 越权修改其他用户消息。
func (r *WSMessageRepository) MarkAsRead(userID, messageID uint) error {
	now := time.Now()
	query := r.db.Model(&models.WebSocketMessage{}).
		Where("id = ? AND user_id = ?", messageID, userID)
	return query.Updates(map[string]interface{}{
		"is_read": true,
		"read_at": now,
	}).Error
}

// MarkAsDelivered 标记消息为已送达
func (r *WSMessageRepository) MarkAsDelivered(messageID uint) error {
	now := time.Now()
	return r.db.Model(&models.WebSocketMessage{}).
		Where("id = ?", messageID).
		Updates(map[string]interface{}{
			"is_delivered": true,
			"delivered_at": now,
		}).Error
}

// MarkAllAsReadByUser 标记用户的所有消息为已读
func (r *WSMessageRepository) MarkAllAsReadByUser(userID uint) error {
	now := time.Now()
	return r.db.Model(&models.WebSocketMessage{}).
		Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{
			"is_read": true,
			"read_at": now,
		}).Error
}

// DeleteOldMessages 删除旧消息（清理策略）
func (r *WSMessageRepository) DeleteOldMessages(before time.Time) error {
	return r.db.Where("created_at < ?", before).Delete(&models.WebSocketMessage{}).Error
}

// GetMessageCount 获取用户的消息数量
func (r *WSMessageRepository) GetMessageCount(userID uint, isRead bool) (int64, error) {
	var count int64
	err := r.db.Model(&models.WebSocketMessage{}).
		Where("user_id = ? AND is_read = ?", userID, isRead).
		Count(&count).Error
	return count, err
}

// SaveMessage 保存WebSocket消息（便捷方法）
func (r *WSMessageRepository) SaveMessage(userID uint, msgType string, data interface{}) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}

	message := &models.WebSocketMessage{
		UserID: userID,
		Type:   msgType,
		Data:   string(dataJSON),
	}

	return r.Create(message)
}
