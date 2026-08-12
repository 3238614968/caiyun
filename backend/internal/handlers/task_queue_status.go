package handlers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"caiyun/internal/models"
	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/internal/services"
	"caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

// QueueStatusHandler 负责返回任务队列与任务监控统计信息。
type QueueStatusHandler struct {
	taskQueue        queue.ReliableTaskQueue
	taskMonitor      *monitor.TaskMonitor
	operationService *services.OperationService
}

// NewQueueStatusHandler creates a queue status handler. The optional monitor
// preserves constructor compatibility for focused tests while application
// assembly passes its owned monitor explicitly.
func NewQueueStatusHandler(taskQueue queue.ReliableTaskQueue, taskMonitors ...*monitor.TaskMonitor) *QueueStatusHandler {
	var taskMonitor *monitor.TaskMonitor
	if len(taskMonitors) > 0 {
		taskMonitor = taskMonitors[0]
	}
	return &QueueStatusHandler{taskQueue: taskQueue, taskMonitor: taskMonitor}
}

func (h *QueueStatusHandler) SetOperationService(service *services.OperationService) {
	if h != nil {
		h.operationService = service
	}
}

// GetQueueStatus 获取队列状态与监控统计。
func (h *QueueStatusHandler) GetQueueStatus(c *gin.Context) {
	var queueLength int64
	var processingLength int64
	var delayedLength int64
	var deadLetterLength int64
	metadata := queue.MetadataOf(h.taskQueue)
	errors := make([]string, 0)
	if h.taskQueue != nil {
		queueLength = collectQueueMetric(&errors, "queue_length", h.taskQueue.GetQueueLength)
		processingLength = collectQueueMetric(&errors, "processing_count", h.taskQueue.GetProcessingLength)
		delayedLength = collectQueueMetric(&errors, "delayed_count", h.taskQueue.GetDelayedLength)
		deadLetterLength = collectQueueMetric(&errors, "dead_letter_count", h.taskQueue.GetDeadLetterLength)
	} else {
		errors = append(errors, "task_queue_not_configured")
	}

	activeWorkers := int32(0)
	completedTasks := int32(0)
	successfulTasks := int32(0)
	failedTasks := int32(0)

	if h.taskMonitor != nil {
		stats := h.taskMonitor.GetStats()
		activeWorkers = toInt32(stats["active_tasks"])
		completedTasks = toInt32(stats["completed_tasks"])
		successfulTasks = toInt32(stats["successful"])
		failedTasks = toInt32(stats["failed"])
	}

	response.Success(c, QueueStatusResponse{
		QueueLength:     queueLength,
		ProcessingCount: processingLength,
		DelayedCount:    delayedLength,
		DeadLetterCount: deadLetterLength,
		ActiveWorkers:   activeWorkers,
		PendingTasks:    int(queueLength + processingLength + delayedLength),
		CompletedTasks:  completedTasks,
		SuccessfulTasks: successfulTasks,
		FailedTasks:     failedTasks,
		Backend:         metadata.Backend,
		BackendMeta:     metadata,
		IsHealthy:       len(errors) == 0,
		Errors:          errors,
	})
}

// ListDeadLetters exposes a bounded, redacted inspection view to admins. The
// route is wrapped by AdminMiddleware and AuditMiddleware at registration.
func (h *QueueStatusHandler) ListDeadLetters(c *gin.Context) {
	replayQueue, ok := h.taskQueue.(queue.DeadLetterReplayQueue)
	if !ok {
		response.ErrorWithBusinessCode(c, http.StatusNotImplemented, "DLQ_REPLAY_UNSUPPORTED", "dead-letter replay is not configured")
		return
	}
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	items, err := replayQueue.ListDeadLetters(limit)
	if err != nil {
		response.Error(c, err)
		return
	}
	response.Success(c, gin.H{"dead_letters": items, "total": len(items)})
}

type replayDeadLetterRequest struct {
	Approval string `json:"approval" binding:"required"`
	Reason   string `json:"reason" binding:"required"`
}

// ReplayDeadLetter requires an explicit approval marker and reason. Generic
// task messages are atomically moved back to the queue; operation messages
// first restore their MySQL lifecycle row and are then archived from the DLQ.
func (h *QueueStatusHandler) ReplayDeadLetter(c *gin.Context) {
	replayQueue, ok := h.taskQueue.(queue.DeadLetterReplayQueue)
	if !ok {
		response.ErrorWithBusinessCode(c, http.StatusNotImplemented, "DLQ_REPLAY_UNSUPPORTED", "dead-letter replay is not configured")
		return
	}
	var request replayDeadLetterRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		response.BadRequest(c, "approval and reason are required")
		return
	}
	if request.Approval != "approved" || len(strings.TrimSpace(request.Reason)) < 8 {
		response.ErrorWithBusinessCode(c, http.StatusConflict, "DLQ_REPLAY_APPROVAL_REQUIRED", "approval must be approved and reason must contain at least 8 characters")
		return
	}
	id := strings.TrimSpace(c.Param("id"))
	item, err := replayQueue.GetDeadLetter(id)
	if err != nil {
		writeDeadLetterError(c, err)
		return
	}
	if item.Task != nil && item.Task.OperationID != "" {
		if h.operationService == nil {
			response.ServiceUnavailable(c, "operation replay service is not configured")
			return
		}
		// A previous approved attempt may have committed MySQL but lost the
		// Redis DLQ archive step. The queued row's outbox is sufficient for
		// delivery, so this retry only finalizes the administrative cleanup.
		if existing, getErr := h.operationService.Get(c.Request.Context(), item.Task.OperationID); getErr == nil && existing.Status == models.OperationQueued {
			if err := replayQueue.ArchiveDeadLetter(item.ID); err != nil {
				response.Error(c, err)
				return
			}
			response.SuccessWithMessage(c, "already queued operation dead-letter archived", gin.H{"dead_letter_id": item.ID, "operation": existing})
			return
		}
		operation, replayErr := h.operationService.ReplayFailed(c.Request.Context(), item.Task.OperationID)
		if replayErr != nil {
			if errors.Is(replayErr, services.ErrOperationNotReplayable) {
				response.ErrorWithBusinessCode(c, http.StatusConflict, "OPERATION_NOT_REPLAYABLE", replayErr.Error())
				return
			}
			response.Error(c, replayErr)
			return
		}
		if err := replayQueue.ArchiveDeadLetter(item.ID); err != nil {
			response.Error(c, err)
			return
		}
		response.SuccessWithMessage(c, "operation replay approved", gin.H{"dead_letter_id": item.ID, "operation": operation})
		return
	}
	message, err := replayQueue.ReplayDeadLetter(item.ID)
	if err != nil {
		writeDeadLetterError(c, err)
		return
	}
	response.SuccessWithMessage(c, "dead-letter replay approved", gin.H{"dead_letter_id": item.ID, "task": message})
}

func writeDeadLetterError(c *gin.Context, err error) {
	if errors.Is(err, queue.ErrDeadLetterNotFound) {
		response.NotFound(c, "dead-letter entry not found")
		return
	}
	if errors.Is(err, queue.ErrOperationDeadLetterReplay) {
		response.ErrorWithBusinessCode(c, http.StatusConflict, "OPERATION_REPLAY_REQUIRED", "operation dead-letter must be replayed through its lifecycle")
		return
	}
	response.Error(c, err)
}

func collectQueueMetric(errors *[]string, metricName string, getter func() (int64, error)) int64 {
	value, err := getter()
	if err != nil {
		*errors = append(*errors, fmt.Sprintf("%s: %v", metricName, err))
		return 0
	}
	return value
}

// toInt32 将监控统计中的通用数值类型转换为 int32。
func toInt32(value interface{}) int32 {
	switch v := value.(type) {
	case int:
		return int32(v)
	case int32:
		return v
	case int64:
		return int32(v)
	case float64:
		return int32(v)
	default:
		return 0
	}
}
