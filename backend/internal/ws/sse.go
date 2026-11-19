package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"caiyun/internal/envutil"
	"caiyun/internal/models"
	"caiyun/internal/repository"
)

func (h *Hub) HandleSSE(w http.ResponseWriter, r *http.Request, userID uint, authorizations ...SessionAuthorization) {
	if !h.beginOperation() {
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	}
	defer h.operationWG.Done()
	authorization := connectionAuthorization(authorizations)
	if !authorization.valid(r.Context()) {
		http.Error(w, "session revoked", http.StatusUnauthorized)
		return
	}
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
		if !h.replaySSE(w, r, userID, repo, authorization, client) {
			return
		}
	}
	ping := time.NewTicker(sseHeartbeatInterval())
	defer ping.Stop()
	expired, recheck, stopSessionTimers := authorization.timers()
	defer stopSessionTimers()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-h.stopCh:
			return
		case <-expired:
			return
		case <-recheck:
			if !authorization.valid(r.Context()) {
				return
			}
		case <-ping.C:
			if _, err := fmt.Fprint(w, ": ping\n\n"); err != nil || !flushSSE(w) {
				return
			}
		case msg := <-client.send:
			if !authorization.valid(r.Context()) || !writeSSE(w, msg) {
				return
			}
		}
	}
}

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

// replaySSE deliberately does not ACK. A written HTTP response is not proof
// that the browser consumed it. Progress belongs to this connection's cursor,
// not to a global delivered flag. History and live overlap are deduplicated by
// message ID and ordered by durable sequence.
func (h *Hub) replaySSE(w http.ResponseWriter, r *http.Request, uid uint, repo *repository.WSMessageRepository, authorization *SessionAuthorization, client ...*SSEClient) bool {
	last := requestedSSELastEventID(r)
	for {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		var rows []*models.WebSocketMessage
		var err error
		if last > 0 {
			rows, err = repo.WithContext(ctx).GetMessagesAfterSequence(uid, last, 100)
		} else {
			rows, err = repo.WithContext(ctx).GetUndeliveredMessages(uid, 100)
		}
		cancel()
		if err != nil {
			return false
		}
		for _, row := range rows {
			if !authorization.valid(r.Context()) {
				return false
			}
			var data interface{}
			_ = json.Unmarshal([]byte(row.Data), &data)
			msg := Message{Type: row.Type, Data: data, UserID: row.UserID, MessageID: row.MessageID, Sequence: row.Sequence, CreatedAtMS: row.CreatedAt.UnixMilli()}
			if !writeSSE(w, msg) {
				return false
			}
			last = row.Sequence
		}
		if len(rows) < 100 {
			break
		}
	}
	if len(client) > 0 && client[0] != nil {
		for _, msg := range drainSSELive(client[0]) {
			if msg.Sequence <= last {
				continue
			}
			if !authorization.valid(r.Context()) || !writeSSE(w, msg) {
				return false
			}
			last = msg.Sequence
		}
	}
	return true
}

func drainSSELive(client *SSEClient) []Message {
	if client == nil {
		return nil
	}
	messages := make([]Message, 0)
	for {
		select {
		case msg := <-client.send:
			messages = append(messages, msg)
		default:
			sort.SliceStable(messages, func(i, j int) bool { return messages[i].Sequence < messages[j].Sequence })
			return messages
		}
	}
}

func requestedSSELastEventID(r *http.Request) uint64 {
	if r == nil {
		return 0
	}
	lastEventID := strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	if lastEventID == "" {
		lastEventID = r.URL.Query().Get("last_event_id")
	}
	last, _ := strconv.ParseUint(lastEventID, 10, 64)
	return last
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
