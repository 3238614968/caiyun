package handlers

import (
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/services"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

type OperationHandler struct {
	operationService *services.OperationService
}

func NewOperationHandler(operationService *services.OperationService) *OperationHandler {
	return &OperationHandler{operationService: operationService}
}

type OperationResponse struct {
	ID           string                 `json:"operation_id"`
	Type         string                 `json:"type"`
	Status       models.OperationStatus `json:"status"`
	AccountID    uint                   `json:"account_id,omitempty"`
	ResourceID   uint                   `json:"resource_id,omitempty"`
	AttemptCount int                    `json:"attempt_count"`
	ErrorSummary string                 `json:"error_summary,omitempty"`
	QueuedAt     time.Time              `json:"queued_at"`
	StartedAt    *time.Time             `json:"started_at,omitempty"`
	CompletedAt  *time.Time             `json:"completed_at,omitempty"`
	CreatedAt    time.Time              `json:"created_at"`
	UpdatedAt    time.Time              `json:"updated_at"`
}

func operationResponse(operation *models.Operation) OperationResponse {
	if operation == nil {
		return OperationResponse{}
	}
	return OperationResponse{
		ID:           operation.ID,
		Type:         operation.OperationType,
		Status:       operation.Status,
		AccountID:    operation.AccountID,
		ResourceID:   operation.ResourceID,
		AttemptCount: operation.AttemptCount,
		ErrorSummary: publicOperationError(operation),
		QueuedAt:     operation.QueuedAt,
		StartedAt:    operation.StartedAt,
		CompletedAt:  operation.CompletedAt,
		CreatedAt:    operation.CreatedAt,
		UpdatedAt:    operation.UpdatedAt,
	}
}

func publicOperationError(operation *models.Operation) string {
	if operation == nil || operation.Status != models.OperationFailed {
		return ""
	}
	return "操作执行失败，请稍后重试"
}

func requestIdempotencyKey(c *gin.Context, fallback string) (string, error) {
	key := strings.TrimSpace(c.GetHeader("Idempotency-Key"))
	if key == "" {
		key = fallback
	}
	if len(key) > 191 {
		return "", errors.New("Idempotency-Key 长度不能超过 191 字节")
	}
	return key, nil
}

func respondOperationAccepted(c *gin.Context, operation *models.Operation, dispatchErr error) {
	if dispatchErr != nil {
		// The database row is the durable outbox. A Worker will redispatch it;
		// never expose Redis internals or downgrade the accepted operation.
		log.Printf("operation immediate dispatch deferred: operation_id=%s err=%v", operation.ID, dispatchErr)
	}
	apiresponse.Accepted(c, operationResponse(operation))
}

func respondOperationSubmitError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, services.ErrOperationIdempotencyConflict):
		respondError(c, http.StatusConflict, "Idempotency-Key 已用于其他操作")
	default:
		if err != nil {
			_ = c.Error(err)
		}
		respondInternalServer(c)
	}
}

func (h *OperationHandler) Get(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	operation, err := h.operationService.GetForUser(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		if errors.Is(err, services.ErrOperationNotFound) {
			respondError(c, http.StatusNotFound, "操作不存在")
			return
		}
		_ = c.Error(err)
		respondInternalServer(c)
		return
	}
	apiresponse.Success(c, operationResponse(operation))
}

func (h *OperationHandler) Cancel(c *gin.Context) {
	userID, ok := getUserID(c)
	if !ok {
		return
	}
	operation, err := h.operationService.Cancel(c.Request.Context(), c.Param("id"), userID)
	if err != nil {
		switch {
		case errors.Is(err, services.ErrOperationNotFound):
			respondError(c, http.StatusNotFound, "操作不存在")
		case errors.Is(err, services.ErrOperationNotCancelable):
			respondError(c, http.StatusConflict, "仅排队中的操作可以取消")
		default:
			_ = c.Error(err)
			respondInternalServer(c)
		}
		return
	}
	apiresponse.Success(c, operationResponse(operation))
}
