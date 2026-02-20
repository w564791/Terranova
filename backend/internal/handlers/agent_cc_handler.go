package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// AgentCCHandler handles Agent C&C (Command & Control) WebSocket connections
type AgentCCHandler struct {
	db       *gorm.DB
	upgrader websocket.Upgrader
	agents   map[string]*AgentConnection // agentID -> connection
	mu       sync.RWMutex
}

// AgentConnection represents a C&C connection to an agent
type AgentConnection struct {
	AgentID    string
	conn       *websocket.Conn
	connMu     sync.Mutex // Protects conn for all operations
	LastPingAt time.Time
	Status     AgentStatus
	statusMu   sync.RWMutex // Separate mutex for status

	// Lifecycle management
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{} // Signals when all goroutines have finished
}

// AgentStatus represents agent runtime status
type AgentStatus struct {
	PlanRunning  int     `json:"plan_running"`
	PlanLimit    int     `json:"plan_limit"`
	ApplyRunning bool    `json:"apply_running"`
	CurrentTasks []uint  `json:"current_tasks"`
	CPUUsage     float64 `json:"cpu_usage"`
	MemUsage     float64 `json:"mem_usage"`
}

// CCMessage represents a C&C message
type CCMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// NewAgentCCHandler creates a new C&C handler
func NewAgentCCHandler(db *gorm.DB) *AgentCCHandler {
	return &AgentCCHandler{
		db: db,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			EnableCompression: false,
			// Add timeouts
			HandshakeTimeout: 10 * time.Second,
		},
		agents: make(map[string]*AgentConnection),
	}
}

// HandleCCConnection handles C&C WebSocket connection
func (h *AgentCCHandler) HandleCCConnection(c *gin.Context) {
	agentID := c.Query("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_id is required"})
		return
	}

	// Verify agent exists
	var agent models.Agent
	if err := h.db.Where("agent_id = ?", agentID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Clean up any existing connection
	h.cleanupExistingConnection(agentID)

	// Create new connection with proper lifecycle management
	ctx, cancel := context.WithCancel(context.Background())
	agentConn := &AgentConnection{
		AgentID:    agentID,
		conn:       conn,
		LastPingAt: time.Now(),
		ctx:        ctx,
		cancel:     cancel,
		done:       make(chan struct{}),
		Status: AgentStatus{
			PlanLimit: 3,
		},
	}

	// Register connection
	h.mu.Lock()
	h.agents[agentID] = agentConn
	h.mu.Unlock()

	log.Printf("Agent %s connected to C&C channel", agentID)

	// Update agent status
	h.db.Model(&agent).Updates(map[string]interface{}{
		"status":       "online",
		"last_ping_at": time.Now(),
	})

	// Start connection handler
	go h.handleConnection(agentConn)
}

// cleanupExistingConnection properly closes an existing connection
func (h *AgentCCHandler) cleanupExistingConnection(agentID string) {
	h.mu.Lock()
	existingConn, exists := h.agents[agentID]
	h.mu.Unlock()

	if !exists {
		return
	}

	log.Printf("Closing existing connection for agent %s", agentID)

	// Signal shutdown
	existingConn.cancel()

	// Wait for graceful shutdown with timeout
	select {
	case <-existingConn.done:
		log.Printf("Existing connection for agent %s closed gracefully", agentID)
	case <-time.After(2 * time.Second):
		log.Printf("Timeout waiting for existing connection to close for agent %s", agentID)
		// Force close
		existingConn.connMu.Lock()
		existingConn.conn.Close()
		existingConn.connMu.Unlock()
	}
}

// handleConnection manages the entire lifecycle of a connection
func (h *AgentCCHandler) handleConnection(agentConn *AgentConnection) {
	defer func() {
		// Cleanup
		h.mu.Lock()
		delete(h.agents, agentConn.AgentID)
		h.mu.Unlock()

		// Close connection
		agentConn.connMu.Lock()
		agentConn.conn.Close()
		agentConn.connMu.Unlock()

		// Update database
		h.db.Model(&models.Agent{}).
			Where("agent_id = ?", agentConn.AgentID).
			Updates(map[string]interface{}{
				"status":       "offline",
				"last_ping_at": time.Now(),
			})

		log.Printf("Agent %s disconnected from C&C channel", agentConn.AgentID)

		// Signal that we're done
		close(agentConn.done)
	}()

	// Start goroutines
	var wg sync.WaitGroup
	wg.Add(2)

	// Read messages
	go func() {
		defer wg.Done()
		h.readMessages(agentConn)
	}()

	// Monitor health
	go func() {
		defer wg.Done()
		h.monitorHealth(agentConn)
	}()

	// Wait for context cancellation
	<-agentConn.ctx.Done()

	// Give goroutines time to finish
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(1 * time.Second):
		log.Printf("Timeout waiting for goroutines to finish for agent %s", agentConn.AgentID)
	}
}

// readMessages reads messages from the agent
func (h *AgentCCHandler) readMessages(agentConn *AgentConnection) {
	log.Printf("[Server] Starting readMessages for agent %s", agentConn.AgentID)

	// Set read deadline
	agentConn.connMu.Lock()
	agentConn.conn.SetReadDeadline(time.Now().Add(150 * time.Second)) // 2.5 minutes
	agentConn.connMu.Unlock()

	for {
		select {
		case <-agentConn.ctx.Done():
			log.Printf("[Server] Context done, stopping readMessages for agent %s", agentConn.AgentID)
			return
		default:
		}

		var msg CCMessage

		// Read with lock
		agentConn.connMu.Lock()
		err := agentConn.conn.ReadJSON(&msg)
		agentConn.conn.SetReadDeadline(time.Now().Add(150 * time.Second)) // Reset deadline
		agentConn.connMu.Unlock()

		if err != nil {
			log.Printf("[Server] Error reading from agent %s: %v", agentConn.AgentID, err)
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
				log.Printf("WebSocket error for agent %s: %v", agentConn.AgentID, err)
			}
			agentConn.cancel() // Signal shutdown
			return
		}

		// Only log non-heartbeat messages to reduce log noise
		if msg.Type != "heartbeat" {
			log.Printf("[Server] Received message from agent %s: type=%s", agentConn.AgentID, msg.Type)
		}

		// Handle message
		switch msg.Type {
		case "heartbeat":
			h.handleHeartbeat(agentConn, msg.Payload)
		case "task_completed":
			h.handleTaskCompleted(agentConn, msg.Payload)
		case "task_failed":
			h.handleTaskFailed(agentConn, msg.Payload)
		default:
			log.Printf("Unknown message type from agent %s: %s", agentConn.AgentID, msg.Type)
		}
	}
}

// monitorHealth monitors connection health
func (h *AgentCCHandler) monitorHealth(agentConn *AgentConnection) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-agentConn.ctx.Done():
			return
		case <-ticker.C:
			agentConn.statusMu.RLock()
			lastPing := agentConn.LastPingAt
			agentConn.statusMu.RUnlock()

			if time.Since(lastPing) > 120*time.Second {
				log.Printf("Agent %s heartbeat timeout", agentConn.AgentID)
				agentConn.cancel() // Signal shutdown
				return
			}
		}
	}
}

// sendMessage sends a message to the agent with proper locking
func (h *AgentCCHandler) sendMessage(agentConn *AgentConnection, msg CCMessage) error {
	// First serialize to JSON
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	// Set write deadline
	deadline := time.Now().Add(10 * time.Second)

	agentConn.connMu.Lock()
	defer agentConn.connMu.Unlock()

	// Check if connection is still valid
	select {
	case <-agentConn.ctx.Done():
		return fmt.Errorf("connection closed")
	default:
	}

	agentConn.conn.SetWriteDeadline(deadline)
	// Use WriteMessage instead of WriteJSON to avoid potential encoding issues
	err = agentConn.conn.WriteMessage(websocket.TextMessage, data)
	if err != nil {
		log.Printf("Failed to send message to agent %s: %v", agentConn.AgentID, err)
		agentConn.cancel() // Signal shutdown on write error
		return err
	}

	log.Printf("[Server] Successfully sent message to agent %s", agentConn.AgentID)
	return nil
}

// handleHeartbeat handles heartbeat message from agent
func (h *AgentCCHandler) handleHeartbeat(agentConn *AgentConnection, payload map[string]interface{}) {
	agentConn.statusMu.Lock()
	agentConn.LastPingAt = time.Now()

	// Update status fields
	if planRunning, ok := payload["plan_running"].(float64); ok {
		agentConn.Status.PlanRunning = int(planRunning)
	}
	if planLimit, ok := payload["plan_limit"].(float64); ok {
		agentConn.Status.PlanLimit = int(planLimit)
	}
	if applyRunning, ok := payload["apply_running"].(bool); ok {
		agentConn.Status.ApplyRunning = applyRunning
	}
	if currentTasks, ok := payload["current_tasks"].([]interface{}); ok {
		tasks := make([]uint, 0, len(currentTasks))
		for _, t := range currentTasks {
			if taskID, ok := t.(float64); ok {
				tasks = append(tasks, uint(taskID))
			}
		}
		agentConn.Status.CurrentTasks = tasks
	}
	if cpuUsage, ok := payload["cpu_usage"].(float64); ok {
		agentConn.Status.CPUUsage = cpuUsage
	}
	if memUsage, ok := payload["mem_usage"].(float64); ok {
		agentConn.Status.MemUsage = memUsage
	}
	agentConn.statusMu.Unlock() // CRITICAL: Must unlock after updating

	// Update database
	h.db.Model(&models.Agent{}).
		Where("agent_id = ?", agentConn.AgentID).
		Updates(map[string]interface{}{
			"status":       "online",
			"last_ping_at": time.Now(),
		})

	// Heartbeat processed silently - no log to reduce noise
}

// handleTaskCompleted handles task completion notification
func (h *AgentCCHandler) handleTaskCompleted(agentConn *AgentConnection, payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("Invalid task_id in task_completed message from agent %s", agentConn.AgentID)
		return
	}
	log.Printf("Agent %s reported task %d completed", agentConn.AgentID, uint(taskID))

	// 发送任务完成通知
	go h.sendTaskCompletedNotification(uint(taskID))
}

// sendTaskCompletedNotification 发送任务完成通知（根据任务状态发送不同类型的通知）
func (h *AgentCCHandler) sendTaskCompletedNotification(taskID uint) {
	// 获取任务信息
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		log.Printf("[Notification] Failed to get task %d for notification: %v", taskID, err)
		return
	}

	// 创建 NotificationSender
	platformConfigService := services.NewPlatformConfigService(h.db)
	baseURL := platformConfigService.GetBaseURL()
	notificationSender := services.NewNotificationSender(h.db, baseURL)

	ctx := context.Background()

	// 根据任务状态决定发送哪种通知
	switch task.Status {
	case models.TaskStatusApplied:
		// Apply 成功完成 - 发送 task_completed 通知
		log.Printf("[Notification] Triggering task_completed notification for task %d (status: applied)", taskID)
		if err := notificationSender.TriggerNotifications(
			ctx,
			task.WorkspaceID,
			models.NotificationEventTaskCompleted,
			&task,
		); err != nil {
			log.Printf("[Notification] Failed to send task_completed notification for task %d: %v", taskID, err)
		} else {
			log.Printf("[Notification] Successfully sent task_completed notification for task %d", taskID)
		}

	case models.TaskStatusApplyPending:
		// Plan 完成，等待 Apply 确认 - 发送 approval_required 通知
		log.Printf("[Notification] Triggering approval_required notification for task %d (status: apply_pending)", taskID)
		if err := notificationSender.TriggerNotifications(
			ctx,
			task.WorkspaceID,
			models.NotificationEventApprovalRequired,
			&task,
		); err != nil {
			log.Printf("[Notification] Failed to send approval_required notification for task %d: %v", taskID, err)
		} else {
			log.Printf("[Notification] Successfully sent approval_required notification for task %d", taskID)
		}

	case models.TaskStatusSuccess:
		// Plan 完成（无变更或 plan-only 任务）- 发送 task_planned 通知
		log.Printf("[Notification] Triggering task_planned notification for task %d (status: success)", taskID)
		if err := notificationSender.TriggerNotifications(
			ctx,
			task.WorkspaceID,
			models.NotificationEventTaskPlanned,
			&task,
		); err != nil {
			log.Printf("[Notification] Failed to send task_planned notification for task %d: %v", taskID, err)
		} else {
			log.Printf("[Notification] Successfully sent task_planned notification for task %d", taskID)
		}

	case models.TaskStatusCancelled:
		// 任务被取消 - 发送 task_cancelled 通知
		log.Printf("[Notification] Triggering task_cancelled notification for task %d (status: cancelled)", taskID)
		if err := notificationSender.TriggerNotifications(
			ctx,
			task.WorkspaceID,
			models.NotificationEventTaskCancelled,
			&task,
		); err != nil {
			log.Printf("[Notification] Failed to send task_cancelled notification for task %d: %v", taskID, err)
		} else {
			log.Printf("[Notification] Successfully sent task_cancelled notification for task %d", taskID)
		}

	default:
		log.Printf("[Notification] Task %d has status %s, no notification sent", taskID, task.Status)
	}
}

// handleTaskFailed handles task failure notification
func (h *AgentCCHandler) handleTaskFailed(agentConn *AgentConnection, payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("Invalid task_id in task_failed message from agent %s", agentConn.AgentID)
		return
	}

	errorMsg := ""
	if err, ok := payload["error"].(string); ok {
		errorMsg = err
	}

	log.Printf("Agent %s reported task %d failed: %s", agentConn.AgentID, uint(taskID), errorMsg)

	// 发送任务失败通知
	go h.sendTaskFailedNotification(uint(taskID))
}

// sendTaskFailedNotification 发送任务失败通知
func (h *AgentCCHandler) sendTaskFailedNotification(taskID uint) {
	// 获取任务信息
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		log.Printf("[Notification] Failed to get task %d for notification: %v", taskID, err)
		return
	}

	// 创建 NotificationSender
	platformConfigService := services.NewPlatformConfigService(h.db)
	baseURL := platformConfigService.GetBaseURL()
	notificationSender := services.NewNotificationSender(h.db, baseURL)

	// 发送通知
	ctx := context.Background()
	if err := notificationSender.TriggerNotifications(
		ctx,
		task.WorkspaceID,
		models.NotificationEventTaskFailed,
		&task,
	); err != nil {
		log.Printf("[Notification] Failed to send task_failed notification for task %d: %v", taskID, err)
	} else {
		log.Printf("[Notification] Successfully sent task_failed notification for task %d", taskID)
	}
}

// SendTaskToAgent sends a task to agent via C&C channel
func (h *AgentCCHandler) SendTaskToAgent(agentID string, taskID uint, workspaceID string, action string) error {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	msg := CCMessage{
		Type: "run_task",
		Payload: map[string]interface{}{
			"task_id":      taskID,
			"workspace_id": workspaceID,
			"action":       action,
		},
	}

	return h.sendMessage(agentConn, msg)
}

// CancelTaskOnAgent sends cancel command to agent
func (h *AgentCCHandler) CancelTaskOnAgent(agentID string, taskID uint) error {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	msg := CCMessage{
		Type: "cancel_task",
		Payload: map[string]interface{}{
			"task_id": taskID,
		},
	}

	return h.sendMessage(agentConn, msg)
}

// StartRealtimeStream requests agent to start real-time log streaming
func (h *AgentCCHandler) StartRealtimeStream(agentID string, taskID uint) error {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	msg := CCMessage{
		Type: "start_realtime_stream",
		Payload: map[string]interface{}{
			"task_id": taskID,
		},
	}

	return h.sendMessage(agentConn, msg)
}

// StopRealtimeStream requests agent to stop real-time log streaming
func (h *AgentCCHandler) StopRealtimeStream(agentID string, taskID uint) error {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	msg := CCMessage{
		Type: "stop_realtime_stream",
		Payload: map[string]interface{}{
			"task_id": taskID,
		},
	}

	return h.sendMessage(agentConn, msg)
}

// BroadcastToAgent sends a message to specific agent
func (h *AgentCCHandler) BroadcastToAgent(agentID string, messageType string, payload map[string]interface{}) error {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return fmt.Errorf("agent %s not connected", agentID)
	}

	msg := CCMessage{
		Type:    messageType,
		Payload: payload,
	}

	return h.sendMessage(agentConn, msg)
}

// GetAgentStatusAPI returns agent status via HTTP API (for debugging)
// @Summary Get agent C&C status
// @Description Get current C&C connection status of an agent
// @Tags Agent
// @Accept json
// @Produce json
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id}/cc-status [get]
func (h *AgentCCHandler) GetAgentStatusAPI(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agent_id is required",
		})
		return
	}

	status, err := h.GetAgentStatus(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
		})
		return
	}

	h.mu.RLock()
	agentConn := h.agents[agentID]
	h.mu.RUnlock()

	c.JSON(http.StatusOK, gin.H{
		"agent_id":     agentID,
		"connected":    true,
		"last_ping_at": agentConn.LastPingAt,
		"status":       status,
	})
}

// GetAgentStatus returns current status of an agent
func (h *AgentCCHandler) GetAgentStatus(agentID string) (*AgentStatus, error) {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent %s not connected", agentID)
	}

	agentConn.statusMu.RLock()
	defer agentConn.statusMu.RUnlock()

	status := agentConn.Status
	return &status, nil
}

// IsAgentAvailable checks if agent can accept new tasks
func (h *AgentCCHandler) IsAgentAvailable(agentID string, taskType models.TaskType) bool {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		return false
	}

	agentConn.statusMu.RLock()
	defer agentConn.statusMu.RUnlock()

	if agentConn.Status.ApplyRunning {
		return false
	}

	if taskType == models.TaskTypePlan {
		return agentConn.Status.PlanRunning < agentConn.Status.PlanLimit
	}

	if taskType == models.TaskTypePlanAndApply {
		return agentConn.Status.PlanRunning == 0 && !agentConn.Status.ApplyRunning
	}

	return false
}

// GetConnectedAgents returns list of connected agent IDs
func (h *AgentCCHandler) GetConnectedAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	agents := make([]string, 0, len(h.agents))
	for agentID := range h.agents {
		agents = append(agents, agentID)
	}
	return agents
}
