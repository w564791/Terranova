package websocket

import (
	"encoding/json"
	"log"
	"os"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"iac-platform/internal/pgpubsub"
)

const metricsChannel = "agent_metrics_broadcast"

// AgentMetricsHub 管理agent metrics的WebSocket连接
type AgentMetricsHub struct {
	// 按pool_id索引的客户端连接（前端连接）
	poolClients map[string]map[*websocket.Conn]bool

	// 存储每个agent的最新metrics
	agentMetrics map[string]*AgentMetrics

	// 广播消息通道
	broadcast chan AgentMetricsMessage

	// 注册新客户端
	register chan *PoolWebSocketClient

	// 注销客户端
	unregister chan *PoolWebSocketClient

	// 保护map的互斥锁
	mu sync.RWMutex

	// 跨副本广播
	db      *gorm.DB
	podName string
}

// PoolWebSocketClient 包含pool_id和websocket连接的结构
type PoolWebSocketClient struct {
	PoolID string
	Conn   *websocket.Conn
}

// AgentMetrics agent的实时metrics数据
type AgentMetrics struct {
	AgentID        string        `json:"agent_id"`
	AgentName      string        `json:"agent_name"`
	CPUUsage       float64       `json:"cpu_usage"`        // CPU使用率 0-100
	MemoryUsage    float64       `json:"memory_usage"`     // 内存使用率 0-100
	RunningTasks   []RunningTask `json:"running_tasks"`    // 当前运行的任务
	LastUpdateTime time.Time     `json:"last_update_time"` // 最后更新时间
	Status         string        `json:"status"`           // agent状态
}

// RunningTask 运行中的任务信息
type RunningTask struct {
	TaskID      uint   `json:"task_id"`
	TaskType    string `json:"task_type"`
	WorkspaceID string `json:"workspace_id"`
	StartedAt   string `json:"started_at"`
}

// AgentMetricsMessage WebSocket消息结构
type AgentMetricsMessage struct {
	Type    string        `json:"type"`    // 消息类型: "metrics_update", "agent_offline"
	PoolID  string        `json:"pool_id"` // 目标pool_id
	Metrics *AgentMetrics `json:"metrics"` // metrics数据
}

// NewAgentMetricsHub 创建新的AgentMetricsHub实例
func NewAgentMetricsHub() *AgentMetricsHub {
	return &AgentMetricsHub{
		poolClients:  make(map[string]map[*websocket.Conn]bool),
		agentMetrics: make(map[string]*AgentMetrics),
		broadcast:    make(chan AgentMetricsMessage, 256),
		register:     make(chan *PoolWebSocketClient),
		unregister:   make(chan *PoolWebSocketClient),
	}
}

// Run 启动Hub的主循环
func (h *AgentMetricsHub) Run() {
	// 启动清理goroutine，定期清理过期的metrics
	go h.cleanupExpiredMetrics()

	for {
		select {
		case poolClient := <-h.register:
			h.mu.Lock()
			if h.poolClients[poolClient.PoolID] == nil {
				h.poolClients[poolClient.PoolID] = make(map[*websocket.Conn]bool)
			}
			h.poolClients[poolClient.PoolID][poolClient.Conn] = true
			h.mu.Unlock()
			log.Printf("[AgentMetricsHub] Client registered: pool=%s, total=%d",
				poolClient.PoolID, len(h.poolClients[poolClient.PoolID]))

			// 发送当前pool的所有agent metrics给新连接的客户端
			h.sendCurrentMetrics(poolClient.PoolID, poolClient.Conn)

		case poolClient := <-h.unregister:
			h.mu.Lock()
			if clients, exists := h.poolClients[poolClient.PoolID]; exists {
				if _, ok := clients[poolClient.Conn]; ok {
					delete(clients, poolClient.Conn)
					poolClient.Conn.Close()
					log.Printf("[AgentMetricsHub] Client unregistered: pool=%s, remaining=%d",
						poolClient.PoolID, len(clients))

					// 如果该pool没有客户端了，删除map entry
					if len(clients) == 0 {
						delete(h.poolClients, poolClient.PoolID)
					}
				}
			}
			h.mu.Unlock()

		case message := <-h.broadcast:
			h.handleBroadcast(message)
		}
	}
}

// RegisterConn 注册WebSocket连接
func (h *AgentMetricsHub) RegisterConn(poolID string, conn *websocket.Conn) {
	h.register <- &PoolWebSocketClient{
		PoolID: poolID,
		Conn:   conn,
	}
}

// UnregisterConn 注销WebSocket连接
func (h *AgentMetricsHub) UnregisterConn(poolID string, conn *websocket.Conn) {
	h.unregister <- &PoolWebSocketClient{
		PoolID: poolID,
		Conn:   conn,
	}
}

// crossReplicaMetrics PG NOTIFY payload
type crossReplicaMetrics struct {
	SourcePod string        `json:"source_pod"`
	PoolID    string        `json:"pool_id"`
	Metrics   *AgentMetrics `json:"metrics"`
}

// SetupCrossReplicaListener configures PG NOTIFY for metrics broadcasting across replicas.
func (h *AgentMetricsHub) SetupCrossReplicaListener(ps *pgpubsub.PubSub, db *gorm.DB) {
	h.db = db
	h.podName = os.Getenv("POD_NAME")
	if h.podName == "" {
		h.podName, _ = os.Hostname()
	}

	ps.Subscribe(metricsChannel, func(payload string) {
		var msg crossReplicaMetrics
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			return
		}
		if msg.SourcePod == h.podName {
			return
		}
		// Apply metrics from other replica locally (no re-broadcast)
		h.applyMetrics(msg.PoolID, msg.Metrics)
	})

	log.Printf("[AgentMetricsHub] Cross-replica listener setup: podName=%s", h.podName)
}

// applyMetrics updates local cache and pushes to local WebSocket clients.
func (h *AgentMetricsHub) applyMetrics(poolID string, metrics *AgentMetrics) {
	h.mu.Lock()
	h.agentMetrics[metrics.AgentID] = metrics
	h.mu.Unlock()

	h.broadcast <- AgentMetricsMessage{
		Type:    "metrics_update",
		PoolID:  poolID,
		Metrics: metrics,
	}
}

// BroadcastMetrics 广播agent metrics更新（本地 + 跨副本）
func (h *AgentMetricsHub) BroadcastMetrics(poolID string, metrics *AgentMetrics) {
	// 本地广播
	h.applyMetrics(poolID, metrics)

	// 跨副本广播
	if h.db != nil {
		msg := crossReplicaMetrics{
			SourcePod: h.podName,
			PoolID:    poolID,
			Metrics:   metrics,
		}
		if err := pgpubsub.Notify(h.db, metricsChannel, msg); err != nil {
			log.Printf("[AgentMetricsHub] Failed to broadcast metrics via PG NOTIFY: %v", err)
		}
	}
}

// BroadcastAgentOffline 广播agent离线消息
func (h *AgentMetricsHub) BroadcastAgentOffline(poolID string, agentID string) {
	// 删除存储的metrics
	h.mu.Lock()
	delete(h.agentMetrics, agentID)
	h.mu.Unlock()

	// 广播离线消息
	h.broadcast <- AgentMetricsMessage{
		Type:   "agent_offline",
		PoolID: poolID,
		Metrics: &AgentMetrics{
			AgentID: agentID,
			Status:  "offline",
		},
	}
}

// handleBroadcast 处理广播消息
func (h *AgentMetricsHub) handleBroadcast(message AgentMetricsMessage) {
	h.mu.RLock()
	clients, exists := h.poolClients[message.PoolID]
	h.mu.RUnlock()

	if !exists || len(clients) == 0 {
		return
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[AgentMetricsHub] Failed to marshal agent metrics message: %v", err)
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()

	for conn := range clients {
		if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
			log.Printf("[AgentMetricsHub] Failed to send message to client: %v", err)
			// 连接出错，将在下次心跳时被清理
		}
	}
}

// sendCurrentMetrics 发送当前pool的所有agent metrics给指定客户端
func (h *AgentMetricsHub) sendCurrentMetrics(poolID string, conn *websocket.Conn) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// 收集该pool的所有agent metrics
	var metricsToSend []*AgentMetrics
	for _, metrics := range h.agentMetrics {
		metricsToSend = append(metricsToSend, metrics)
	}

	if len(metricsToSend) == 0 {
		return
	}

	// 发送初始化消息
	message := map[string]interface{}{
		"type":    "initial_metrics",
		"pool_id": poolID,
		"metrics": metricsToSend,
	}

	data, err := json.Marshal(message)
	if err != nil {
		log.Printf("[AgentMetricsHub] Failed to marshal initial metrics: %v", err)
		return
	}

	if err := conn.WriteMessage(websocket.TextMessage, data); err != nil {
		log.Printf("[AgentMetricsHub] Failed to send initial metrics: %v", err)
	} else {
		log.Printf("[AgentMetricsHub] Sent initial metrics to client: pool=%s, count=%d", poolID, len(metricsToSend))
	}
}

// cleanupExpiredMetrics 定期清理过期的metrics（超过5分钟未更新）
func (h *AgentMetricsHub) cleanupExpiredMetrics() {
	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		h.mu.Lock()
		now := time.Now()
		for agentID, metrics := range h.agentMetrics {
			if now.Sub(metrics.LastUpdateTime) > 5*time.Minute {
				log.Printf("[AgentMetricsHub] Cleaning up expired metrics for agent: %s", agentID)
				delete(h.agentMetrics, agentID)
			}
		}
		h.mu.Unlock()
	}
}
