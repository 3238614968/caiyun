package ws

import (
	"context"
	"testing"
	"time"

	"caiyun/internal/models"
)

func TestOperationEventPublisherDeliversPublicOperationUpdate(t *testing.T) {
	hub := startTestHub(t)
	client := &SSEClient{userID: 42, send: make(chan Message, 1)}
	hub.registerSSE(client)
	t.Cleanup(func() { hub.unregisterSSE(client) })

	publisher := NewOperationEventPublisher(hub)
	publisher.PublishOperationUpdated(context.Background(), 42, models.OperationUpdate{
		OperationID:  "op-42",
		Type:         models.OperationTypeExchangeMonthly,
		Status:       models.OperationSucceeded,
		AttemptCount: 1,
		UpdatedAt:    time.Now().UTC(),
	})

	select {
	case message := <-client.send:
		if message.Type != "operation.updated" || message.UserID != 42 || message.MessageID == "" || message.Sequence == 0 {
			t.Fatalf("unexpected operation envelope: %+v", message)
		}
		update, ok := message.Data.(models.OperationUpdate)
		if !ok || update.OperationID != "op-42" || update.Status != models.OperationSucceeded {
			t.Fatalf("unexpected operation update: %#v", message.Data)
		}
	case <-time.After(time.Second):
		t.Fatal("operation update was not delivered to SSE client")
	}
}
