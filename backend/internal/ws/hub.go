package ws

import (
	"caiyun/internal/repository"
	"encoding/json"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		if origin == "" {
			return true
		}
		if sameOriginHost(origin, r.Host) {
			return true
		}
		allowedOrigins := strings.TrimSpace(os.Getenv("ALLOWED_ORIGINS"))
		if allowedOrigins == "" {
			allowedOrigins = "http://localhost:3000,http://localhost:5173,http://127.0.0.1:3000,http://127.0.0.1:5173"
		}
		for _, allowed := range strings.Split(allowedOrigins, ",") {
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

	originHost := strings.ToLower(parsed.Hostname())
	originPort := parsed.Port()
	if originPort == "" {
		originPort = defaultPort(parsed.Scheme)
	}

	host, port, err := net.SplitHostPort(requestHost)
	if err != nil {
		host = requestHost
		port = ""
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

// Message WebSocket消息结构
type Message struct {
	Type   string      `json:"type"` // task_progress, task_complete, notification, queue_status
	Data   interface{} `json:"data"`
	UserID uint        `json:"user_id,omitempty"` // 可选，用于指定接收用户
}

// Client 单个WebSocket连接
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	send   chan []byte
	userID uint
}

// Hub 管理所有WebSocket连接，按userID分组
type Hub struct {
	mu         sync.RWMutex
	clients    map[uint]map[*Client]bool // userID -> clients
	register   chan *Client
	unregister chan *Client
	wsRepo     *repository.WSMessageRepository // WebSocket消息仓库
}

// 全局单例
var globalHub *Hub
var hubOnce sync.Once

// GetHub 获取全局Hub实例
func GetHub() *Hub {
	hubOnce.Do(func() {
		globalHub = &Hub{
			clients:    make(map[uint]map[*Client]bool),
			register:   make(chan *Client, 64),
			unregister: make(chan *Client, 64),
		}
		go globalHub.run()
	})
	return globalHub
}

// SetWSMessageRepository 设置WebSocket消息仓库（用于消息持久化）
func (h *Hub) SetWSMessageRepository(repo *repository.WSMessageRepository) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.wsRepo = repo
}

func (h *Hub) run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			if h.clients[client.userID] == nil {
				h.clients[client.userID] = make(map[*Client]bool)
			}
			h.clients[client.userID][client] = true
			wsRepo := h.wsRepo
			h.mu.Unlock()
			log.Printf("[WS] 用户 %d 已连接，当前连接数: %d", client.userID, len(h.clients[client.userID]))

			// 用户上线时推送离线消息
			if wsRepo != nil {
				go h.deliverOfflineMessages(client.userID, wsRepo)
			}

		case client := <-h.unregister:
			h.mu.Lock()
			if conns, ok := h.clients[client.userID]; ok {
				if _, exists := conns[client]; exists {
					delete(conns, client)
					close(client.send)
					if len(conns) == 0 {
						delete(h.clients, client.userID)
					}
				}
			}
			h.mu.Unlock()
			log.Printf("[WS] 用户 %d 已断开", client.userID)
		}
	}
}

// deliverOfflineMessages 推送离线消息给用户
func (h *Hub) deliverOfflineMessages(userID uint, wsRepo *repository.WSMessageRepository) {
	// 获取未读消息
	messages, err := wsRepo.GetUndeliveredMessages(userID, 50)
	if err != nil {
		log.Printf("[WS] 获取用户 %d 的离线消息失败: %v", userID, err)
		return
	}

	if len(messages) == 0 {
		return
	}

	log.Printf("[WS] 推送 %d 条离线消息给用户 %d", len(messages), userID)

	for _, msg := range messages {
		// 解析消息数据
		var data interface{}
		if err := json.Unmarshal([]byte(msg.Data), &data); err != nil {
			data = msg.Data
		}

		// 发送消息
		h.SendToUser(userID, Message{
			Type: msg.Type,
			Data: data,
		})

		// 标记为已送达
		if err := wsRepo.MarkAsDelivered(msg.ID); err != nil {
			log.Printf("[WS] 标记消息 %d 为已送达失败: %v", msg.ID, err)
		}
	}
}

// SendToUser 向指定用户的所有连接推送消息（支持持久化）
func (h *Hub) SendToUser(userID uint, msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		log.Printf("[WS] 序列化消息失败: %v", err)
		return
	}

	h.mu.RLock()
	conns := h.clients[userID]
	clients := make([]*Client, 0, len(conns))
	for client := range conns {
		clients = append(clients, client)
	}
	wsRepo := h.wsRepo
	h.mu.RUnlock()

	// 检查用户是否在线
	isOnline := len(clients) > 0

	// 如果用户不在线且启用了持久化，保存消息到数据库
	if !isOnline && wsRepo != nil {
		err := wsRepo.SaveMessage(userID, msg.Type, msg.Data)
		if err != nil {
			log.Printf("[WS] 保存离线消息失败: %v", err)
		} else {
			log.Printf("[WS] 用户 %d 不在线，消息已持久化", userID)
		}
		return
	}

	// 发送给所有连接
	delivered := false
	for _, client := range clients {
		select {
		case client.send <- data:
			delivered = true
		default:
			// 发送缓冲区满，关闭连接
			h.unregister <- client
		}
	}

	// 如果发送失败且启用了持久化，保存消息
	if !delivered && wsRepo != nil {
		err := wsRepo.SaveMessage(userID, msg.Type, msg.Data)
		if err != nil {
			log.Printf("[WS] 保存未送达消息失败: %v", err)
		}
	}
}

// Broadcast 向所有连接广播消息
func (h *Hub) Broadcast(msg Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, conns := range h.clients {
		for client := range conns {
			select {
			case client.send <- data:
			default:
				go func(c *Client) { h.unregister <- c }(client)
			}
		}
	}
}

// HandleWebSocket 处理WebSocket升级请求
func (h *Hub) HandleWebSocket(w http.ResponseWriter, r *http.Request, userID uint) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("[WS] 升级失败: %v", err)
		return
	}

	client := &Client{
		hub:    h,
		conn:   conn,
		send:   make(chan []byte, 256),
		userID: userID,
	}

	h.register <- client

	go client.writePump()
	go client.readPump()
}

// readPump 读取客户端消息（主要用于保持连接和处理ping/pong）
func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(4096)
	c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	for {
		_, _, err := c.conn.ReadMessage()
		if err != nil {
			break
		}
	}
}

// writePump 向客户端写入消息
func (c *Client) writePump() {
	ticker := time.NewTicker(30 * time.Second)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case message, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if !ok {
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, message); err != nil {
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
