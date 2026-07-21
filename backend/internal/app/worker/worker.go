package worker

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"caiyun/internal/concurrency"
	"caiyun/internal/models"
	"caiyun/internal/monitor"
	"caiyun/internal/notification"
	"caiyun/internal/queue"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"
)

// Worker 任务执行器
type Worker struct {
	accountService   *services.AccountService
	taskService      *services.TaskService
	cloudService     *services.CloudService
	taskManager      *concurrency.TaskManager
	taskMonitor      *monitor.TaskMonitor
	jobScheduler     *scheduler.Scheduler
	retryManager     *monitor.RetryManager
	taskQueue        queue.ReliableTaskQueue
	operationService *services.OperationService
	exchangeService  *services.ExchangeService
	notifier         notification.Notifier
	metrics          *monitor.Metrics
	concurrency      int
	wg               sync.WaitGroup
	ctx              context.Context
	cancel           context.CancelFunc
}

func NewWorker(
	accountService *services.AccountService,
	taskService *services.TaskService,
	cloudService *services.CloudService,
	taskManager *concurrency.TaskManager,
	taskMonitor *monitor.TaskMonitor,
	jobScheduler *scheduler.Scheduler,
	retryManager *monitor.RetryManager,
	taskQueue queue.ReliableTaskQueue,
	notifier notification.Notifier,
	metricsCollector *monitor.Metrics,
	concurrency int,
) *Worker {
	return NewWorkerContext(context.Background(), accountService, taskService, cloudService, taskManager, taskMonitor, jobScheduler, retryManager, taskQueue, notifier, metricsCollector, concurrency)
}

// NewWorkerContext creates a worker whose lifecycle is bounded by parent.
// It lets the unified command own signal handling while retaining NewWorker for
// existing in-process callers.
func NewWorkerContext(
	parent context.Context,
	accountService *services.AccountService,
	taskService *services.TaskService,
	cloudService *services.CloudService,
	taskManager *concurrency.TaskManager,
	taskMonitor *monitor.TaskMonitor,
	jobScheduler *scheduler.Scheduler,
	retryManager *monitor.RetryManager,
	taskQueue queue.ReliableTaskQueue,
	notifier notification.Notifier,
	metricsCollector *monitor.Metrics,
	concurrency int,
) *Worker {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)

	return &Worker{
		accountService: accountService,
		taskService:    taskService,
		cloudService:   cloudService,
		taskManager:    taskManager,
		taskMonitor:    taskMonitor,
		jobScheduler:   jobScheduler,
		retryManager:   retryManager,
		taskQueue:      taskQueue,
		notifier:       notifier,
		metrics:        metricsCollector,
		concurrency:    concurrency,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// SetOperationServices installs the durable command state machine and the
// exchange executor after the compatibility constructor has built Worker.
func (w *Worker) SetOperationServices(operationService *services.OperationService, exchangeService *services.ExchangeService) {
	w.operationService = operationService
	w.exchangeService = exchangeService
}

// Start 启动Worker
func (w *Worker) Start() {
	// 启动任务管理器
	w.taskManager.Start()

	// 启动定时任务调度器
	w.jobScheduler.Start()

	// 启动任务监控器
	w.taskMonitor.StartCleanupJob(30*time.Minute, 24*time.Hour)

	if w.metrics != nil {
		w.metrics.SetWorkerState(true)
		w.metrics.TouchWorkerHeartbeat(time.Now())
	}

	// 启动队列监听器
	w.wg.Add(1)
	go w.queueListener()

	log.Printf("Worker已启动，并发数: %d", w.concurrency)
}

// startBackground registers a worker-owned background component. Stop cancels
// the worker context and waits for every registered component before shared
// services and the database are closed by Run.
func (w *Worker) startBackground(run func()) {
	if run == nil {
		return
	}
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		defer w.recoverBackgroundPanic()
		run()
	}()
}

// queueListener 队列监听器
func (w *Worker) queueListener() {
	defer w.wg.Done()

	log.Println("队列监听器已启动")
	workerLimit := w.concurrency
	if workerLimit <= 0 {
		workerLimit = 1
	}
	sem := make(chan struct{}, workerLimit)
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
			log.Println("队列监听器已停止")
			return
		case <-recoverTicker.C:
			recovered, err := w.taskQueue.RecoverStaleProcessing(queue.DefaultVisibilityDelay)
			if err != nil {
				log.Printf("恢复超时处理中任务失败: %v", err)
			} else if recovered > 0 {
				log.Printf("已恢复 %d 个超时处理中任务", recovered)
			}
		case <-delayedTicker.C:
			promoted, err := w.taskQueue.PromoteDueDelayed(100)
			if err != nil {
				log.Printf("恢复到期延迟任务失败: %v", err)
			} else if promoted > 0 {
				log.Printf("已恢复 %d 个到期延迟任务", promoted)
			}
		case <-operationTicker.C:
			if w.operationService != nil {
				dispatched, err := w.operationService.RedispatchQueued(w.ctx, 10*time.Second, 100)
				if err != nil {
					log.Printf("operation outbox reconciliation failed: %v", err)
				} else if dispatched > 0 {
					log.Printf("operation outbox redispatched: count=%d", dispatched)
				}
			}
		case <-heartbeatTicker.C:
			if w.metrics != nil {
				w.metrics.TouchWorkerHeartbeat(time.Now())
			}
		default:
			// 从队列获取任务（阻塞5秒）
			message, err := w.taskQueue.Dequeue(5 * time.Second)
			if err != nil {
				if !errors.Is(err, queue.ErrQueueTimeout) {
					log.Printf("队列取任务失败: %v", err)
				}
				continue
			}

			select {
			case sem <- struct{}{}:
			case <-w.ctx.Done():
				if err := w.taskQueue.Requeue(message); err != nil {
					log.Printf("Worker 退出时任务重新入队失败: account_id=%d task_type=%s err=%v", message.AccountID, message.TaskType, err)
				}
				return
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

// processOperationTask executes one durable API command. A queue frame is ACKed
// only after the database state reaches a terminal status. State-update errors
// deliberately leave the frame pending for visibility-timeout recovery.
func (w *Worker) processOperationTask(message *queue.TaskMessage) {
	if message == nil || strings.TrimSpace(message.OperationID) == "" {
		return
	}
	if w.operationService == nil {
		log.Printf("operation service is not configured; leave message pending: operation_id=%s", message.OperationID)
		return
	}

	operation, claimed, err := w.operationService.TryStart(w.ctx, message.OperationID, queue.DefaultVisibilityDelay)
	if err != nil {
		if errors.Is(err, services.ErrOperationNotFound) {
			if dlqErr := w.taskQueue.DeadLetter(message, "operation not found"); dlqErr != nil {
				log.Printf("dead-letter orphan operation failed: operation_id=%s err=%v", message.OperationID, dlqErr)
			}
			return
		}
		log.Printf("claim operation failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
		return
	}
	if !claimed {
		// This frame is a duplicate outbox delivery or a canceled/terminal command.
		// The active owner has its own queue frame, so this duplicate can be ACKed.
		if err := w.taskQueue.Ack(message); err != nil {
			log.Printf("ack duplicate operation failed: operation_id=%s err=%v", message.OperationID, err)
		}
		return
	}

	if err := w.executeOperation(operation); err != nil {
		w.handleOperationFailure(message, err)
		return
	}
	if err := w.operationService.MarkSucceeded(w.ctx, operation.ID); err != nil {
		log.Printf("mark operation succeeded failed; leave message pending: operation_id=%s err=%v", operation.ID, err)
		return
	}
	if err := w.taskQueue.Ack(message); err != nil {
		log.Printf("ack completed operation failed: operation_id=%s err=%v", operation.ID, err)
	}
}

func (w *Worker) handleOperationFailure(message *queue.TaskMessage, cause error) {
	log.Printf("operation execution failed: operation_id=%s retry=%d err=%v", message.OperationID, message.RetryCount, cause)
	if isTerminalOperationError(cause) {
		if err := w.operationService.MarkFailed(w.ctx, message.OperationID, cause); err != nil {
			log.Printf("mark terminal operation failed failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
			return
		}
		if err := w.taskQueue.DeadLetter(message, "terminal operation error"); err != nil {
			log.Printf("dead-letter terminal operation failed: operation_id=%s err=%v", message.OperationID, err)
		}
		return
	}
	message.RetryCount++
	if message.RetryCount < queue.DefaultMaxAttempts {
		if err := w.operationService.MarkRetryQueued(w.ctx, message.OperationID, cause); err != nil {
			log.Printf("mark operation queued failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
			return
		}
		if err := w.taskQueue.Requeue(message); err != nil {
			// The durable row is queued, so the outbox reconciler can republish it.
			log.Printf("requeue operation failed; outbox will reconcile: operation_id=%s err=%v", message.OperationID, err)
		}
		return
	}
	if err := w.operationService.MarkFailed(w.ctx, message.OperationID, cause); err != nil {
		log.Printf("mark operation failed failed; leave message pending: operation_id=%s err=%v", message.OperationID, err)
		return
	}
	if err := w.taskQueue.DeadLetter(message, "operation execution failed"); err != nil {
		log.Printf("dead-letter failed operation failed: operation_id=%s err=%v", message.OperationID, err)
	}
}

// isTerminalOperationError identifies validation and business-rule failures
// that cannot succeed on retry. Retrying them only creates duplicate outbox
// deliveries and hides the actionable error from the operation record.
func isTerminalOperationError(err error) bool {
	return errors.Is(err, services.ErrExchangeTaskConflict) ||
		errors.Is(err, services.ErrExchangeTaskAlreadyExists) ||
		errors.Is(err, services.ErrExchangeMonthlyLimitReached) ||
		errors.Is(err, services.ErrExchangeExecutionFailed) ||
		errors.Is(err, services.ErrExchangeBatchPartialFailure) ||
		errors.Is(err, services.ErrExchangeBatchFailed) ||
		errors.Is(err, services.ErrExchangeInvalidInput) ||
		errors.Is(err, services.ErrExchangeCloudAccountMissing) ||
		errors.Is(err, services.ErrExchangeRuleNotFound) ||
		errors.Is(err, services.ErrExchangeTaskNotFound) ||
		errors.Is(err, services.ErrExchangeProductNotFound) ||
		errors.Is(err, services.ErrExchangePermissionDenied) ||
		errors.Is(err, services.ErrExchangeAccountDisabled) ||
		errors.Is(err, services.ErrExchangeCredentialsMissing) ||
		errors.Is(err, services.ErrExchangeProductInactive)
}

func (w *Worker) executeOperation(operation *models.Operation) error {
	if operation == nil {
		return errors.New("operation is nil")
	}
	if err := w.ctx.Err(); err != nil {
		return err
	}
	switch operation.OperationType {
	case models.OperationTypeAccountTask:
		var payload services.AccountTaskOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if payload.AccountID == 0 || strings.TrimSpace(payload.TaskType) == "" {
			return errors.New("invalid account task payload")
		}
		if err := w.requireAccountOwner(payload.AccountID, operation.UserID); err != nil {
			return err
		}
		return w.ExecuteQueueAccountTask(payload.AccountID, payload.TaskType)

	case models.OperationTypeAccountTaskBatch:
		var payload services.AccountTaskBatchOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if len(payload.AccountIDs) == 0 || len(payload.AccountIDs) > 1000 {
			return errors.New("invalid account batch payload")
		}
		var failures []error
		for _, accountID := range payload.AccountIDs {
			if err := w.ctx.Err(); err != nil {
				return err
			}
			if err := w.requireAccountOwner(accountID, operation.UserID); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := w.ExecuteQueueAccountTask(accountID, "all_tasks"); err != nil {
				failures = append(failures, fmt.Errorf("account %d: %w", accountID, err))
			}
		}
		return errors.Join(failures...)

	case models.OperationTypeExchangeTask:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		var payload services.ExchangeTaskOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if payload.TaskID == 0 {
			return errors.New("invalid exchange task payload")
		}
		return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, payload.TaskID, operation.UserID)

	case models.OperationTypeExchangeTaskBatch:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		var payload services.ExchangeTaskBatchOperationPayload
		if err := decodeOperationPayload(operation, &payload); err != nil {
			return err
		}
		if len(payload.TaskIDs) == 0 || len(payload.TaskIDs) > 1000 {
			return errors.New("invalid exchange task batch payload")
		}
		results := w.exchangeService.BatchExecuteExchangeTasksContext(w.ctx, payload.TaskIDs, operation.UserID)
		succeeded := 0
		failed := 0
		failures := make([]string, 0, len(results))
		for _, result := range results {
			if result.Success {
				succeeded++
				continue
			}
			failed++
			message := strings.TrimSpace(result.Message)
			if message == "" {
				message = "抢兑失败，请查看执行结果"
			}
			if len(failures) < 8 {
				failures = append(failures, fmt.Sprintf("任务%d：%s", result.TaskID, message))
			}
		}
		if failed > 0 {
			summary := fmt.Sprintf("批量抢兑结果：成功 %d，失败 %d，共 %d", succeeded, failed, len(results))
			if len(failures) > 0 {
				summary += "；失败项：" + strings.Join(failures, "；")
			}
			if succeeded > 0 {
				return fmt.Errorf("%w: %s", services.ErrExchangeBatchPartialFailure, summary)
			}
			return fmt.Errorf("%w: %s", services.ErrExchangeBatchFailed, summary)
		}
		return nil

	case models.OperationTypeExchangeImmediate:
		return w.executeImmediateExchangeOperation(operation)

	case models.OperationTypeExchangeMonthly:
		if w.exchangeService == nil {
			return errors.New("exchange service is not configured")
		}
		if err := w.ctx.Err(); err != nil {
			return err
		}
		return w.exchangeService.ExecuteMonthlyExchangeContext(w.ctx)

	default:
		return fmt.Errorf("unsupported operation type %q", operation.OperationType)
	}
}

func (w *Worker) executeImmediateExchangeOperation(operation *models.Operation) error {
	if w.exchangeService == nil {
		return errors.New("exchange service is not configured")
	}
	if operation.ResourceID > 0 {
		return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, operation.ResourceID, operation.UserID)
	}
	var payload services.ImmediateExchangeOperationPayload
	if err := decodeOperationPayload(operation, &payload); err != nil {
		return err
	}
	if payload.ProductID == 0 || (payload.ExchangeRuleID == 0 && payload.AccountID == 0) {
		return errors.New("invalid immediate exchange payload")
	}
	options := services.ExchangeTaskCreateOptions{SourceOperationID: operation.ID, TaskType: string(models.ExchangeTaskFixed), MaxAttempts: 1, RestockCycle: "once", CalendarPolicy: "all"}
	var task *models.ExchangeTask
	var err error
	if payload.ExchangeRuleID > 0 {
		task, err = w.exchangeService.CreateExchangeTaskWithOptionsContext(w.ctx, operation.UserID, payload.ExchangeRuleID, payload.ProductID, options)
	} else {
		task, err = w.exchangeService.CreateExchangeTaskByAccountIDWithOptionsContext(w.ctx, operation.UserID, payload.AccountID, payload.ProductID, options)
	}
	if err != nil {
		return err
	}
	if err := w.operationService.SetResourceID(w.ctx, operation.ID, task.ID); err != nil {
		return fmt.Errorf("link immediate exchange task: %w", err)
	}
	return w.exchangeService.ExecuteExchangeTaskContext(w.ctx, task.ID, operation.UserID)
}

func (w *Worker) requireAccountOwner(accountID, userID uint) error {
	if accountID == 0 || userID == 0 {
		return errors.New("invalid account ownership input")
	}
	account, err := w.accountService.GetAccountByID(accountID)
	if err != nil {
		return err
	}
	if account.UserID != userID {
		return errors.New("account does not belong to operation user")
	}
	return nil
}

func decodeOperationPayload(operation *models.Operation, target interface{}) error {
	if operation == nil || strings.TrimSpace(operation.Payload) == "" {
		return errors.New("operation payload is empty")
	}
	if err := json.Unmarshal([]byte(operation.Payload), target); err != nil {
		return fmt.Errorf("decode operation payload: %w", err)
	}
	return nil
}

// Stop 停止Worker
func (w *Worker) Stop() {
	log.Println("正在停止Worker...")
	if w.metrics != nil {
		w.metrics.SetWorkerState(false)
	}

	// 先阻止队列监听器、监控服务和任务执行继续接收新工作。
	w.cancel()

	// cron job 可能正在向 TaskManager 提交任务并等待批次完成，因此必须
	// 先停止调度并等待已启动的 job，再关闭 TaskManager。顺序反过来会让
	// job 永久等待已被取消的 worker，或在已关闭队列上提交任务。
	w.jobScheduler.Stop()
	w.taskManager.Stop()

	// 等待队列任务与监控 API 完整退出后再停止 TaskMonitor，避免后台
	// goroutine 在其清理循环关闭后继续读写监控状态。
	w.wg.Wait()
	w.taskMonitor.Stop()
	log.Println("Worker已停止")
}

// ExecuteSingleAccount 执行单个账号的任务
func (w *Worker) ExecuteSingleAccount(accountID uint) error {
	return w.ExecuteQueueAccountTask(accountID, "all_tasks")
}

// ExecuteQueueAccountTask 执行队列指定的账号任务，支持具体任务类型。
func (w *Worker) ExecuteQueueAccountTask(accountID uint, taskType string) error {
	taskType = strings.TrimSpace(taskType)
	if taskType == "" {
		taskType = "all_tasks"
	}

	// 获取账号详情
	account, err := w.accountService.GetAccountByID(accountID)
	if err != nil {
		return err
	}

	// 检查账号是否激活
	if !account.IsActive {
		return fmt.Errorf("账号 %d 未激活", accountID)
	}

	// 使用重试管理器执行任务
	err = w.retryManager.ExecuteWithRetry(
		accountID,
		taskType,
		func() error {
			// 刷新Token（如果需要）
			if err := w.accountService.RefreshTokenIfNeeded(account); err != nil {
				return err
			}

			// 执行队列指定任务；taskType=all/all_tasks 时执行全部批量任务。
			_, err := w.taskService.ExecuteSelectedTaskForAccountContext(w.ctx, account, taskType)
			return err
		},
		func(progress float64, message string) {
			w.taskMonitor.UpdateTaskProgress(accountID, taskType, progress, message)
		},
	)

	return err
}

// RunAllAccounts 执行所有激活账号的任务
func (w *Worker) RunAllAccounts() error {
	const batchSize = 200
	totalSubmitted := 0

	for offset := 0; ; offset += batchSize {
		accounts, err := w.accountService.ListActiveAccounts(offset, batchSize)
		if err != nil {
			return err
		}
		if len(accounts) == 0 {
			break
		}

		log.Printf("开始执行第 %d 批激活账号任务，账号数: %d", offset/batchSize+1, len(accounts))
		if err := w.taskManager.SubmitBatchTasks(accounts); err != nil {
			return err
		}
		w.taskManager.WaitForCompletion()
		totalSubmitted += len(accounts)

		if len(accounts) < batchSize {
			break
		}
	}

	if totalSubmitted == 0 {
		log.Println("没有激活账号需要执行")
		return nil
	}

	log.Printf("所有账号任务执行完成，累计账号数: %d", totalSubmitted)

	// 获取任务统计
	stats := w.taskManager.GetStatus()
	log.Printf("任务执行统计: %+v", stats)

	// 计算每日统计
	if err := w.cloudService.CalculateDailyStats(); err != nil {
		log.Printf("计算每日统计失败: %v", err)
	}

	return nil
}
