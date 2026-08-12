package worker

import (
	"context"
	"sync"
	"testing"
	"time"

	"caiyun/internal/queue"
)

// listenerQueueStub embeds the full interface because this regression only
// needs to observe whether intake reaches Dequeue before Worker cancellation.
type listenerQueueStub struct {
	queue.ReliableTaskQueue
	once     sync.Once
	dequeued chan struct{}
}

func (q *listenerQueueStub) Dequeue(time.Duration) (*queue.TaskMessage, error) {
	q.once.Do(func() { close(q.dequeued) })
	return nil, queue.ErrQueueTimeout
}

func TestQueueListenerDoesNotBlockBeforeDequeue(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	q := &listenerQueueStub{dequeued: make(chan struct{})}
	w := &Worker{ctx: ctx, taskQueue: q, concurrency: 1}
	w.wg.Add(1)
	go w.queueListener()

	select {
	case <-q.dequeued:
		// The listener acquired capacity and reached Redis intake.  This
		// regresses the accidental one-case select that blocked until shutdown.
	case <-time.After(time.Second):
		cancel()
		w.wg.Wait()
		t.Fatal("queue listener did not reach Dequeue before cancellation")
	}
	cancel()
	w.wg.Wait()
}
