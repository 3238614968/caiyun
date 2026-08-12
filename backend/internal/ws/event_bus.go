package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"

	"caiyun/internal/cache"
)

const defaultEventChannel = "caiyun:ws:events"

// EventTransport is intentionally small so the Hub can be tested with an
// in-memory cross-node transport while production uses Redis Pub/Sub.
type EventTransport interface {
	Publish(context.Context, string, string) error
	Subscribe(context.Context, string) (*cache.PubSubSubscription, error)
}

type redisEventBus struct {
	transport EventTransport
	channel   string
	nodeID    string
	hub       *Hub
	ctx       context.Context
	cancel    context.CancelFunc
	sub       *cache.PubSubSubscription
	wg        sync.WaitGroup
	stopOnce  sync.Once
}

func newRedisEventBus(parent context.Context, transport EventTransport, channel, nodeID string, hub *Hub) (*redisEventBus, error) {
	if transport == nil {
		return nil, fmt.Errorf("WebSocket event transport is nil")
	}
	if parent == nil {
		parent = context.Background()
	}
	if strings.TrimSpace(channel) == "" {
		channel = defaultEventChannel
	}
	ctx, cancel := context.WithCancel(parent)
	sub, err := transport.Subscribe(ctx, channel)
	if err != nil {
		cancel()
		return nil, err
	}
	bus := &redisEventBus{transport: transport, channel: channel, nodeID: nodeID, hub: hub, ctx: ctx, cancel: cancel, sub: sub}
	bus.wg.Add(1)
	go bus.consume()
	return bus, nil
}

func (b *redisEventBus) consume() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case payload, ok := <-b.sub.Messages:
			if !ok {
				return
			}
			var message Message
			if err := json.Unmarshal([]byte(payload), &message); err != nil {
				log.Printf("[WS] 忽略无效跨节点事件: %v", err)
				continue
			}
			b.hub.acceptEnvelope(message)
		}
	}
}

func (b *redisEventBus) publish(message Message) error {
	payload, err := json.Marshal(message)
	if err != nil {
		return err
	}
	return b.transport.Publish(b.ctx, b.channel, string(payload))
}

func (b *redisEventBus) Stop() {
	if b == nil {
		return
	}
	b.stopOnce.Do(func() {
		b.cancel()
		if b.sub != nil {
			_ = b.sub.Close()
		}
		b.wg.Wait()
	})
}
