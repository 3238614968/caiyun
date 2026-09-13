//go:build cgo

package ws

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/repository"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// 本文件锁定 SSE 的投递语义：写入 HTTP 响应不等于送达，SSE 永远不做 ACK；
// 持久化消息保持 pending，直到 WebSocket 显式 ACK 或 TTL 清理。

func TestSSELIVEKeepsMessagePendingAcrossConnections(t *testing.T) {
	h, db, _ := newSSEDeliveryHub(t)
	const userID uint = 42

	first := newSSETestWriter(false)
	second := newSSETestWriter(false)
	firstDone, cancelFirst := serveSSEForTest(t, h, first, userID)
	secondDone, cancelSecond := serveSSEForTest(t, h, second, userID)
	defer func() {
		cancelFirst()
		cancelSecond()
		waitSSEHandlerExit(t, firstDone)
		waitSSEHandlerExit(t, secondDone)
	}()
	waitSSEClients(t, h, userID, 2)

	h.SendToUser(userID, Message{Type: "notification", Data: map[string]any{"message": "hello"}})
	waitSSEData(t, first.eventWritten)
	waitSSEData(t, second.eventWritten)

	var stored models.WebSocketMessage
	if err := db.Where("user_id = ?", userID).First(&stored).Error; err != nil {
		t.Fatalf("load persisted message: %v", err)
	}
	assertSSEPending(t, stored)
}

func TestSSEReplayKeepsMessagePendingAcrossConnections(t *testing.T) {
	h, db, repo := newSSEDeliveryHub(t)
	const userID uint = 9
	const messageID = "sse-replay-pending"
	if _, err := repo.PersistMessageEnvelope(userID, "notification", map[string]any{"message": "hello"}, messageID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("persist pending message: %v", err)
	}

	first := newSSETestWriter(false)
	second := newSSETestWriter(false)
	firstDone, cancelFirst := serveSSEForTest(t, h, first, userID)
	secondDone, cancelSecond := serveSSEForTest(t, h, second, userID)
	defer func() {
		cancelFirst()
		cancelSecond()
		waitSSEHandlerExit(t, firstDone)
		waitSSEHandlerExit(t, secondDone)
	}()
	waitSSEData(t, first.eventWritten)
	waitSSEData(t, second.eventWritten)

	var stored models.WebSocketMessage
	if err := db.Where("user_id = ? AND message_id = ?", userID, messageID).First(&stored).Error; err != nil {
		t.Fatalf("load replayed message: %v", err)
	}
	assertSSEPending(t, stored)
}

func TestSSEReplayWriteFailureKeepsMessagePending(t *testing.T) {
	h, db, repo := newSSEDeliveryHub(t)
	const userID uint = 7
	const messageID = "sse-replay-write-failure"
	if _, err := repo.PersistMessageEnvelope(userID, "notification", map[string]any{"message": "hello"}, messageID, time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("persist pending message: %v", err)
	}

	writer := newSSETestWriter(true)
	done, cancel := serveSSEForTest(t, h, writer, userID)
	waitSSEData(t, writer.eventWritten)
	cancel()
	waitSSEHandlerExit(t, done)

	var stored models.WebSocketMessage
	if err := db.Where("user_id = ? AND message_id = ?", userID, messageID).First(&stored).Error; err != nil {
		t.Fatalf("load failed-delivery message: %v", err)
	}
	assertSSEPending(t, stored)
}

func newSSEDeliveryHub(t *testing.T) (*Hub, *gorm.DB, *repository.WSMessageRepository) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sqlite DB: %v", err)
	}
	sqlDB.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&models.WebSocketMessage{}, &models.WebSocketSequence{}); err != nil {
		t.Fatalf("migrate sqlite: %v", err)
	}
	repo := repository.NewWSMessageRepository(db)
	h := newHub()
	h.SetWSMessageRepository(repo)
	return h, db, repo
}

func assertSSEPending(t *testing.T, message models.WebSocketMessage) {
	t.Helper()
	if message.IsDelivered || message.DeliveredAt != nil || message.AckedAt != nil {
		t.Fatalf("SSE must not acknowledge persisted message: %+v", message)
	}
}

func serveSSEForTest(t *testing.T, h *Hub, writer http.ResponseWriter, userID uint) (<-chan struct{}, context.CancelFunc) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/sse", nil).WithContext(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		h.HandleSSE(writer, req, userID)
	}()
	return done, cancel
}

func waitSSEClients(t *testing.T, h *Hub, userID uint, want int) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		h.mu.RLock()
		got := len(h.sseClients[userID])
		h.mu.RUnlock()
		if got == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("SSE client count did not reach %d", want)
}

func waitSSEData(t *testing.T, eventWritten <-chan struct{}) {
	t.Helper()
	select {
	case <-eventWritten:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE event write")
	}
}

func waitSSEHandlerExit(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("SSE handler did not exit")
	}
}

type sseTestWriter struct {
	mu           sync.Mutex
	header       http.Header
	failEvent    bool
	eventWritten chan struct{}
}

func newSSETestWriter(failEvent bool) *sseTestWriter {
	return &sseTestWriter{
		header:       make(http.Header),
		failEvent:    failEvent,
		eventWritten: make(chan struct{}, 1),
	}
}

func (w *sseTestWriter) Header() http.Header { return w.header }

func (w *sseTestWriter) WriteHeader(int) {}

func (w *sseTestWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if bytes.Contains(p, []byte("data:")) {
		select {
		case w.eventWritten <- struct{}{}:
		default:
		}
		if w.failEvent {
			return 0, errors.New("SSE client disconnected")
		}
	}
	return len(p), nil
}

func (w *sseTestWriter) Flush() {}
