package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"time"

	"caiyun/internal/queue"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// queueListener 队列监听器
func (w *Worker) queueListener() {
	defer w.wg.Done()

	log.Println("队列监听器已启动")
	workerLimit := w.concurrency
	if workerLimit <= 0 {
		workerLimit = 1
	}
	sem := make(chan struct{}, workerLimit)
	for {
		select {
		case <-w.ctx.Done():
			log.Println("队列监听器已停止")
			return
		default:
		}

		// Acquire capacity before reading from Redis.  Holding an unstarted
		// delivery while the pool is saturated increases PEL idle time and made
		// the old select loop unable to service its maintenance tickers.
		select {
		case sem <- struct{}{}:
		case <-w.ctx.Done():
			log.Println("队列监听器已停止")
			return
		case <-time.After(100 * time.Millisecond):
			continue
		}

		// 从队列获取任务（阻塞5秒）
		message, err := w.taskQueue.Dequeue(5 * time.Second)
		if err != nil {
			<-sem
			if !errors.Is(err, queue.ErrQueueTimeout) {
				log.Printf("队列取任务失败: %v", err)
			}
			continue
		}

		w.wg.Add(1)
		go func(msg *queue.TaskMessage) {
			defer w.wg.Done()
			defer func() { <-sem }()
			defer w.recoverQueueTaskPanic(msg)
			w.processQueueTask(msg)
		}(message)
	}
}

// queueMaintenance owns queue housekeeping independently from task intake.
// It intentionally keeps the existing cadence so operational load remains
// predictable while avoiding semaphore-induced starvation.
func (w *Worker) queueMaintenance() {
	recoverTicker := time.NewTicker(time.Minute)
	delayedTicker := time.NewTicker(10 * time.Second)
	operationTicker := time.NewTicker(15 * time.Second)
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer recoverTicker.Stop()
	defer delayedTicker.Stop()
	defer operationTicker.Stop()
	defer heartbeatTicker.Stop()

	for {
		select {
		case <-w.ctx.Done():
			return
		case <-recoverTicker.C:
			w.recoverStaleQueueProcessing()
		case <-delayedTicker.C:
			w.promoteDueDelayedQueueTasks()
		case <-operationTicker.C:
			w.reconcileOperations()
		case <-heartbeatTicker.C:
			if w.metrics != nil {
				w.metrics.TouchWorkerHeartbeat(time.Now())
			}
		}
	}
}

func (w *Worker) recoverStaleQueueProcessing() {
	if w.taskQueue == nil {
		return
	}
	recovered, err := w.taskQueue.RecoverStaleProcessing(queue.DefaultVisibilityDelay)
	if err != nil {
		log.Printf("恢复超时处理中任务失败: %v", err)
	} else if recovered > 0 {
		log.Printf("已恢复 %d 个超时处理中任务", recovered)
	}
}

func (w *Worker) promoteDueDelayedQueueTasks() {
	if w.taskQueue == nil {
		return
	}
	promoted, err := w.taskQueue.PromoteDueDelayed(100)
	if err != nil {
		log.Printf("恢复到期延迟任务失败: %v", err)
	} else if promoted > 0 {
		log.Printf("已恢复 %d 个到期延迟任务", promoted)
	}
}

func (w *Worker) reconcileOperations() {
	if w.operationService == nil {
		return
	}
	recovered, err := w.operationService.RecoverStaleRunning(w.ctx, queue.DefaultVisibilityDelay, 100)
	if err != nil {
		log.Printf("stale running operation recovery failed: %v", err)
	} else if recovered > 0 {
		log.Printf("stale running operations requeued: count=%d", recovered)
	}
	dispatched, err := w.operationService.RedispatchQueued(w.ctx, 10*time.Second, 100)
	if err != nil {
		log.Printf("operation outbox reconciliation failed: %v", err)
	} else if dispatched > 0 {
		log.Printf("operation outbox redispatched: count=%d", dispatched)
	}
}

func (w *Worker) recoverQueueTaskPanic(message *queue.TaskMessage) {
	if recovered := recover(); recovered != nil {
		operationID := ""
		accountID := uint(0)
		taskType := ""
		if message != nil {
			operationID = message.OperationID
			accountID = message.AccountID
			taskType = message.TaskType
		}
		err := fmt.Errorf("worker task panic: %v", recovered)
		log.Printf("Worker 任务 panic，已进入失败处理: operation_id=%s account_id=%d task_type=%s err=%v\n%s",
			operationID, accountID, taskType, err, debug.Stack())
		w.handleQueueTaskFailure(message, err)
	}
}

func (w *Worker) recoverBackgroundPanic() {
	if recovered := recover(); recovered != nil {
		log.Printf("Worker 后台组件 panic，组件已退出: %v\n%s", recovered, debug.Stack())
	}
}

// processQueueTask 处理队列任务
func (w *Worker) processQueueTask(message *queue.TaskMessage) {
	if message == nil {
		log.Println("收到空队列消息，已忽略")
		return
	}
	ctx := w.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	_, span := otel.Tracer("caiyun/worker").Start(ctx, "worker.process_queue_task", trace.WithSpanKind(trace.SpanKindConsumer))
	span.SetAttributes(
		attribute.String("messaging.operation", "process"),
		attribute.String("caiyun.task_type", message.TaskType),
		attribute.Bool("caiyun.operation_message", strings.TrimSpace(message.OperationID) != ""),
	)
	defer span.End()
	log.Printf("从队列获取任务: 账号ID=%d, 任务类型=%s operation_id=%s", message.AccountID, message.TaskType, message.OperationID)
	if strings.TrimSpace(message.OperationID) != "" {
		w.processOperationTask(message)
		return
	}

	// 获取账号信息
	account, err := w.accountService.GetAccountByID(message.AccountID)
	if err != nil {
		log.Printf("获取账号失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, fmt.Sprintf("获取账号失败: %v", err))
		w.handleQueueTaskFailure(message, err)
		return
	}

	// 执行任务：队列消息可以指定 all/all_tasks 或具体任务类型。
	if err := w.ExecuteQueueAccountTask(account.ID, message.TaskType); err != nil {
		log.Printf("任务执行失败: %v", err)
		w.notifier.SendTaskFailure(message.AccountID, message.TaskType, err.Error())
		w.handleQueueTaskFailure(message, err)
	} else {
		log.Printf("任务执行成功: 账号ID=%d", message.AccountID)
		w.notifier.SendTaskSuccess(message.AccountID, message.TaskType, "任务执行成功")
		if err := w.taskQueue.Ack(message); err != nil {
			log.Printf("任务确认失败: account_id=%d task_type=%s err=%v", message.AccountID, message.TaskType, err)
		}
	}
}

func (w *Worker) handleQueueTaskFailure(message *queue.TaskMessage, cause error) {
	if message == nil {
		return
	}
	message.RetryCount++
	if message.RetryCount < queue.DefaultMaxAttempts {
		if err := w.taskQueue.Requeue(message); err != nil {
			log.Printf("任务重新入队失败: account_id=%d task_type=%s retry=%d err=%v", message.AccountID, message.TaskType, message.RetryCount, err)
		} else {
			log.Printf("任务已重新入队: account_id=%d task_type=%s retry=%d/%d", message.AccountID, message.TaskType, message.RetryCount, queue.DefaultMaxAttempts)
		}
		return
	}

	reason := ""
	if cause != nil {
		reason = cause.Error()
	}
	if err := w.taskQueue.DeadLetter(message, reason); err != nil {
		log.Printf("任务写入死信队列失败: account_id=%d task_type=%s err=%v", message.AccountID, message.TaskType, err)
		return
	}
	log.Printf("任务已移入死信队列: account_id=%d task_type=%s retries=%d reason=%s", message.AccountID, message.TaskType, message.RetryCount, reason)
}
