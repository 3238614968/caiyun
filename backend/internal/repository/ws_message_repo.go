package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"caiyun/internal/models"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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
	_, err := r.PersistMessageEnvelope(userID, msgType, data, uuid.NewString(), time.Now().Add(24*time.Hour))
	return err
}

// PersistMessageEnvelope allocates and writes a user envelope in one database
// transaction. Redis Pub/Sub is a transport only; this table is the ordering
// authority used by SSE Last-Event-ID replay across API replicas.
func (r *WSMessageRepository) PersistMessageEnvelope(userID uint, msgType string, data interface{}, messageID string, expiresAt time.Time) (uint64, error) {
	if r == nil || r.db == nil {
		return 0, fmt.Errorf("WebSocket message repository is not configured")
	}
	if userID == 0 {
		return 0, fmt.Errorf("WebSocket message user id is required")
	}
	dataJSON, err := json.Marshal(data)
	if err != nil {
		return 0, err
	}
	if messageID == "" {
		messageID = uuid.NewString()
	}

	var sequence uint64
	err = r.db.Transaction(func(tx *gorm.DB) error {
		var allocator models.WebSocketSequence
		if err := tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "user_id"}},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"sequence": gorm.Expr("sequence + 1"),
			}),
		}).Create(&models.WebSocketSequence{UserID: userID, Sequence: 1}).Error; err != nil {
			return err
		}
		if err := tx.Where("user_id = ?", userID).First(&allocator).Error; err != nil {
			return err
		}
		sequence = allocator.Sequence
		return tx.Create(&models.WebSocketMessage{
			UserID: userID, Type: msgType, Data: string(dataJSON), MessageID: messageID,
			Sequence: sequence, ExpiresAt: &expiresAt,
		}).Error
	})
	if err != nil {
		return 0, err
	}
	return sequence, nil
}
