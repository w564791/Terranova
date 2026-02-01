package websocket

import (
	"encoding/json"
	"log"
	"sync"
)

// Hub 管理所有WebSocket连接
type Hub struct {
	// 按session_id索引的客户端连接
	clients map[string]*Client

	// 广播消息通道
	broadcast chan Message

	// 注册新客户端
	register chan *Client

	// 注销客户端
	unregister chan *Client

	// 保护clients map的互斥锁
	mu sync.RWMutex
}

// Message WebSocket消息结构
type Message struct {
	Type      string      `json:"type"`       // 消息类型
	SessionID string      `json:"session_id"` // 目标session_id（点对点消息）
	Data      interface{} `json:"data"`       // 消息数据
}

// NewHub 创建新的Hub实例
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		broadcast:  make(chan Message, 256),
		register:   make(chan *Client),
		unregister: make(chan *Client),
	}
}

// Run 启动Hub的主循环
func (h *Hub) Run() {
	for {
		select {
		case client := <-h.register:
			h.mu.Lock()
			// 如果该session已有连接，先关闭旧连接
			if oldClient, exists := h.clients[client.sessionID]; exists {
				log.Printf("  Session %s already connected, closing old connection", client.sessionID)
				close(oldClient.send)
			}
			h.clients[client.sessionID] = client
			h.mu.Unlock()
			log.Printf(" Client registered: session=%s, total=%d", client.sessionID, len(h.clients))

		case client := <-h.unregister:
			h.mu.Lock()
			// 只有当前注册的client与要注销的client是同一个时才注销
			// 这样可以避免旧连接被新连接替换后，旧连接的unregister关闭新连接的channel
			if currentClient, exists := h.clients[client.sessionID]; exists && currentClient == client {
				delete(h.clients, client.sessionID)
				close(client.send)
				log.Printf("❌ Client unregistered: session=%s, total=%d", client.sessionID, len(h.clients))
			} else if exists {
				log.Printf("  Ignoring unregister for old client: session=%s (already replaced)", client.sessionID)
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			// 如果指定了session_id，则点对点发送
			if message.SessionID != "" {
				h.sendToSession(message.SessionID, message)
			} else {
				// 否则广播给所有客户端
				h.broadcastToAll(message)
			}
		}
	}
}

// Register 注册客户端（公开方法）
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// Unregister 注销客户端（公开方法）
func (h *Hub) Unregister(client *Client) {
	h.unregister <- client
}

// Broadcast 广播消息（公开方法）
func (h *Hub) Broadcast(message Message) {
	h.broadcast <- message
}

// SendToSession 发送消息给指定session
func (h *Hub) SendToSession(sessionID string, message Message) {
	h.mu.RLock()
	client, exists := h.clients[sessionID]
	h.mu.RUnlock()

	if exists {
		h.sendToClient(client, message)
	} else {
		log.Printf("  Session %s not connected, message not sent: type=%s", sessionID, message.Type)
	}
}

// sendToSession 内部方法，发送消息给指定session
func (h *Hub) sendToSession(sessionID string, message Message) {
	h.mu.RLock()
	client, exists := h.clients[sessionID]
	h.mu.RUnlock()

	if exists {
		h.sendToClient(client, message)
	}
}

// broadcastToAll 广播消息给所有客户端
func (h *Hub) broadcastToAll(message Message) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, client := range h.clients {
		h.sendToClient(client, message)
	}
}

// sendToClient 发送消息给指定客户端
func (h *Hub) sendToClient(client *Client, message Message) {
	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("❌ Failed to marshal message: %v", err)
		return
	}

	select {
	case client.send <- data:
		log.Printf("📤 Message sent to session %s: type=%s", client.sessionID, message.Type)
	default:
		// 发送缓冲区已满，关闭连接
		log.Printf("  Client send buffer full, closing connection: session=%s", client.sessionID)
		h.mu.Lock()
		delete(h.clients, client.sessionID)
		close(client.send)
		h.mu.Unlock()
	}
}

// GetConnectedSessions 获取所有已连接的session列表
func (h *Hub) GetConnectedSessions() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	sessions := make([]string, 0, len(h.clients))
	for sessionID := range h.clients {
		sessions = append(sessions, sessionID)
	}
	return sessions
}

// IsSessionConnected 检查指定session是否已连接
func (h *Hub) IsSessionConnected(sessionID string) bool {
	h.mu.RLock()
	defer h.mu.RUnlock()

	_, exists := h.clients[sessionID]
	return exists
}

// GetClientCount 获取当前连接的客户端数量
func (h *Hub) GetClientCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()

	return len(h.clients)
}
