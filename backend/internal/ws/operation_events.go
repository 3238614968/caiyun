package ws

import (
	"context"

	"caiyun/internal/models"
)

// OperationEventPublisher adapts the application OperationEventPublisher port
// to the durable SSE/WebSocket hub. SendToUser persists the envelope before
// fan-out, so reconnecting clients receive missed state transitions by replay.
type OperationEventPublisher struct {
	hub *Hub
}

func NewOperationEventPublisher(hub *Hub) *OperationEventPublisher {
	return &OperationEventPublisher{hub: hub}
}

func (p *OperationEventPublisher) PublishOperationUpdated(_ context.Context, userID uint, update models.OperationUpdate) {
	if p == nil || p.hub == nil || userID == 0 || update.OperationID == "" {
		return
	}
	p.hub.SendToUser(userID, Message{
		Type: "operation.updated",
		Data: update,
	})
}
