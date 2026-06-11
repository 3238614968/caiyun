package handlers

import (
	"fmt"

	"caiyun/internal/monitor"
	"caiyun/internal/queue"
	"caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

// QueueStatusHandler 负责返回任务队列与任务监控统计信息。
type QueueStatusHandler struct {
	taskQueue queue.ReliableTaskQueue
}

// NewQueueStatusHandler 创建队列状态处理器。
func NewQueueStatusHandler(taskQueue queue.ReliableTaskQueue) *QueueStatusHandler {
	return &QueueStatusHandler{taskQueue: taskQueue}
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

	if tm := monitor.GetGlobalTaskMonitor(); tm != nil {
		stats := tm.GetStats()
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
