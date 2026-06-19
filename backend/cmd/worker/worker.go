package main

import (
	"context"
	"errors"
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
	accountService *services.AccountService
	taskService    *services.TaskService
	cloudService   *services.CloudService
	taskManager    *concurrency.TaskManager
	taskMonitor    *monitor.TaskMonitor
	jobScheduler   *scheduler.Scheduler
	retryManager   *monitor.RetryManager
	taskQueue      queue.ReliableTaskQueue
	notifier       notification.Notifier
	concurrency    int
	wg             sync.WaitGroup
	ctx            context.Context
	cancel         context.CancelFunc
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
	concurrency int,
) *Worker {
	ctx, cancel := context.WithCancel(context.Background())

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
		concurrency:    concurrency,
		ctx:            ctx,
		cancel:         cancel,
	}
}

// Start 启动Worker
func (w *Worker) Start() {
	// 启动任务管理器
	w.taskManager.Start()

	// 启动定时任务调度器
	w.jobScheduler.Start()

	// 启动任务监控器
	w.taskMonitor.StartCleanupJob(30*time.Minute, 24*time.Hour)

	// 启动队列监听器
	w.wg.Add(1)
	go w.queueListener()

	log.Printf("Worker已启动，并发数: %d", w.concurrency)
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
	defer recoverTicker.Stop()
	defer delayedTicker.Stop()

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
				w.processQueueTask(msg)
			}(message)
		}
	}
}

// processQueueTask 处理队列任务
func (w *Worker) processQueueTask(message *queue.TaskMessage) {
	if message == nil {
		log.Println("收到空队列消息，已忽略")
		return
	}
	log.Printf("从队列获取任务: 账号ID=%d, 任务类型=%s", message.AccountID, message.TaskType)

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

// Stop 停止Worker
func (w *Worker) Stop() {
	log.Println("正在停止Worker...")
	w.cancel()

	// 停止所有组件
	w.taskManager.Stop()
	w.jobScheduler.Stop()
	w.taskMonitor.Stop()

	w.wg.Wait()
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
