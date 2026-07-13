package bootstrap

import (
	"context"
	"fmt"

	"caiyun/internal/queue"
)

// CheckReadiness validates every serving dependency under one caller-provided
// deadline. Queue APIs are legacy context-free, so their bounded Redis calls run
// in a buffered goroutine while the probe still respects the outer deadline.
func CheckReadiness(ctx context.Context, core *Core, taskQueue queue.ReliableTaskQueue) error {
	if core == nil || core.DB == nil || core.Redis == nil {
		return fmt.Errorf("核心依赖未初始化")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sqlDB, err := core.DB.DB()
	if err != nil {
		return fmt.Errorf("获取数据库连接失败: %w", err)
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return fmt.Errorf("数据库不可用: %w", err)
	}
	if err := core.Redis.Ping(ctx); err != nil {
		return fmt.Errorf("Redis 不可用: %w", err)
	}
	if taskQueue == nil {
		return fmt.Errorf("任务队列未初始化")
	}

	queueResult := make(chan error, 1)
	go func() {
		_, queueErr := taskQueue.GetQueueLength()
		queueResult <- queueErr
	}()
	select {
	case err := <-queueResult:
		if err != nil {
			return fmt.Errorf("任务队列不可用: %w", err)
		}
		return nil
	case <-ctx.Done():
		return fmt.Errorf("任务队列健康检查超时: %w", ctx.Err())
	}
}
