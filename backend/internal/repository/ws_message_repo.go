package repository

import (
	"context"
	"encoding/json"
	"time"

	"caiyun/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// WSMessageRepository persists WebSocket messages until the browser explicitly
// acknowledges them. Queuing a frame is not treated as delivery.
type WSMessageRepository struct {
	db *gorm.DB
}

func NewWSMessageRepository(db *gorm.DB) *WSMessageRepository { return &WSMessageRepository{db: db} }

func (r *WSMessageRepository) WithContext(ctx context.Context) *WSMessageRepository {
	if r == nil || ctx == nil {
		return r
	}
	return &WSMessageRepository{db: r.db.WithContext(ctx)}
}

func (r *WSMessageRepository) Create(message *models.WebSocketMessage) error {
	return r.db.Create(message).Error
}

func (r *WSMessageRepository) GetUnreadMessages(userID uint, limit int) ([]*models.WebSocketMessage, error) {
	var messages []*models.WebSocketMessage
	err := r.db.Where("user_id = ? AND is_read = ?", userID, false).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("sequence ASC, created_at ASC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (r *WSMessageRepository) GetUndeliveredMessages(userID uint, limit int) ([]*models.WebSocketMessage, error) {
	var messages []*models.WebSocketMessage
	err := r.db.Where("user_id = ? AND is_delivered = ?", userID, false).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).
		Order("sequence ASC, created_at ASC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (r *WSMessageRepository) MarkAsRead(userID, messageID uint) error {
	now := time.Now()
	return r.db.Model(&models.WebSocketMessage{}).Where("id = ? AND user_id = ?", messageID, userID).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *WSMessageRepository) MarkAsDelivered(messageID uint) error {
	now := time.Now()
	return r.db.Model(&models.WebSocketMessage{}).Where("id = ?", messageID).
		Updates(map[string]interface{}{"is_delivered": true, "delivered_at": now, "acked_at": now}).Error
}

// MarkAsDeliveredByMessageID scopes ACK by authenticated user and opaque ID.
func (r *WSMessageRepository) GetMessagesAfterSequence(userID uint, sequence uint64, limit int) ([]*models.WebSocketMessage, error) {
	var messages []*models.WebSocketMessage
	err := r.db.Where("user_id = ? AND sequence > ?", userID, sequence).Where("expires_at IS NULL OR expires_at > ?", time.Now()).Order("sequence ASC, created_at ASC").Limit(limit).Find(&messages).Error
	return messages, err
}

func (r *WSMessageRepository) MarkAsDeliveredByMessageID(userID uint, messageID string) (bool, error) {
	if messageID == "" {
		return false, nil
	}
	now := time.Now()
	result := r.db.Model(&models.WebSocketMessage{}).
		Where("user_id = ? AND message_id = ? AND is_delivered = ?", userID, messageID, false).
		Updates(map[string]interface{}{"is_delivered": true, "delivered_at": now, "acked_at": now})
	return result.RowsAffected == 1, result.Error
}

func (r *WSMessageRepository) MarkAllAsReadByUser(userID uint) error {
	now := time.Now()
	return r.db.Model(&models.WebSocketMessage{}).Where("user_id = ? AND is_read = ?", userID, false).
		Updates(map[string]interface{}{"is_read": true, "read_at": now}).Error
}

func (r *WSMessageRepository) DeleteOldMessages(before time.Time) error {
	return r.db.Where("created_at < ? OR (expires_at IS NOT NULL AND expires_at < ?)", before, time.Now()).Delete(&models.WebSocketMessage{}).Error
}

func (r *WSMessageRepository) GetMessageCount(userID uint, isRead bool) (int64, error) {
	var count int64
	err := r.db.Model(&models.WebSocketMessage{}).Where("user_id = ? AND is_read = ?", userID, isRead).
		Where("expires_at IS NULL OR expires_at > ?", time.Now()).Count(&count).Error
	return count, err
}

func (r *WSMessageRepository) SaveMessage(userID uint, msgType string, data interface{}) error {
	return r.SaveMessageEnvelope(userID, msgType, data, uuid.NewString(), 0, time.Now().Add(24*time.Hour))
}

func (r *WSMessageRepository) SaveMessageEnvelope(userID uint, msgType string, data interface{}, messageID string, sequence uint64, expiresAt time.Time) error {
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if messageID == "" {
		messageID = uuid.NewString()
	}
	return r.Create(&models.WebSocketMessage{UserID: userID, Type: msgType, Data: string(dataJSON), MessageID: messageID, Sequence: sequence, ExpiresAt: &expiresAt})
}
