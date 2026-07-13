package ws

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"caiyun/internal/envutil"
	"caiyun/internal/repository"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize: 1024, WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if sameOriginHost(origin, r.Host) {
			return true
		}
		for _, allowed := range strings.Split(envutil.String("ALLOWED_ORIGINS", ""), ",") {
			if strings.TrimSpace(allowed) == origin {
				return true
			}
		}
		return false
	},
}

func sameOriginHost(origin, requestHost string) bool {
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" || requestHost == "" {
		return false
	}
	originHost, originPort := strings.ToLower(parsed.Hostname()), parsed.Port()
	if originPort == "" {
		originPort = defaultPort(parsed.Scheme)
	}
	host, port, err := net.SplitHostPort(requestHost)
	if err != nil {
		host, port = requestHost, ""
	}
	host = strings.ToLower(strings.Trim(host, "[]"))
	if originHost != host {
		return false
	}
	if port == "" {
		return originPort == defaultPort(parsed.Scheme)
	}
	return originPort == port
}

func defaultPort(scheme string) string {
	switch strings.ToLower(scheme) {
	case "http", "ws":
		return "80"
	case "https", "wss":
		return "443"
	default:
		return ""
	}
}

// Message is the versioned, at-least-once WebSocket delivery envelope.
type Message struct {
	Type        string      `json:"type"`
	Data        interface{} `json:"data"`
	UserID      uint        `json:"user_id,omitempty"`
	MessageID   string      `json:"message_id,omitempty"`
	Sequence    uint64      `json:"sequence,omitempty"`
	CreatedAtMS int64       `json:"created_at,omitempty"`
	ExpiresAtMS int64       `json:"expires_at,omitempty"`
	PublisherID string      `json:"publisher_id,omitempty"`
}

type pendingDelivery struct {
	data      []byte
	attempts  int
	nextRetry time.Time
	expiresAt time.Time
}

type SSEClient struct {
	userID uint
	send   chan Message
}

type Client struct {
	hub       *Hub
	conn      *websocket.Conn
	send      chan []byte
	userID    uint
	pendingMu sync.Mutex
	pending   map[string]*pendingDelivery
}

type Hub struct {
	mu          sync.RWMutex
	clients     map[uint]map[*Client]bool
	sseClients  map[uint]map[*SSEClient]bool
	register    chan *Client
	unregister  chan *Client
	stopCh      chan struct{}
	runDone     chan struct{}
	stopOnce    sync.Once
	offlineSem  chan struct{}
	offlineWG   sync.WaitGroup
	operationWG sync.WaitGroup
	clientWG    sync.WaitGroup
	stopped     bool
	wsRepo      *repository.WSMessageRepository
	eventBus    *redisEventBus
	seen        map[string]time.Time
	fallbackSeq atomic.Uint64
}

var globalHub *Hub
var hubOnce sync.Once

func GetHub() *Hub {
	hubOnce.Do(func() {
		globalHub = newHub()
		go globalHub.run()
	})
	return globalHub
}

func newHub() *Hub {
	return &Hub{
		clients: make(map[uint]map[*Client]bool), sseClients: make(map[uint]map[*SSEClient]bool), register: make(chan *Client, 64),
		unregister: make(chan *Client, 64), stopCh: make(chan struct{}), runDone: make(chan struct{}),
		offlineSem: make(chan struct{}, 4), seen: make(map[string]time.Time),
	}
}

// ConfigureEventBus starts one Redis subscription per API/Worker process.
func (h *Hub) ConfigureEventBus(parent context.Context, transport EventTransport, channel, nodeID string) error {
	if h == nil {
		return fmt.Errorf("WebSocket hub is nil")
	}
	bus, err := newRedisEventBus(parent, transport, channel, nodeID, h)
	if err != nil {
		return fmt.Errorf("启动 WebSocket 事件总线失败: %w", err)
	}
	h.mu.Lock()
	if h.stopped {
		h.mu.Unlock()
		bus.Stop()
		return fmt.Errorf("WebSocket hub already stopped")
	}
	old := h.eventBus
	h.eventBus = bus
	h.mu.Unlock()
	if old != nil {
		old.Stop()
	}
	return nil
}

func (h *Hub) Stop() {
	if h == nil {
		return
	}
	h.stopOnce.Do(func() {
		h.mu.Lock()
		h.stopped = true
		bus := h.eventBus
		h.eventBus = nil
		h.mu.Unlock()
		if bus != nil {
			bus.Stop()
		}
		close(h.stopCh)
		<-h.runDone
		h.offlineWG.Wait()
		h.operationWG.Wait()

		h.mu.Lock()
		all := make(map[*Client]struct{})
		for _, conns := range h.clients {
			for client := range conns {
				all[client] = struct{}{}
			}
		}
		for {
			select {
			case client := <-h.register:
				all[client] = struct{}{}
			default:
				goto drained
			}
		}
	drained:
		h.clients = make(map[uint]map[*Client]bool)
		h.mu.Unlock()
		for client := range all {
			close(client.send)
			_ = client.conn.Close()
		}
		h.clientWG.Wait()
	})
}

func (h *Hub) SetWSMessageRepository(repo *repository.WSMessageRepository) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wsRepo = repo
}

func (h *Hub) run() {
	defer close(h.runDone)
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.stopped {
				h.mu.Unlock()
				_ = client.conn.Close()
				continue
			}
			if h.clients[client.userID] == nil {
				h.clients[client.userID] = make(map[*Client]bool)
			}
			h.clients[client.userID][client] = true
			repo, count := h.wsRepo, len(h.clients[client.userID])
			h.mu.Unlock()
			log.Printf("[WS] 用户 %d 已连接，当前连接数: %d", client.userID, count)
			if repo != nil {
				h.scheduleOfflineDelivery(client, repo)
			}
		case client := <-h.unregister:
			h.mu.Lock()
			if conns := h.clients[client.userID]; conns != nil {
				if _, ok := conns[client]; ok {
					delete(conns, client)
					close(client.send)
				}
				if len(conns) == 0 {
					delete(h.clients, client.userID)
				}
			}
			h.mu.Unlock()
		case <-h.stopCh:
			return
		}
	}
}

func (h *Hub) scheduleOfflineDelivery(client *Client, repo *repository.WSMessageRepository) {
	if h.isStopped() {
		return
	}
	select {
	case h.offlineSem <- struct{}{}:
		h.offlineWG.Add(1)
		go func() {
			defer h.offlineWG.Done()
			defer func() { <-h.offlineSem }()
			h.deliverOfflineMessages(client, repo)
		}()
	default:
		log.Printf("[WS] 离线消息投递并发已满，稍后由重连补偿 user_id=%d", client.userID)
	}
}

func (h *Hub) isStopped() bool { h.mu.RLock(); defer h.mu.RUnlock(); return h.stopped }

func (h *Hub) beginOperation() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	if h.stopped {
		return false
	}
	h.operationWG.Add(1)
	return true
}

func (h *Hub) deliverOfflineMessages(client *Client, repo *repository.WSMessageRepository) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	messages, err := repo.WithContext(ctx).GetUndeliveredMessages(client.userID, 50)
	if err != nil {
		log.Printf("[WS] 获取用户 %d 离线消息失败: %v", client.userID, err)
		return
	}
	for _, stored := range messages {
		if h.isStopped() {
			return
		}
		var data interface{}
		if err := json.Unmarshal([]byte(stored.Data), &data); err != nil {
			data = stored.Data
		}
		expires := int64(0)
		if stored.ExpiresAt != nil {
			expires = stored.ExpiresAt.UnixMilli()
		}
		messageID := stored.MessageID
		if messageID == "" {
			messageID = fmt.Sprintf("legacy-%d", stored.ID)
		}
		msg := Message{Type: stored.Type, Data: data, UserID: stored.UserID, MessageID: messageID,
			Sequence: stored.Sequence, CreatedAtMS: stored.CreatedAt.UnixMilli(), ExpiresAtMS: expires}
		payload, marshalErr := json.Marshal(msg)
		if marshalErr != nil {
			continue
		}
		client.enqueue(msg, payload)
	}
}

func (h *Hub) prepareMessage(userID uint, msg Message) Message {
	now := time.Now()
	ttl := envutil.Duration("WS_MESSAGE_TTL", 24*time.Hour)
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	msg.UserID = userID
	if msg.MessageID == "" {
		msg.MessageID = uuid.NewString()
	}
	if msg.CreatedAtMS == 0 {
		msg.CreatedAtMS = now.UnixMilli()
	}
	if msg.ExpiresAtMS == 0 {
		msg.ExpiresAtMS = now.Add(ttl).UnixMilli()
	}
	h.mu.RLock()
	bus := h.eventBus
	h.mu.RUnlock()
	if msg.Sequence == 0 && userID != 0 {
		if bus != nil {
			if seq, err := bus.nextSequence(userID); err == nil {
				msg.Sequence = seq
			} else {
				log.Printf("[WS] 分配用户序号失败: %v", err)
			}
		}
		if msg.Sequence == 0 {
			msg.Sequence = h.fallbackSeq.Add(1)
		}
	}
	if bus != nil {
		msg.PublisherID = bus.nodeID
	}
	return msg
}

func (h *Hub) SendToUser(userID uint, msg Message) {
	if userID == 0 || !h.beginOperation() {
		return
	}
	defer h.operationWG.Done()
	msg = h.prepareMessage(userID, msg)
	h.mu.RLock()
	repo, bus := h.wsRepo, h.eventBus
	h.mu.RUnlock()
	if repo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		err := repo.WithContext(ctx).SaveMessageEnvelope(userID, msg.Type, msg.Data, msg.MessageID, msg.Sequence, time.UnixMilli(msg.ExpiresAtMS))
		cancel()
		if err != nil {
			log.Printf("[WS] 持久化消息失败 message_id=%s: %v", msg.MessageID, err)
		}
	}
	h.deliverNewEnvelope(msg)
	if bus != nil {
		if err := bus.publish(msg); err != nil {
			log.Printf("[WS] 跨节点发布失败 message_id=%s: %v", msg.MessageID, err)
		}
	}
}

func (h *Hub) Broadcast(msg Message) {
	if !h.beginOperation() {
		return
	}
	defer h.operationWG.Done()
	msg = h.prepareMessage(0, msg)
	h.mu.RLock()
	bus := h.eventBus
	h.mu.RUnlock()
	h.deliverNewEnvelope(msg)
	if bus != nil {
		if err := bus.publish(msg); err != nil {
			log.Printf("[WS] 跨节点广播失败 message_id=%s: %v", msg.MessageID, err)
		}
	}
}

func (h *Hub) acceptEnvelope(msg Message) {
	if !h.beginOperation() {
		return
	}
	defer h.operationWG.Done()
	h.deliverNewEnvelope(msg)
}

func (h *Hub) deliverNewEnvelope(msg Message) {
	if msg.ExpiresAtMS > 0 && time.Now().UnixMilli() >= msg.ExpiresAtMS {
		return
	}
	if !h.markSeen(msg.MessageID, msg.ExpiresAtMS) {
		return
	}
	payload, err := json.Marshal(msg)
	if err != nil {
		return
	}
	h.mu.RLock()
	clients := make([]*Client, 0)
	if msg.UserID == 0 {
		for _, conns := range h.clients {
			for client := range conns {
				clients = append(clients, client)
			}
		}
	} else {
		for client := range h.clients[msg.UserID] {
			clients = append(clients, client)
		}
	}
	sseClients := make([]*SSEClient, 0)
	if msg.UserID == 0 {
		for _, conns := range h.sseClients {
			for client := range conns {
				sseClients = append(sseClients, client)
			}
		}
	} else {
		for client := range h.sseClients[msg.UserID] {
			sseClients = append(sseClients, client)
		}
	}
	h.mu.RUnlock()
	for _, client := range clients {
		if !client.enqueue(msg, payload) {
			h.tryUnregister(client)
		}
	}
	for _, client := range sseClients {
		select {
		case client.send <- msg:
		default:
			h.unregisterSSE(client)
		}
	}
}

func (h *Hub) markSeen(messageID string, expiresAtMS int64) bool {
	if messageID == "" {
		return true
	}
	now := time.Now()
	expiry := now.Add(24 * time.Hour)
	if expiresAtMS > 0 {
		expiry = time.UnixMilli(expiresAtMS)
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if existing, ok := h.seen[messageID]; ok && existing.After(now) {
		return false
	}
	h.seen[messageID] = expiry
	if len(h.seen) > 4096 {
		for id, exp := range h.seen {
			if !exp.After(now) {
				delete(h.seen, id)
			}
		}
	}
	return true
}

func (h *Hub) acknowledge(userID uint, messageID string) {
	if messageID == "" || !h.beginOperation() {
		return
	}
	defer h.operationWG.Done()
	h.mu.RLock()
	clients := make([]*Client, 0)
	for client := range h.clients[userID] {
		clients = append(clients, client)
	}
	repo := h.wsRepo
	h.mu.RUnlock()
	for _, client := range clients {
		client.ack(messageID)
	}
	if repo != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := repo.WithContext(ctx).MarkAsDeliveredByMessageID(userID, messageID); err != nil {
			log.Printf("[WS] ACK 持久化失败 user_id=%d message_id=%s: %v", userID, messageID, err)
		}
	}
}

func (h *Hub) tryUnregister(client *Client) {
	select {
	case h.unregister <- client:
	default:
		log.Printf("[WS] unregister 队列已满 user_id=%d", client.userID)
	}
}

func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID uint) {
	if !h.beginOperation() {
		http.Error(w, "service shutting down", http.StatusServiceUnavailable)
		return
	}
	defer h.operationWG.Done()
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] 升级失败: %v", err)
		return
	}
	client := &Client{hub: h, conn: conn, send: make(chan []byte, 256), userID: userID, pending: make(map[string]*pendingDelivery)}
	select {
	case h.register <- client:
	case <-h.stopCh:
		_ = conn.Close()
		return
	default:
		_ = conn.Close()
		return
	}
	h.clientWG.Add(2)
	go func() { defer h.clientWG.Done(); client.writePump() }()
	go func() { defer h.clientWG.Done(); client.readPump() }()
}

func (c *Client) enqueue(msg Message, data []byte) bool {
	if msg.MessageID != "" && msg.Type != "pong" {
		timeout := envutil.Duration("WS_ACK_TIMEOUT", 5*time.Second)
		if timeout <= 0 {
			timeout = 5 * time.Second
		}
		expires := time.Time{}
		if msg.ExpiresAtMS > 0 {
			expires = time.UnixMilli(msg.ExpiresAtMS)
		}
		c.pendingMu.Lock()
		c.pending[msg.MessageID] = &pendingDelivery{data: append([]byte(nil), data...), attempts: 1, nextRetry: time.Now().Add(timeout), expiresAt: expires}
		c.pendingMu.Unlock()
	}
	select {
	case c.send <- data:
		return true
	default:
		c.ack(msg.MessageID)
		return false
	}
}

func (c *Client) ack(messageID string) {
	c.pendingMu.Lock()
	delete(c.pending, messageID)
	c.pendingMu.Unlock()
}

func (c *Client) retryDue(now time.Time) [][]byte {
	maxRetries := envutil.Int("WS_MAX_RETRIES", 3)
	if maxRetries < 1 {
		maxRetries = 1
	}
	timeout := envutil.Duration("WS_ACK_TIMEOUT", 5*time.Second)
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	var due [][]byte
	c.pendingMu.Lock()
	defer c.pendingMu.Unlock()
	for id, p := range c.pending {
		if !p.expiresAt.IsZero() && !p.expiresAt.After(now) {
			delete(c.pending, id)
			continue
		}
		if now.Before(p.nextRetry) {
			continue
		}
		if p.attempts >= maxRetries {
			delete(c.pending, id)
			continue
		}
		p.attempts++
		p.nextRetry = now.Add(timeout)
		due = append(due, append([]byte(nil), p.data...))
	}
	return due
}

func (c *Client) readPump() {
	defer func() { c.hub.tryUnregister(c); _ = c.conn.Close() }()
	c.conn.SetReadLimit(4096)
	_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error { return c.conn.SetReadDeadline(time.Now().Add(60 * time.Second)) })
	for {
		_, payload, err := c.conn.ReadMessage()
		if err != nil {
			return
		}
		_ = c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		var command struct {
			Type      string `json:"type"`
			MessageID string `json:"message_id"`
		}
		if json.Unmarshal(payload, &command) == nil && command.Type == "ack" {
			c.hub.acknowledge(c.userID, command.MessageID)
			continue
		}
		if isApplicationPing(payload) {
			pong, _ := json.Marshal(Message{Type: "pong", Data: map[string]interface{}{"ts": time.Now().UnixMilli()}})
			select {
			case c.send <- pong:
			default:
				return
			}
		}
	}
}

func isApplicationPing(payload []byte) bool {
	if len(payload) == 0 || len(payload) > 1024 {
		return false
	}
	var msg Message
	if json.Unmarshal(payload, &msg) != nil {
		return false
	}
	return msg.Type == "ping"
}

func (c *Client) writePump() {
	heartbeat := time.NewTicker(30 * time.Second)
	retryEvery := envutil.Duration("WS_ACK_TIMEOUT", 5*time.Second) / 2
	if retryEvery < 500*time.Millisecond {
		retryEvery = 500 * time.Millisecond
	}
	retry := time.NewTicker(retryEvery)
	defer func() { heartbeat.Stop(); retry.Stop(); _ = c.conn.Close() }()
	write := func(kind int, data []byte) error {
		_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
		return c.conn.WriteMessage(kind, data)
	}
	for {
		select {
		case message, ok := <-c.send:
			if !ok {
				_ = write(websocket.CloseMessage, nil)
				return
			}
			if write(websocket.TextMessage, message) != nil {
				return
			}
		case <-retry.C:
			for _, message := range c.retryDue(time.Now()) {
				if write(websocket.TextMessage, message) != nil {
					return
				}
			}
		case <-heartbeat.C:
			if write(websocket.PingMessage, nil) != nil {
				return
			}
		}
	}
}
