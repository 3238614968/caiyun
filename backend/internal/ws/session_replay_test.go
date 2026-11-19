//go:build cgo

package ws

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestSSERevokedSessionCannotReplayStoredMessages(t *testing.T) {
	hub, _, repo := newSSEDeliveryHub(t)
	if _, err := repo.PersistMessageEnvelope(42, "notification", "private", "private-replay", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var validations atomic.Int32
	auth := SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool {
		// Handshake passes, then the session is revoked before replay writes.
		return validations.Add(1) == 1
	}}
	writer := newSSETestWriter(false)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		defer close(done)
		hub.HandleSSE(writer, httptest.NewRequest(http.MethodGet, "/events", nil).WithContext(ctx), 42, auth)
	}()
	awaitSessionExit(t, done)
	select {
	case <-writer.eventWritten:
		t.Fatal("stored private message was replayed after revocation")
	default:
	}
	if validations.Load() < 2 {
		t.Fatal("stored replay did not revalidate the authenticated session")
	}
}

func TestWebSocketRevokedSessionCannotReplayStoredMessages(t *testing.T) {
	hub, _, repo := newSSEDeliveryHub(t)
	go hub.run()
	t.Cleanup(hub.Stop)
	if _, err := repo.PersistMessageEnvelope(42, "notification", "private", "private-replay", time.Now().Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var validations atomic.Int32
	auth := SessionAuthorization{ExpiresAt: time.Now().Add(time.Minute), Validate: func(context.Context) bool {
		return validations.Add(1) == 1
	}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hub.HandleWebSocket(w, r, 42, auth)
	}))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(server.URL, "http"), http.Header{"Origin": []string{server.URL}})
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	requireWebSocketSessionClosed(t, conn)
}
