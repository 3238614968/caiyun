package services

import (
	"context"

	"caiyun/internal/models"
)

// OperationEventPublisher is an application output port. Transports such as
// SSE/WebSocket implement it in an adapter package; the operation state
// machine therefore has no direct dependency on Gin, Redis or WebSocket.
type OperationEventPublisher interface {
	PublishOperationUpdated(ctx context.Context, userID uint, update models.OperationUpdate)
}

func operationUpdate(operation *models.Operation) models.OperationUpdate {
	if operation == nil {
		return models.OperationUpdate{}
	}
	return models.OperationUpdate{
		OperationID:  operation.ID,
		Type:         operation.OperationType,
		Status:       operation.Status,
		AccountID:    operation.AccountID,
		ResourceID:   operation.ResourceID,
		AttemptCount: operation.AttemptCount,
		// Raw operation errors can contain upstream implementation details.
		// Keep the event consistent with the HTTP operation representation.
		ErrorSummary: operationPublicError(operation),
		QueuedAt:     operation.QueuedAt,
		StartedAt:    operation.StartedAt,
		CompletedAt:  operation.CompletedAt,
		UpdatedAt:    operation.UpdatedAt,
	}
}

func operationPublicError(operation *models.Operation) string {
	if operation == nil || operation.Status != models.OperationFailed {
		return ""
	}
	return "操作执行失败，请稍后重试"
}
