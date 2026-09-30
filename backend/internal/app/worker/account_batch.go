package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// Independent accounts use a bounded pool; each account's task sequence stays
// ordered. ExecuteQueueAccountTask enforces the worker-wide account limit.
func runAccountBatch(ctx context.Context, ids []uint, concurrency int, run func(uint) error) error {
	jobs := make(chan uint)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []error
	for i := 0; i < min(max(1, concurrency), len(ids)); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range jobs {
				if err := runAccountBatchItem(id, run); err != nil {
					mu.Lock()
					failures = append(failures, fmt.Errorf("account %d: %w", id, err))
					mu.Unlock()
				}
			}
		}()
	}
	seen := make(map[uint]bool)
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		select {
		case jobs <- id:
		case <-ctx.Done():
			close(jobs)
			wg.Wait()
			return ctx.Err()
		}
	}
	close(jobs)
	wg.Wait()
	return errors.Join(failures...)
}

func runAccountBatchItem(id uint, run func(uint) error) (err error) {
	defer func() {
		if value := recover(); value != nil {
			err = fmt.Errorf("account task panic: %v", value)
		}
	}()
	return run(id)
}
