package cache

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// PubSubSubscription is a lifecycle-aware Redis Pub/Sub subscription. Messages
// is closed after Close, Redis shutdown, or parent context cancellation.
type PubSubSubscription struct {
	Messages <-chan string
	close    func() error
}

func (s *PubSubSubscription) Close() error {
	if s == nil || s.close == nil {
		return nil
	}
	return s.close()
}

// Publish sends a message while honoring the caller deadline. A bounded Redis
// operation timeout is added when the caller has no deadline.
func (r *RedisCache) Publish(parent context.Context, channel, payload string) error {
	if r == nil || r.client == nil {
		return fmt.Errorf("redis client is nil")
	}
	if channel == "" {
		return fmt.Errorf("redis pubsub channel is empty")
	}
	ctx, cancel := r.contextForCaller(parent)
	defer cancel()
	return r.client.Publish(ctx, channel, payload).Err()
}

// NextSequence atomically allocates a monotonically increasing sequence for a
// logical key (for example, one key per WebSocket user).
func (r *RedisCache) NextSequence(parent context.Context, key string) (uint64, error) {
	if r == nil || r.client == nil {
		return 0, fmt.Errorf("redis client is nil")
	}
	if key == "" {
		return 0, fmt.Errorf("redis sequence key is empty")
	}
	ctx, cancel := r.contextForCaller(parent)
	defer cancel()
	value, err := r.client.Incr(ctx, key).Uint64()
	if err != nil {
		return 0, err
	}
	return value, nil
}

// Subscribe creates a bounded, cancellation-aware subscription. Calling Close
// is idempotent and waits for the forwarding goroutine to finish.
func (r *RedisCache) Subscribe(parent context.Context, channel string) (*PubSubSubscription, error) {
	if r == nil || r.client == nil {
		return nil, fmt.Errorf("redis client is nil")
	}
	if channel == "" {
		return nil, fmt.Errorf("redis pubsub channel is empty")
	}
	if parent == nil {
		parent = context.Background()
	}
	base := r.ctx
	if base == nil {
		base = context.Background()
	}
	ctx, cancel := context.WithCancel(base)
	pubsub := r.client.Subscribe(ctx, channel)

	receiveCtx, receiveCancel := context.WithTimeout(ctx, r.operationTimeoutOrDefault())
	_, err := pubsub.Receive(receiveCtx)
	receiveCancel()
	if err != nil {
		cancel()
		_ = pubsub.Close()
		return nil, fmt.Errorf("subscribe %s: %w", channel, err)
	}

	out := make(chan string, 128)
	done := make(chan struct{})
	var closeOnce sync.Once
	closeFn := func() error {
		var closeErr error
		closeOnce.Do(func() {
			cancel()
			closeErr = pubsub.Close()
			<-done
		})
		return closeErr
	}

	go func() {
		defer close(done)
		defer close(out)
		messages := pubsub.Channel()
		for {
			select {
			case <-ctx.Done():
				return
			case <-parent.Done():
				cancel()
				return
			case message, ok := <-messages:
				if !ok {
					return
				}
				select {
				case out <- message.Payload:
				case <-ctx.Done():
					return
				case <-parent.Done():
					cancel()
					return
				}
			}
		}
	}()

	return &PubSubSubscription{Messages: out, close: closeFn}, nil
}

func (r *RedisCache) contextForCaller(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}
	if _, ok := parent.Deadline(); ok {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, r.operationTimeoutOrDefault())
}

func (r *RedisCache) operationTimeoutOrDefault() time.Duration {
	if r != nil && r.operationTimeout > 0 {
		return r.operationTimeout
	}
	return 5 * time.Second
}
