package worker

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"caiyun/internal/concurrency"
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
	// Queue maintenance must not share the dequeue loop.  A full execution
	// semaphore used to block that loop before it could run PEL recovery,
	// delayed promotion, or durable operation reconciliation.
	w.startBackground(w.queueMaintenance)

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
