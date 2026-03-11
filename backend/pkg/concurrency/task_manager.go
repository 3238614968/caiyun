package concurrency

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// TaskFunc 任务函数类型
type TaskFunc func(ctx context.Context) error

// TaskResult 任务结果
type TaskResult struct {
	Error     error
	Duration  time.Duration
	PanicInfo interface{}
}

// TaskManager 任务管理器（支持并发控制和优雅关闭）
type TaskManager struct {
	mu          sync.RWMutex
	maxWorkers  int
	semaphore   chan struct{}
	wg          sync.WaitGroup
	ctx         context.Context
	cancel      context.CancelFunc
	taskCount   int64
	activeCount int64
}

// NewTaskManager 创建任务管理器
func NewTaskManager(maxWorkers int) *TaskManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &TaskManager{
		maxWorkers: maxWorkers,
		semaphore:  make(chan struct{}, maxWorkers),
		ctx:        ctx,
		cancel:     cancel,
	}
}

// Submit 提交任务（非阻塞）
func (tm *TaskManager) Submit(task TaskFunc) {
	tm.wg.Add(1)
	go func() {
		defer tm.wg.Done()

		// 获取信号量（会阻塞直到有空闲位置）
		tm.semaphore <- struct{}{}
		defer func() { <-tm.semaphore }()

		// 更新活跃计数
		tm.mu.Lock()
		tm.activeCount++
		tm.taskCount++
		tm.mu.Unlock()
		defer func() {
			tm.mu.Lock()
			tm.activeCount--
			tm.mu.Unlock()
		}()

		// 执行任务并捕获 panic
		startTime := time.Now()
		result := &TaskResult{}
		
		func() {
			defer func() {
				if r := recover(); r != nil {
					result.PanicInfo = r
					result.Error = fmt.Errorf("panic: %v", r)
				}
			}()
			
			result.Error = task(tm.ctx)
		}()
		
		result.Duration = time.Since(startTime)
		
		// 这里可以添加日志或监控
		if result.Error != nil {
			// log.Printf("任务执行失败：%v", result.Error)
		}
	}()
}

// SubmitAndWait 提交任务并等待完成（用于测试或小批量任务）
func (tm *TaskManager) SubmitAndWait(tasks ...TaskFunc) []error {
	var mu sync.Mutex
	errors := make([]error, 0, len(tasks))
	var wg sync.WaitGroup

	for _, task := range tasks {
		wg.Add(1)
		tm.Submit(func(ctx context.Context) error {
			defer wg.Done()
			err := task(ctx)
			if err != nil {
				mu.Lock()
				errors = append(errors, err)
				mu.Unlock()
			}
			return err
		})
	}

	wg.Wait()
	return errors
}

// Wait 等待所有任务完成
func (tm *TaskManager) Wait() {
	tm.wg.Wait()
}

// Shutdown 优雅关闭（等待正在运行的任务完成）
func (tm *TaskManager) Shutdown(timeout time.Duration) error {
	// 停止接收新任务
	tm.cancel()

	// 等待所有任务完成
	done := make(chan struct{})
	go func() {
		tm.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-time.After(timeout):
		return fmt.Errorf("关闭超时，仍有任务在运行")
	}
}

// ShutdownNow 立即关闭（不等待任务完成）
func (tm *TaskManager) ShutdownNow() {
	tm.cancel()
}

// GetStats 获取统计信息
func (tm *TaskManager) GetStats() (taskCount, activeCount int64) {
	tm.mu.RLock()
	defer tm.mu.RUnlock()
	return tm.taskCount, tm.activeCount
}

// GetMaxWorkers 获取最大工作协程数
func (tm *TaskManager) GetMaxWorkers() int {
	return tm.maxWorkers
}

// WorkerPool 工作池（更精细的控制）
type WorkerPool struct {
	mu        sync.Mutex
	workers   int
	taskQueue chan TaskFunc
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
	started   bool
}

// NewWorkerPool 创建工作池
func NewWorkerPool(workers int, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan TaskFunc, queueSize),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动工作池
func (wp *WorkerPool) Start() {
	wp.mu.Lock()
	defer wp.mu.Unlock()

	if wp.started {
		return
	}
	wp.started = true

	for i := 0; i < wp.workers; i++ {
		wp.wg.Add(1)
		go func(id int) {
			defer wp.wg.Done()
			for {
				select {
				case task, ok := <-wp.taskQueue:
					if !ok {
						return
					}
					// 执行任务并捕获 panic
					func() {
						defer func() {
							if r := recover(); r != nil {
								// log.Printf("Worker %d panic: %v", id, r)
							}
						}()
						task(wp.ctx)
					}()
				case <-wp.ctx.Done():
					return
				}
			}
		}(i)
	}
}

// Submit 提交任务到工作池
func (wp *WorkerPool) Submit(task TaskFunc) error {
	select {
	case wp.taskQueue <- task:
		return nil
	case <-wp.ctx.Done():
		return fmt.Errorf("工作池已关闭")
	default:
		return fmt.Errorf("任务队列已满")
	}
}

// Stop 停止工作池
func (wp *WorkerPool) Stop() {
	wp.cancel()
	close(wp.taskQueue)
	wp.wg.Wait()
}

// BatchProcessor 批处理器
type BatchProcessor struct {
	mu          sync.Mutex
	buffer      []interface{}
	bufferSize  int
	processFunc func([]interface{}) error
	timer       *time.Timer
	timeout     time.Duration
}

// NewBatchProcessor 创建批处理器
func NewBatchProcessor(bufferSize int, timeout time.Duration, processFunc func([]interface{}) error) *BatchProcessor {
	return &BatchProcessor{
		buffer:      make([]interface{}, 0, bufferSize),
		bufferSize:  bufferSize,
		processFunc: processFunc,
		timeout:     timeout,
	}
}

// Add 添加数据到批次
func (bp *BatchProcessor) Add(data interface{}) error {
	bp.mu.Lock()
	defer bp.mu.Unlock()

	bp.buffer = append(bp.buffer, data)

	// 如果缓冲区满了，立即处理
	if len(bp.buffer) >= bp.bufferSize {
		return bp.flush()
	}

	// 启动或重置定时器
	if bp.timer == nil {
		bp.timer = time.AfterFunc(bp.timeout, func() {
			bp.mu.Lock()
			defer bp.mu.Unlock()
			if len(bp.buffer) > 0 {
				bp.flush()
			}
		})
	} else {
		bp.timer.Reset(bp.timeout)
	}

	return nil
}

// flush 处理批次数据
func (bp *BatchProcessor) flush() error {
	if len(bp.buffer) == 0 {
		return nil
	}

	data := make([]interface{}, len(bp.buffer))
	copy(data, bp.buffer)
	bp.buffer = bp.buffer[:0]

	if bp.timer != nil {
		bp.timer.Stop()
		bp.timer = nil
	}

	return bp.processFunc(data)
}

// Flush 手动刷新缓冲区
func (bp *BatchProcessor) Flush() error {
	bp.mu.Lock()
	defer bp.mu.Unlock()
	return bp.flush()
}

// RateLimiter 限流器（令牌桶算法改进版）
type RateLimiter struct {
	mu         sync.Mutex
	rate       float64 // 每秒生成的令牌数
	bucket     float64 // 当前令牌数
	maxBucket  float64 // 桶容量
	lastUpdate time.Time
}

// NewRateLimiter 创建限流器
func NewRateLimiter(rate float64, burst int) *RateLimiter {
	return &RateLimiter{
		rate:       rate,
		bucket:     float64(burst),
		maxBucket:  float64(burst),
		lastUpdate: time.Now(),
	}
}

// Allow 检查是否允许请求
func (rl *RateLimiter) Allow() bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastUpdate).Seconds()
	rl.lastUpdate = now

	// 添加令牌
	rl.bucket += elapsed * rl.rate
	if rl.bucket > rl.maxBucket {
		rl.bucket = rl.maxBucket
	}

	// 消耗令牌
	if rl.bucket >= 1 {
		rl.bucket--
		return true
	}
	return false
}

// Wait 等待直到可以获得令牌
func (rl *RateLimiter) Wait(ctx context.Context) error {
	for {
		if rl.Allow() {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(10 * time.Millisecond):
			// 短暂等待后重试
		}
	}
}
