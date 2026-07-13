package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/envutil"
	"caiyun/internal/models"
	"caiyun/internal/repository"
)

func (h *Hub) HandleSSE(w http.ResponseWriter, r *http.Request, userID uint) {
	if !h.beginOperation() {
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	}
	defer h.operationWG.Done()
	client := &SSEClient{userID: userID, send: make(chan Message, 256)}
	h.registerSSE(client)
	defer h.unregisterSSE(client)
	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 3000\n\n")
	if !flushSSE(w) {
		return
	}
	if repo := h.getSSERepo(); repo != nil {
		h.replaySSE(w, r, userID, repo)
	}
	ping := time.NewTicker(sseHeartbeatInterval())
	defer ping.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.stopCh:
			return
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || !flushSSE(w) {
				return
			}
		case msg := <-client.send:
			if !writeSSE(w, msg) {
				return
			}
			if repo := h.getSSERepo(); repo != nil && msg.UserID != 0 && msg.MessageID != "" {
				_, _ = repo.MarkAsDeliveredByMessageID(userID, msg.MessageID)
			}
		}
	}
}

// sseHeartbeatInterval keeps bytes flowing through CDNs that close an idle
// chunked response before the normal Nginx upstream timeout. It can be tuned
// without a rebuild, but must stay below the CDN idle timeout.
func sseHeartbeatInterval() time.Duration {
	interval := envutil.Duration("SSE_HEARTBEAT_INTERVAL", 3*time.Second)
	if interval < time.Second {
		return time.Second
	}
	return interval
}

func (h *Hub) getSSERepo() *repository.WSMessageRepository {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.wsRepo
}
func (h *Hub) registerSSE(c *SSEClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.sseClients[c.userID] == nil {
		h.sseClients[c.userID] = map[*SSEClient]bool{}
	}
	h.sseClients[c.userID][c] = true
}
func (h *Hub) unregisterSSE(c *SSEClient) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if m := h.sseClients[c.userID]; m != nil {
		delete(m, c)
		if len(m) == 0 {
			delete(h.sseClients, c.userID)
		}
	}
}
func (h *Hub) replaySSE(w http.ResponseWriter, r *http.Request, uid uint, repo *repository.WSMessageRepository) {
	last, _ := strconv.ParseUint(strings.TrimSpace(r.Header.Get("Last-Event-ID")), 10, 64)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	var rows []*models.WebSocketMessage
	var err error
	if last > 0 {
		rows, err = repo.WithContext(ctx).GetMessagesAfterSequence(uid, last, 100)
	} else {
		rows, err = repo.WithContext(ctx).GetUndeliveredMessages(uid, 100)
	}
	if err != nil {
		return
	}
	for _, row := range rows {
		var data interface{}
		_ = json.Unmarshal([]byte(row.Data), &data)
		msg := Message{Type: row.Type, Data: data, UserID: row.UserID, MessageID: row.MessageID, Sequence: row.Sequence, CreatedAtMS: row.CreatedAt.UnixMilli()}
		if !writeSSE(w, msg) {
			return
		}
		_, _ = repo.MarkAsDeliveredByMessageID(uid, row.MessageID)
	}
}
func writeSSE(w http.ResponseWriter, msg Message) bool {
	raw, err := json.Marshal(msg)
	if err != nil {
		return false
	}
	if msg.Sequence > 0 {
		fmt.Fprintf(w, "id: %d\n", msg.Sequence)
	}
	if _, err = fmt.Fprintf(w, "data: %s\n\n", raw); err != nil {
		return false
	}
	return flushSSE(w)
}
func flushSSE(w http.ResponseWriter) bool {
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
		return true
	}
	return false
}
