package handlers

import (
	"log"
	"net/http"

	"iac-platform/internal/websocket"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
)

var upgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// 允许所有来源（生产环境应该限制）
		return true
	},
	// 处理子协议：当客户端使用 Sec-WebSocket-Protocol 传递token时，
	// 服务器必须在响应中返回相同的子协议，否则连接会失败
	Subprotocols: []string{"access_token"},
}

// WebSocketHandler WebSocket处理器
type WebSocketHandler struct {
	hub *websocket.Hub
}

// NewWebSocketHandler 创建WebSocket处理器
func NewWebSocketHandler(hub *websocket.Hub) *WebSocketHandler {
	return &WebSocketHandler{
		hub: hub,
	}
}

// HandleConnection 处理WebSocket连接
// @Summary WebSocket连接
// @Description 建立WebSocket连接用于实时通信
// @Tags WebSocket
// @Param session_id path string true "Session ID"
// @Success 101 {string} string "Switching Protocols"
// @Router /api/v1/ws/editing/{session_id} [get]
func (h *WebSocketHandler) HandleConnection(c *gin.Context) {
	sessionID := c.Param("session_id")
	if sessionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	// 从上下文获取用户ID（假设已通过认证中间件）
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "未认证"})
		return
	}

	// 升级HTTP连接为WebSocket
	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ Failed to upgrade connection: %v", err)
		return
	}

	// 创建客户端
	client := websocket.NewClient(h.hub, conn, sessionID, userID.(string))

	// 注册客户端
	h.hub.Register(client)

	// 启动客户端的读写循环
	client.Start()

	log.Printf(" WebSocket connection established: session=%s, user=%s", sessionID, userID)
}

// GetConnectedSessions 获取已连接的会话列表
// @Summary 获取已连接的会话
// @Description 获取当前所有已连接的WebSocket会话列表
// @Tags WebSocket
// @Produce json
// @Success 200 {object} map[string]interface{} "成功"
// @Router /api/v1/ws/sessions [get]
func (h *WebSocketHandler) GetConnectedSessions(c *gin.Context) {
	sessions := h.hub.GetConnectedSessions()
	c.JSON(http.StatusOK, gin.H{
		"sessions": sessions,
		"count":    len(sessions),
	})
}
