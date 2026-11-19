package ws

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

type sessionSSEWriter struct {
	header http.Header
	mu     sync.Mutex
	body   bytes.Buffer
	events chan struct{}
}

func (w *sessionSSEWriter) Header() http.Header { return w.header }
func (w *sessionSSEWriter) WriteHeader(int)     {}
func (w *sessionSSEWriter) Flush()              {}
func (w *sessionSSEWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if bytes.HasPrefix(data, []byte("data:")) {
		w.events <- struct{}{}
	}
	return w.body.Write(data)
}

func serveSessionSSE(t *testing.T, hub *Hub, authorization SessionAuthorization) (*sessionSSEWriter, <-chan struct{}) {
	t.Helper()
	w := &sessionSSEWriter{header: make(http.Header), events: make(chan struct{}, 8)}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.HandleSSE(w, httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), 42, authorization)
	}()
	t.Cleanup(func() { cancel(); awaitSessionExit(t, done) })
	deadline := time.After(time.Second)
	for {
		hub.mu.RLock()
		registered := len(hub.sseClients[42]) == 1
		hub.mu.RUnlock()
		if registered {
			return w, done
		}
		select {
		case <-deadline:
			t.Fatal("SSE did not register")
		case <-time.After(time.Millisecond):
		}
	}
}

func awaitSessionExit(t *testing.T, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("revoked/expired realtime connection remained open")
	}
}

func TestSSESessionRevocationBlocksNextPrivateEvent(t *testing.T) {
	hub := startTestHub(t)
	var active atomic.Bool
	active.Store(true)
	w, done := serveSessionSSE(t, hub, SessionAuthorization{
		ExpiresAt: time.Now().Add(time.Minute),
		Validate:  func(context.Context) bool { return active.Load() },
		// Exercise the per-write check, before the periodic check runs.
		recheckInterval: time.Hour,
	})
	hub.SendToUser(42, Message{Type: "before_revocation", Data: "allowed"})
	select {
	case <-w.events:
	case <-time.After(time.Second):
		t.Fatal("active SSE session did not receive its event")
	}
	active.Store(false)
	hub.SendToUser(42, Message{Type: "after_revocation", Data: "private-phone"})
	awaitSessionExit(t, done)
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.Contains(w.body.String(), "private-phone") {
		t.Fatal("revoked SSE session received private data")
	}
}

func TestSSEIdleSessionExpiresOrIsRevoked(t *testing.T) {
	for _, reason := range []string{"expired", "revoked"} {
		t.Run(reason, func(t *testing.T) {
			hub := startTestHub(t)
			var active atomic.Bool
			active.Store(true)
			auth := SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool { return active.Load() }, recheckInterval: 10 * time.Millisecond}
			if reason == "expired" {
				auth.ExpiresAt = time.Now().Add(300 * time.Millisecond)
			}
			_, done := serveSessionSSE(t, hub, auth)
			if reason == "revoked" {
				active.Store(false)
			}
			awaitSessionExit(t, done)
		})
	}
}

func dialSessionWebSocket(t *testing.T, hub *Hub, authorization SessionAuthorization) *websocket.Conn {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 42, authorization)
	}))
	t.Cleanup(server.Close)
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Origin": []string{server.URL}})
	if err != nil {
		t.Fatalf("dial WebSocket: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	deadline := time.After(time.Second)
	for {
		hub.mu.RLock()
		registered := len(hub.clients[42]) == 1
		hub.mu.RUnlock()
		if registered {
			return conn
		}
		select {
		case <-deadline:
			t.Fatal("WebSocket did not register")
		case <-time.After(time.Millisecond):
		}
	}
}

func requireWebSocketSessionClosed(t *testing.T, conn *websocket.Conn) {
	t.Helper()
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, payload, err := conn.ReadMessage()
	if err == nil {
		t.Fatalf("invalid session received data: %s", payload)
	}
	if timeout, ok := err.(interface{ Timeout() bool }); ok && timeout.Timeout() {
		t.Fatal("invalid WebSocket session was not disconnected")
	}
	if !websocket.IsCloseError(err, 4001) {
		t.Fatalf("session close error = %v, want auth close code 4001", err)
	}
}

func TestWebSocketSessionRevocationBlocksNextPrivateEvent(t *testing.T) {
	hub := startTestHub(t)
	var active atomic.Bool
	active.Store(true)
	conn := dialSessionWebSocket(t, hub, SessionAuthorization{
		ExpiresAt:       time.Now().Add(time.Minute),
		Validate:        func(context.Context) bool { return active.Load() },
		recheckInterval: time.Hour,
	})
	hub.SendToUser(42, Message{Type: "before_revocation", Data: "allowed"})
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	_, payload, err := conn.ReadMessage()
	if err != nil || !bytes.Contains(payload, []byte("before_revocation")) {
		t.Fatalf("active WebSocket session event = %s, error = %v", payload, err)
	}
	active.Store(false)
	hub.SendToUser(42, Message{Type: "after_revocation", Data: "private-phone"})
	requireWebSocketSessionClosed(t, conn)
}

func TestWebSocketIdleSessionExpiresOrIsRevoked(t *testing.T) {
	for _, reason := range []string{"expired", "revoked"} {
		t.Run(reason, func(t *testing.T) {
			hub := startTestHub(t)
			var active atomic.Bool
			active.Store(true)
			auth := SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool { return active.Load() }, recheckInterval: 10 * time.Millisecond}
			if reason == "expired" {
				auth.ExpiresAt = time.Now().Add(300 * time.Millisecond)
			}
			conn := dialSessionWebSocket(t, hub, auth)
			if reason == "revoked" {
				active.Store(false)
			}
			requireWebSocketSessionClosed(t, conn)
		})
	}
}

func TestRealtimeRejectsInvalidSessionBeforeOpening(t *testing.T) {
	for _, transport := range []string{"sse", "websocket"} {
		t.Run(transport, func(t *testing.T) {
			hub := startTestHub(t)
			writer := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/", nil)
			auth := SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool { return false }}
			if transport == "sse" {
				hub.HandleSSE(writer, request, 42, auth)
			} else {
				hub.HandleWebSocket(writer, request, 42, auth)
			}
			if writer.Code != http.StatusUnauthorized {
				t.Fatalf("status = %d, want 401", writer.Code)
			}
		})
	}
}
