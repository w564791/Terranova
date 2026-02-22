package handlers

import (
	"log"
	"net/http"
	"time"

	"iac-platform/internal/websocket"

	"github.com/gin-gonic/gin"
	gorillaws "github.com/gorilla/websocket"
)

var agentMetricsUpgrader = gorillaws.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin:     checkWebSocketOrigin,
}

// AgentMetricsWSHandler handles WebSocket connections for agent metrics
type AgentMetricsWSHandler struct {
	hub *websocket.AgentMetricsHub
}

// NewAgentMetricsWSHandler creates a new agent metrics WebSocket handler
func NewAgentMetricsWSHandler(hub *websocket.AgentMetricsHub) *AgentMetricsWSHandler {
	return &AgentMetricsWSHandler{
		hub: hub,
	}
}

// HandleAgentMetricsWS handles WebSocket connection for agent metrics
// @Summary Agent metrics WebSocket
// @Description WebSocket endpoint for real-time agent metrics updates
// @Tags WebSocket
// @Param pool_id path string true "Pool ID"
// @Router /ws/agent-pools/{pool_id}/metrics [get]
func (h *AgentMetricsWSHandler) HandleAgentMetricsWS(c *gin.Context) {
	poolID := c.Param("pool_id")
	if poolID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "pool_id is required"})
		return
	}

	// Upgrade HTTP connection to WebSocket
	conn, err := agentMetricsUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("❌ Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Register connection to hub
	h.hub.RegisterConn(poolID, conn)

	// Start goroutines for reading (to handle close and ping/pong)
	go h.readPump(poolID, conn)
}

// readPump reads messages from the WebSocket connection
// This is mainly to detect connection close and handle ping/pong
func (h *AgentMetricsWSHandler) readPump(poolID string, conn *gorillaws.Conn) {
	defer func() {
		h.hub.UnregisterConn(poolID, conn)
		conn.Close()
	}()

	conn.SetReadDeadline(time.Now().Add(60 * time.Second))
	conn.SetPongHandler(func(string) error {
		conn.SetReadDeadline(time.Now().Add(60 * time.Second))
		return nil
	})

	// Start ping ticker
	ticker := time.NewTicker(54 * time.Second)
	defer ticker.Stop()

	go func() {
		for range ticker.C {
			if err := conn.WriteMessage(gorillaws.PingMessage, nil); err != nil {
				return
			}
		}
	}()

	for {
		_, _, err := conn.ReadMessage()
		if err != nil {
			if gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseGoingAway, gorillaws.CloseAbnormalClosure) {
				log.Printf("  WebSocket error: %v", err)
			}
			break
		}
		// 前端不需要发送消息，只接收metrics更新
	}
}
