package utils

import "sync"

// ConcurrentExecutor 并发执行器
type ConcurrentExecutor struct {
	concurrency int
	semaphore   chan struct{}
	wg          sync.WaitGroup
}

// NewConcurrentExecutor 创建并发执行器
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
		e.semaphore <- struct{}{}
		defer func() { <-e.semaphore }()
		task()
	}()
}

// Wait 等待所有任务完成
func (e *ConcurrentExecutor) Wait() {
	e.wg.Wait()
}
