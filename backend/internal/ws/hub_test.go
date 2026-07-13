package ws

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"caiyun/internal/cache"
)

type fakeEventTransport struct {
	mu          sync.Mutex
	subscribers map[string][]chan string
	sequences   map[string]uint64
}

func newFakeEventTransport() *fakeEventTransport {
	return &fakeEventTransport{subscribers: make(map[string][]chan string), sequences: make(map[string]uint64)}
}

func (f *fakeEventTransport) Publish(_ context.Context, channel, payload string) error {
	f.mu.Lock()
	targets := append([]chan string(nil), f.subscribers[channel]...)
	f.mu.Unlock()
	for _, target := range targets {
		target <- payload
	}
	return nil
}

func (f *fakeEventTransport) NextSequence(_ context.Context, key string) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sequences[key]++
	return f.sequences[key], nil
}

func (f *fakeEventTransport) Subscribe(_ context.Context, channel string) (*cache.PubSubSubscription, error) {
	messages := make(chan string, 16)
	f.mu.Lock()
	f.subscribers[channel] = append(f.subscribers[channel], messages)
	f.mu.Unlock()
	return &cache.PubSubSubscription{Messages: messages}, nil
}

func startTestHub(t *testing.T) *Hub {
	t.Helper()
	hub := newHub()
	go hub.run()
	t.Cleanup(hub.Stop)
	return hub
}

func TestEventBusDeliversAcrossHubInstancesOnce(t *testing.T) {
	transport := newFakeEventTransport()
	first, second := startTestHub(t), startTestHub(t)
	if err := first.ConfigureEventBus(context.Background(), transport, "test:events", "api-a"); err != nil {
		t.Fatal(err)
	}
	if err := second.ConfigureEventBus(context.Background(), transport, "test:events", "api-b"); err != nil {
		t.Fatal(err)
	}

	client := &Client{hub: second, userID: 42, send: make(chan []byte, 4), pending: make(map[string]*pendingDelivery)}
	second.mu.Lock()
	second.clients[42] = map[*Client]bool{client: true}
	second.mu.Unlock()
	defer func() { second.mu.Lock(); delete(second.clients, 42); second.mu.Unlock() }()

	first.SendToUser(42, Message{Type: "task_complete", Data: map[string]interface{}{"ok": true}})
	select {
	case payload := <-client.send:
		var got Message
		if err := json.Unmarshal(payload, &got); err != nil {
			t.Fatal(err)
		}
		if got.UserID != 42 || got.Sequence != 1 || got.MessageID == "" {
			t.Fatalf("unexpected envelope: %+v", got)
		}
		if got.PublisherID != "api-a" {
			t.Fatalf("publisher=%q", got.PublisherID)
		}
	case <-time.After(time.Second):
		t.Fatal("cross-hub event was not delivered")
	}
	select {
	case duplicate := <-client.send:
		t.Fatalf("duplicate delivery: %s", duplicate)
	case <-time.After(50 * time.Millisecond):
	}
}

func TestPendingDeliveryRetriesAreBoundedAndAckRemovesEntry(t *testing.T) {
	client := &Client{send: make(chan []byte, 2), pending: make(map[string]*pendingDelivery)}
	now := time.Now()
	msg := Message{Type: "notification", MessageID: "m-1", ExpiresAtMS: now.Add(time.Minute).UnixMilli()}
	if !client.enqueue(msg, []byte(`{"type":"notification"}`)) {
		t.Fatal("enqueue failed")
	}
	client.pendingMu.Lock()
	client.pending["m-1"].nextRetry = now.Add(-time.Second)
	client.pendingMu.Unlock()
	if got := client.retryDue(now); len(got) != 1 {
		t.Fatalf("retry count=%d want 1", len(got))
	}
	client.ack("m-1")
	if got := client.retryDue(now.Add(time.Hour)); len(got) != 0 {
		t.Fatalf("retry after ack=%d", len(got))
	}
	client.pendingMu.Lock()
	defer client.pendingMu.Unlock()
	if len(client.pending) != 0 {
		t.Fatalf("pending=%d", len(client.pending))
	}
}
