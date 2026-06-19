package concurrency

import (
	"context"
	"fmt"
	"log"
	"sync"
	"sync/atomic"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/services"
)

// TaskResult 表示任务执行结果。
type TaskResult struct {
	AccountID uint
	TaskType  string
	Status    string
	Message   string
	Duration  time.Duration
	Error     error
}

// TaskManager 管理批量账号任务的并发执行与结果统计。
type TaskManager struct {
	taskService *services.TaskService
	concurrency int

	taskQueue   chan *models.Account
	resultQueue chan *TaskResult

	activeWorkers int32
	activeTasks   int32

	// workerWG 仅跟踪常驻 worker 协程与结果处理协程的生命周期。
	workerWG sync.WaitGroup
	resultWG sync.WaitGroup
	// taskWG 跟踪“已提交账号任务”的完成情况，供 WaitForCompletion 使用。
	taskWG sync.WaitGroup

	ctx    context.Context
	cancel context.CancelFunc

	mu          sync.RWMutex
	taskMetrics map[string]*TaskMetrics
	stopOnce    sync.Once
}

// TaskMetrics 记录某类任务的执行指标。
type TaskMetrics struct {
	TotalTasks    int64
	SuccessTasks  int64
	FailedTasks   int64
	TotalDuration time.Duration
	LastExecuted  time.Time
}

// NewTaskManager 创建任务管理器。
func NewTaskManager(taskService *services.TaskService, concurrency int) *TaskManager {
	ctx, cancel := context.WithCancel(context.Background())

	if concurrency <= 0 {
		concurrency = 1
	}

	return &TaskManager{
		taskService: taskService,
		concurrency: concurrency,
		taskQueue:   make(chan *models.Account, concurrency*20),
		resultQueue: make(chan *TaskResult, concurrency*50),
		ctx:         ctx,
		cancel:      cancel,
		taskMetrics: make(map[string]*TaskMetrics),
	}
}

// Start 启动任务管理器。
func (tm *TaskManager) Start() {
	// 启动账号任务 worker。
	for i := 0; i < tm.concurrency; i++ {
		tm.workerWG.Add(1)
		go tm.worker(i)
	}

	// 启动结果处理协程。
	tm.resultWG.Add(1)
	go tm.resultProcessor()

	log.Printf("TaskManager 已启动，并发数: %d", tm.concurrency)
}

// Stop 停止任务管理器。
func (tm *TaskManager) Stop() {
	tm.stopOnce.Do(func() {
		log.Println("正在停止 TaskManager...")

		// 先通知所有协程停止拉取新任务。
		tm.cancel()
		close(tm.taskQueue)

		// 等待 worker 全部退出，确保不会再向结果队列写入。
		tm.workerWG.Wait()

		// worker 已退出后再关闭结果队列，避免发送到已关闭通道。
		close(tm.resultQueue)
		tm.resultWG.Wait()

		log.Println("TaskManager 已停止")
	})
}

// SubmitTask 提交单个账号任务。
func (tm *TaskManager) SubmitTask(account *models.Account) (err error) {
	if account == nil {
		return fmt.Errorf("账号不能为空")
	}

	// 在任务进入队列前先计入批次等待计数，避免 worker 提前完成导致计数不平衡。
	tm.taskWG.Add(1)
	defer func() {
		if r := recover(); r != nil {
			// 可能在停止过程中向已关闭 taskQueue 发送。
			tm.taskWG.Done()
			if tm.ctx.Err() != nil {
				err = tm.ctx.Err()
				return
			}
			err = fmt.Errorf("提交任务失败: %v", r)
		}
	}()

	select {
	case <-tm.ctx.Done():
		tm.taskWG.Done()
		return tm.ctx.Err()
	case tm.taskQueue <- account:
		return nil
	default:
		tm.taskWG.Done()
		return context.DeadlineExceeded
	}
}

// SubmitBatchTasks 批量提交账号任务。
func (tm *TaskManager) SubmitBatchTasks(accounts []*models.Account) error {
	for _, account := range accounts {
		if err := tm.SubmitTask(account); err != nil {
			return err
		}
	}
	return nil
}

// worker 执行账号任务并写入结果队列。
func (tm *TaskManager) worker(id int) {
	defer tm.workerWG.Done()
	atomic.AddInt32(&tm.activeWorkers, 1)
	defer atomic.AddInt32(&tm.activeWorkers, -1)

	for {
		select {
		case <-tm.ctx.Done():
			return
		case account, ok := <-tm.taskQueue:
			if !ok {
				return
			}
			if account == nil {
				tm.taskWG.Done()
				continue
			}

			func() {
				atomic.AddInt32(&tm.activeTasks, 1)
				defer atomic.AddInt32(&tm.activeTasks, -1)
				defer tm.taskWG.Done()
				defer func() {
					if r := recover(); r != nil {
						tm.recordResult(account.ID, "all", "failed", fmt.Sprintf("任务执行 panic: %v", r), 0, fmt.Errorf("panic: %v", r))
					}
				}()
				tm.processAccount(account, id)
			}()
		}
	}
}

// processAccount 处理单个账号的所有任务。
func (tm *TaskManager) processAccount(account *models.Account, workerID int) {
	startTime := time.Now()

	if !account.IsActive {
		tm.recordResult(account.ID, "check", "skipped", "账号未激活", time.Since(startTime), nil)
		return
	}

	log.Printf("[Worker %d] 开始执行账号 %d 的任务", workerID, account.ID)

	taskResults, err := tm.taskService.ExecuteTaskForAccount(account)
	if err != nil {
		tm.recordResult(account.ID, "all", "failed", err.Error(), time.Since(startTime), err)
		return
	}

	for _, result := range taskResults {
		tm.recordResult(account.ID, result.TaskType, result.Status, result.Message, time.Duration(result.ExecutionTime)*time.Millisecond, nil)
	}

	log.Printf("[Worker %d] 账号 %d 任务执行完成", workerID, account.ID)
}

// recordResult 将执行结果写入结果队列。
func (tm *TaskManager) recordResult(accountID uint, taskType, status, message string, duration time.Duration, err error) {
	result := &TaskResult{
		AccountID: accountID,
		TaskType:  taskType,
		Status:    status,
		Message:   message,
		Duration:  duration,
		Error:     err,
	}

	select {
	case tm.resultQueue <- result:
	case <-tm.ctx.Done():
		return
	}
}

// resultProcessor 异步处理结果统计。
func (tm *TaskManager) resultProcessor() {
	defer tm.resultWG.Done()

	for result := range tm.resultQueue {
		tm.processResult(result)
	}
}

// processResult 更新统计信息并输出日志。
func (tm *TaskManager) processResult(result *TaskResult) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	metrics, exists := tm.taskMetrics[result.TaskType]
	if !exists {
		metrics = &TaskMetrics{}
		tm.taskMetrics[result.TaskType] = metrics
	}

	metrics.TotalTasks++
	metrics.TotalDuration += result.Duration
	metrics.LastExecuted = time.Now()

	if result.Status == "success" {
		metrics.SuccessTasks++
	} else {
		metrics.FailedTasks++
	}

	if result.Error != nil {
		log.Printf("任务执行失败 - 账号: %d, 类型: %s, 错误: %v", result.AccountID, result.TaskType, result.Error)
		return
	}

	log.Printf("任务执行完成 - 账号: %d, 类型: %s, 状态: %s, 耗时: %v",
		result.AccountID, result.TaskType, result.Status, result.Duration)
}

// GetMetrics 获取任务指标快照。
func (tm *TaskManager) GetMetrics() map[string]*TaskMetrics {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	metrics := make(map[string]*TaskMetrics)
	for k, v := range tm.taskMetrics {
		metrics[k] = &TaskMetrics{
			TotalTasks:    v.TotalTasks,
			SuccessTasks:  v.SuccessTasks,
			FailedTasks:   v.FailedTasks,
			TotalDuration: v.TotalDuration,
			LastExecuted:  v.LastExecuted,
		}
	}
	return metrics
}

// GetActiveWorkers 获取活跃 worker 数量。
func (tm *TaskManager) GetActiveWorkers() int32 {
	return atomic.LoadInt32(&tm.activeWorkers)
}

// GetActiveTasks 获取正在执行账号任务的 worker 数量。
func (tm *TaskManager) GetActiveTasks() int32 {
	return atomic.LoadInt32(&tm.activeTasks)
}

// GetQueueSize 获取当前队列长度。
func (tm *TaskManager) GetQueueSize() int {
	return len(tm.taskQueue)
}

// GetPendingTasks 获取待处理任务数量（队列中 + 正在执行的账号任务）。
func (tm *TaskManager) GetPendingTasks() int {
	return len(tm.taskQueue) + int(tm.GetActiveTasks())
}

// WaitForCompletion 等待当前已提交批次任务执行完成。
// 注意：该方法只等待“已提交任务”，不会等待常驻 worker 协程退出。
func (tm *TaskManager) WaitForCompletion() {
	tm.taskWG.Wait()
}

// GetStatus 获取任务管理器状态。
func (tm *TaskManager) GetStatus() map[string]interface{} {
	return map[string]interface{}{
		"active_workers": tm.GetActiveWorkers(),
		"active_tasks":   tm.GetActiveTasks(),
		"queue_size":     tm.GetQueueSize(),
		"pending_tasks":  tm.GetPendingTasks(),
		"task_metrics":   tm.GetMetrics(),
		"is_running":     tm.ctx.Err() == nil,
	}
}
