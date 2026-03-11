package utils

import (
	"context"
	"sync"
)

// WorkerPool 工作池结构
type WorkerPool struct {
	workers   int           // 工作协程数量
	taskQueue chan func()   // 任务队列
	wg        sync.WaitGroup
	ctx       context.Context
	cancel    context.CancelFunc
}

// NewWorkerPool 创建工作池
// workers: 工作协程数量
// queueSize: 任务队列大小
func NewWorkerPool(workers, queueSize int) *WorkerPool {
	ctx, cancel := context.WithCancel(context.Background())
	return &WorkerPool{
		workers:   workers,
		taskQueue: make(chan func(), queueSize),
		ctx:       ctx,
		cancel:    cancel,
	}
}

// Start 启动工作池
func (p *WorkerPool) Start() {
	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.worker(i)
	}
}

// worker 工作协程
func (p *WorkerPool) worker(id int) {
	defer p.wg.Done()
	for {
		select {
		case task, ok := <-p.taskQueue:
			if !ok {
				return // 队列关闭，退出
			}
			task() // 执行任务
		case <-p.ctx.Done():
			return // 上下文取消，退出
		}
	}
}

// Submit 提交任务到工作池
// 返回是否成功提交
func (p *WorkerPool) Submit(task func()) bool {
	select {
	case p.taskQueue <- task:
		return true
	case <-p.ctx.Done():
		return false
	default:
		return false // 队列已满
	}
}

// SubmitAndWait 提交任务并等待执行完成
func (p *WorkerPool) SubmitAndWait(task func()) bool {
	done := make(chan struct{})
	wrappedTask := func() {
		task()
		close(done)
	}

	if !p.Submit(wrappedTask) {
		return false
	}

	<-done
	return true
}

// Stop 停止工作池
func (p *WorkerPool) Stop() {
	p.cancel()
	close(p.taskQueue)
	p.wg.Wait()
}

// StopGracefully 优雅停止工作池（等待所有任务完成）
func (p *WorkerPool) StopGracefully() {
	// 等待所有任务完成
	for len(p.taskQueue) > 0 {
		// 简单等待，实际可以使用更复杂的同步机制
	}
	p.Stop()
}

// TaskQueueSize 获取当前任务队列大小
func (p *WorkerPool) TaskQueueSize() int {
	return len(p.taskQueue)
}

// TaskQueueCapacity 获取任务队列容量
func (p *WorkerPool) TaskQueueCapacity() int {
	return cap(p.taskQueue)
}

// ConcurrentExecutor 并发执行器（简化版工作池）
type ConcurrentExecutor struct {
	concurrency int
	semaphore   chan struct{}
	wg          sync.WaitGroup
}

// NewConcurrentExecutor 创建并发执行器
// concurrency: 最大并发数
func NewConcurrentExecutor(concurrency int) *ConcurrentExecutor {
	if concurrency <= 0 {
		concurrency = 10
	}
	return &ConcurrentExecutor{
		concurrency: concurrency,
		semaphore:   make(chan struct{}, concurrency),
	}
}

// Execute 执行任务
func (e *ConcurrentExecutor) Execute(task func()) {
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		e.semaphore <- struct{}{}        // 获取信号量
		defer func() { <-e.semaphore }() // 释放信号量
		task()
	}()
}

// Submit 提交任务（Execute的别名，用于兼容）
func (e *ConcurrentExecutor) Submit(task func()) {
	e.Execute(task)
}

// Wait 等待所有任务完成
func (e *ConcurrentExecutor) Wait() {
	e.wg.Wait()
}

// ExecuteWithResults 执行任务并收集结果
type TaskResult struct {
	Index  int
	Result interface{}
	Error  error
}

// ExecuteBatchWithResults 批量执行任务并收集结果
// tasks: 任务列表
// concurrency: 并发数
func ExecuteBatchWithResults(tasks []func() (interface{}, error), concurrency int) []TaskResult {
	if concurrency <= 0 {
		concurrency = 10
	}

	results := make([]TaskResult, len(tasks))
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, task := range tasks {
		wg.Add(1)
		go func(index int, t func() (interface{}, error)) {
			defer wg.Done()

			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			result, err := t()

			mu.Lock()
			results[index] = TaskResult{
				Index:  index,
				Result: result,
				Error:  err,
			}
			mu.Unlock()
		}(i, task)
	}

	wg.Wait()
	return results
}
