package handlers

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"iac-platform/internal/models"
	"iac-platform/internal/websocket"
	"iac-platform/internal/pgpubsub"
	"iac-platform/services"

	gorillaws "github.com/gorilla/websocket"
	"gorm.io/gorm"
)

// TaskDispatchChannel is the PG NOTIFY channel used for cross-replica agent task dispatch.
const TaskDispatchChannel = "task_dispatch"

// LogStreamForwardChannel is the PG NOTIFY channel used for forwarding
// real-time log lines across replicas so that frontends connected to any
// replica can see live task output.
const LogStreamForwardChannel = "log_stream_forward"

// TaskDispatchMessage is the payload sent over PG NOTIFY when the leader
// cannot deliver a task locally and needs another replica to handle it.
type TaskDispatchMessage struct {
	AgentID     string `json:"agent_id"`
	TaskID      uint   `json:"task_id"`
	WorkspaceID string `json:"workspace_id"`
	Action      string `json:"action"`
	SourcePod   string `json:"source_pod"`
}

// LogStreamForwardMessage is the payload sent over PG NOTIFY to forward
// real-time log lines from the replica that receives them (via agent WebSocket)
// to all other replicas for frontend delivery.
type LogStreamForwardMessage struct {
	TaskID    uint   `json:"task_id"`
	Type      string `json:"type"`
	Line      string `json:"line"`
	LineNum   int    `json:"line_num"`
	Stage     string `json:"stage,omitempty"`
	Status    string `json:"status,omitempty"`
	SourcePod string `json:"source_pod"`
}

// RawAgentCCHandler handles Agent C&C WebSocket connections using raw http.Handler
type RawAgentCCHandler struct {
	db            *gorm.DB
	streamManager *services.OutputStreamManager
	metricsHub    *websocket.AgentMetricsHub
	upgrader      gorillaws.Upgrader
	agents        map[string]*RawAgentConnection
	mu            sync.RWMutex
	podName       string // cached pod name for PG NOTIFY source identification
}

// RawAgentConnection represents a C&C connection to an agent
type RawAgentConnection struct {
	AgentID    string
	conn       *gorillaws.Conn
	connMu     sync.Mutex
	LastPingAt time.Time
	Status     AgentStatus
	statusMu   sync.RWMutex
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
}

// NewRawAgentCCHandler creates a new raw C&C handler
func NewRawAgentCCHandler(db *gorm.DB, streamManager *services.OutputStreamManager) *RawAgentCCHandler {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}
	return &RawAgentCCHandler{
		db:            db,
		streamManager: streamManager,
		podName:       podName,
		upgrader: gorillaws.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true
			},
			EnableCompression: false,
			HandshakeTimeout:  10 * time.Second,
		},
		agents: make(map[string]*RawAgentConnection),
	}
}

// ServeHTTP implements http.Handler interface for raw WebSocket handling
func (h *RawAgentCCHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Step 1: Validate Pool Token from Authorization header
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		log.Printf("[Raw] C&C connection rejected: missing or invalid Authorization header")
		http.Error(w, "Unauthorized: missing or invalid Authorization header", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// Calculate token hash (using Base64 to match PoolTokenService)
	hash := sha256.Sum256([]byte(token))
	tokenHash := base64.StdEncoding.EncodeToString(hash[:])

	// Query pool_tokens table
	var poolToken models.PoolToken
	if err := h.db.Where("token_hash = ? AND is_active = true", tokenHash).First(&poolToken).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("[Raw] C&C connection rejected: invalid or inactive token")
			http.Error(w, "Unauthorized: invalid or inactive token", http.StatusUnauthorized)
			return
		}
		log.Printf("[Raw] C&C connection rejected: database error: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Check token expiration
	if poolToken.ExpiresAt != nil && poolToken.ExpiresAt.Before(time.Now()) {
		log.Printf("[Raw] C&C connection rejected: token expired")
		http.Error(w, "Unauthorized: token expired", http.StatusUnauthorized)
		return
	}

	// Step 2: Extract and validate agent_id
	agentID := r.URL.Query().Get("agent_id")
	if agentID == "" {
		http.Error(w, "agent_id is required", http.StatusBadRequest)
		return
	}

	// Step 3: Verify agent exists AND belongs to the authenticated pool
	var agent models.Agent
	if err := h.db.Where("agent_id = ? AND pool_id = ?", agentID, poolToken.PoolID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			log.Printf("[Raw] C&C connection rejected: agent %s not found or not authorized for pool %s", agentID, poolToken.PoolID)
			http.Error(w, "Forbidden: agent not found or not authorized for this pool", http.StatusForbidden)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("[Raw] C&C connection authenticated: agent=%s, pool=%s", agentID, poolToken.PoolID)

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade to WebSocket: %v", err)
		return
	}

	// Clean up any existing connection
	h.cleanupExistingConnection(agentID)

	// Create new connection
	ctx, cancel := context.WithCancel(context.Background())
	agentConn := &RawAgentConnection{
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

	log.Printf("[Raw] Agent %s connected to C&C channel", agentID)

	// Update agent status (including connected_pod for HA routing)
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}
	h.db.Model(&agent).Updates(map[string]interface{}{
		"status":        "online",
		"last_ping_at":  time.Now(),
		"connected_pod": podName,
	})

	// Start connection handler
	go h.handleConnection(agentConn)
}

func (h *RawAgentCCHandler) cleanupExistingConnection(agentID string) {
	h.mu.Lock()
	existingConn, exists := h.agents[agentID]
	h.mu.Unlock()

	if !exists {
		return
	}

	log.Printf("[Raw] Closing existing connection for agent %s", agentID)
	existingConn.cancel()

	select {
	case <-existingConn.done:
		log.Printf("[Raw] Existing connection for agent %s closed gracefully", agentID)
	case <-time.After(2 * time.Second):
		log.Printf("[Raw] Timeout waiting for existing connection to close for agent %s", agentID)
		existingConn.connMu.Lock()
		existingConn.conn.Close()
		existingConn.connMu.Unlock()
	}
}

func (h *RawAgentCCHandler) handleConnection(agentConn *RawAgentConnection) {
	defer func() {
		h.mu.Lock()
		delete(h.agents, agentConn.AgentID)
		h.mu.Unlock()

		agentConn.connMu.Lock()
		agentConn.conn.Close()
		agentConn.connMu.Unlock()

		// 清理该 agent 正在执行的任务
		h.cleanupAgentTasks(agentConn.AgentID)

		h.db.Model(&models.Agent{}).
			Where("agent_id = ?", agentConn.AgentID).
			Updates(map[string]interface{}{
				"status":        "offline",
				"last_ping_at":  time.Now(),
				"connected_pod": gorm.Expr("NULL"),
			})

		log.Printf("[Raw] Agent %s disconnected from C&C channel", agentConn.AgentID)
		close(agentConn.done)
	}()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		h.readMessages(agentConn)
	}()

	go func() {
		defer wg.Done()
		h.monitorHealth(agentConn)
	}()

	<-agentConn.ctx.Done()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean shutdown
	case <-time.After(1 * time.Second):
		log.Printf("[Raw] Timeout waiting for goroutines to finish for agent %s", agentConn.AgentID)
	}
}

func (h *RawAgentCCHandler) readMessages(agentConn *RawAgentConnection) {
	log.Printf("[Raw] Starting readMessages for agent %s", agentConn.AgentID)

	// Don't set read deadline - let it block indefinitely
	// The health monitor will detect timeout

	for {
		select {
		case <-agentConn.ctx.Done():
			log.Printf("[Raw] Context done, stopping readMessages for agent %s", agentConn.AgentID)
			return
		default:
		}

		var msg CCMessage

		agentConn.connMu.Lock()
		conn := agentConn.conn
		agentConn.connMu.Unlock()

		if conn == nil {
			log.Printf("[Raw] Connection is nil for agent %s", agentConn.AgentID)
			return
		}

		err := conn.ReadJSON(&msg)

		if err != nil {
			log.Printf("[Raw] Error reading from agent %s: %v", agentConn.AgentID, err)
			if gorillaws.IsUnexpectedCloseError(err, gorillaws.CloseGoingAway, gorillaws.CloseAbnormalClosure) {
				log.Printf("[Raw] WebSocket error for agent %s: %v", agentConn.AgentID, err)
			}
			agentConn.cancel()
			return
		}

		// Debug logging - only log non-heartbeat and non-log_stream messages to reduce noise
		if msg.Type != "heartbeat" && msg.Type != "log_stream" {
			log.Printf("[Raw] Received message from agent %s: type=%s", agentConn.AgentID, msg.Type)
		}

		switch msg.Type {
		case "heartbeat":
			h.handleHeartbeat(agentConn, msg.Payload)
		case "task_completed":
			h.handleTaskCompleted(agentConn, msg.Payload)
		case "task_failed":
			h.handleTaskFailed(agentConn, msg.Payload)
		case "log_stream":
			h.handleLogStream(agentConn, msg.Payload)
		default:
			log.Printf("[Raw] Unknown message type from agent %s: %s", agentConn.AgentID, msg.Type)
		}
	}
}

func (h *RawAgentCCHandler) monitorHealth(agentConn *RawAgentConnection) {
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
				log.Printf("[Raw] Agent %s heartbeat timeout", agentConn.AgentID)
				agentConn.cancel()
				return
			}
		}
	}
}

func (h *RawAgentCCHandler) sendMessage(agentConn *RawAgentConnection, msg CCMessage) error {
	// Use raw bytes approach
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	deadline := time.Now().Add(10 * time.Second)

	agentConn.connMu.Lock()
	defer agentConn.connMu.Unlock()

	select {
	case <-agentConn.ctx.Done():
		return fmt.Errorf("connection closed")
	default:
	}

	agentConn.conn.SetWriteDeadline(deadline)

	// Write raw bytes directly
	err = agentConn.conn.WriteMessage(gorillaws.TextMessage, data)
	if err != nil {
		log.Printf("[Raw] Failed to send message to agent %s: %v", agentConn.AgentID, err)
		agentConn.cancel()
		return err
	}

	log.Printf("[Raw] Successfully sent message to agent %s", agentConn.AgentID)
	return nil
}

func (h *RawAgentCCHandler) handleHeartbeat(agentConn *RawAgentConnection, payload map[string]interface{}) {
	agentConn.statusMu.Lock()
	agentConn.LastPingAt = time.Now()

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

	// Copy status for metrics broadcasting
	cpuUsage := agentConn.Status.CPUUsage
	memUsage := agentConn.Status.MemUsage
	currentTasks := make([]uint, len(agentConn.Status.CurrentTasks))
	copy(currentTasks, agentConn.Status.CurrentTasks)

	// Debug logging
	log.Printf("[Metrics DEBUG] Agent %s heartbeat: cpu=%.2f%%, mem=%.2f%%, tasks=%v",
		agentConn.AgentID, cpuUsage, memUsage, currentTasks)

	agentConn.statusMu.Unlock()

	// Get agent info for pool_id and name
	var agent models.Agent
	if err := h.db.Where("agent_id = ?", agentConn.AgentID).First(&agent).Error; err == nil {
		// Update agent status in database (including connected_pod for HA routing)
		podName := os.Getenv("POD_NAME")
		if podName == "" {
			podName, _ = os.Hostname()
		}
		h.db.Model(&agent).Updates(map[string]interface{}{
			"status":        "online",
			"last_ping_at":  time.Now(),
			"connected_pod": podName,
		})

		// Broadcast metrics to AgentMetricsHub if available
		if h.metricsHub != nil && agent.PoolID != nil {
			// Convert current_tasks to RunningTask format
			var runningTasks []websocket.RunningTask
			for _, taskID := range currentTasks {
				// Get task info from database
				var task models.WorkspaceTask
				if err := h.db.Select("id, task_type, workspace_id, created_at").
					Where("id = ?", taskID).First(&task).Error; err == nil {
					runningTasks = append(runningTasks, websocket.RunningTask{
						TaskID:      taskID,
						TaskType:    string(task.TaskType),
						WorkspaceID: task.WorkspaceID,
						StartedAt:   task.CreatedAt.Format(time.RFC3339),
					})
				}
			}

			metrics := &websocket.AgentMetrics{
				AgentID:        agentConn.AgentID,
				AgentName:      agent.Name,
				CPUUsage:       cpuUsage,
				MemoryUsage:    memUsage,
				RunningTasks:   runningTasks,
				LastUpdateTime: time.Now(),
				Status:         agent.Status,
			}
			log.Printf("[Metrics DEBUG] Broadcasting to pool %s: agent=%s, cpu=%.2f%%, mem=%.2f%%",
				*agent.PoolID, agentConn.AgentID, cpuUsage, memUsage)
			h.metricsHub.BroadcastMetrics(*agent.PoolID, metrics)
		} else {
			if h.metricsHub == nil {
				log.Printf("[Metrics DEBUG] ERROR: metricsHub is nil!")
			}
			if agent.PoolID == nil {
				log.Printf("[Metrics DEBUG] ERROR: agent.PoolID is nil for agent %s", agentConn.AgentID)
			}
		}
	}

	// Heartbeat processed silently - no log to reduce noise
}

func (h *RawAgentCCHandler) handleTaskCompleted(agentConn *RawAgentConnection, payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("[Raw] Invalid task_id in task_completed message from agent %s", agentConn.AgentID)
		return
	}
	log.Printf("[Raw] Agent %s reported task %d completed", agentConn.AgentID, uint(taskID))

	// 处理 drift_check 任务结果
	go h.processDriftCheckResult(uint(taskID))

	// 发送任务完成通知
	go h.sendTaskCompletedNotification(uint(taskID))
}

// sendTaskCompletedNotification 发送任务完成通知（根据任务状态发送不同类型的通知）
func (h *RawAgentCCHandler) sendTaskCompletedNotification(taskID uint) {
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

		// 如果是全量 apply（没有 --target 参数），清除 drift 状态
		if !h.hasTargetParameter(&task) {
			log.Printf("[Drift] Clearing drift status for workspace %s after full apply (task %d)", task.WorkspaceID, taskID)
			driftService := services.NewDriftCheckService(h.db)
			if err := driftService.ClearDriftOnFullApply(task.WorkspaceID); err != nil {
				log.Printf("[Drift] Failed to clear drift status for workspace %s: %v", task.WorkspaceID, err)
			} else {
				log.Printf("[Drift] Successfully cleared drift status for workspace %s", task.WorkspaceID)
			}
		} else {
			log.Printf("[Drift] Skipping drift clear for workspace %s - task %d has --target parameter", task.WorkspaceID, taskID)
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

		// 如果是全量 plan（没有 --target 参数）且没有任何变更，清除 drift 状态
		// 这表示当前状态与代码完全一致
		if !h.hasTargetParameter(&task) && !h.hasResourceChanges(&task) {
			log.Printf("[Drift] Clearing drift status for workspace %s after plan with no changes (task %d)", task.WorkspaceID, taskID)
			driftService := services.NewDriftCheckService(h.db)
			if err := driftService.ClearDriftOnFullApply(task.WorkspaceID); err != nil {
				log.Printf("[Drift] Failed to clear drift status for workspace %s: %v", task.WorkspaceID, err)
			} else {
				log.Printf("[Drift] Successfully cleared drift status for workspace %s", task.WorkspaceID)
			}
		}

	case models.TaskStatusPlannedAndFinished:
		// Plan_and_apply 任务完成但没有变更 - 发送 task_planned 通知
		log.Printf("[Notification] Triggering task_planned notification for task %d (status: planned_and_finished)", taskID)
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

		// 如果是全量 plan_and_apply（没有 --target 参数）且没有任何变更，清除 drift 状态
		// 这表示当前状态与代码完全一致
		if !h.hasTargetParameter(&task) {
			log.Printf("[Drift] Clearing drift status for workspace %s after plan_and_apply with no changes (task %d)", task.WorkspaceID, taskID)
			driftService := services.NewDriftCheckService(h.db)
			if err := driftService.ClearDriftOnFullApply(task.WorkspaceID); err != nil {
				log.Printf("[Drift] Failed to clear drift status for workspace %s: %v", task.WorkspaceID, err)
			} else {
				log.Printf("[Drift] Successfully cleared drift status for workspace %s", task.WorkspaceID)
			}
		} else {
			log.Printf("[Drift] Skipping drift clear for workspace %s - task %d has --target parameter", task.WorkspaceID, taskID)
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

func (h *RawAgentCCHandler) handleTaskFailed(agentConn *RawAgentConnection, payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("[Raw] Invalid task_id in task_failed message from agent %s", agentConn.AgentID)
		return
	}

	errorMsg := ""
	if err, ok := payload["error"].(string); ok {
		errorMsg = err
	}

	log.Printf("[Raw] Agent %s reported task %d failed: %s", agentConn.AgentID, uint(taskID), errorMsg)

	// 处理 drift_check 任务结果（失败情况）
	go h.processDriftCheckResult(uint(taskID))

	// 发送任务失败通知
	go h.sendTaskFailedNotification(uint(taskID))
}

// sendTaskFailedNotification 发送任务失败通知
func (h *RawAgentCCHandler) sendTaskFailedNotification(taskID uint) {
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

// handleLogStream handles real-time log stream from agent
func (h *RawAgentCCHandler) handleLogStream(agentConn *RawAgentConnection, payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("[Raw] Invalid task_id in log_stream message from agent %s", agentConn.AgentID)
		return
	}

	// Extract log message fields
	msgType, _ := payload["type"].(string)
	line, _ := payload["line"].(string)
	lineNum, _ := payload["line_num"].(float64)
	stage, _ := payload["stage"].(string)
	status, _ := payload["status"].(string)

	// Handle resource_status_update messages specially - persist to database
	if msgType == "resource_status_update" {
		h.handleResourceStatusUpdate(uint(taskID), line)
	}

	// Forward to OutputStreamManager for frontend WebSocket clients
	if h.streamManager != nil {
		stream := h.streamManager.GetOrCreate(uint(taskID))
		if stream != nil {
			outputMsg := services.OutputMessage{
				Type:      msgType,
				Line:      line,
				Timestamp: time.Now(), // Use current time
				LineNum:   int(lineNum),
				Stage:     stage,
				Status:    status,
			}
			stream.Broadcast(outputMsg) // Use Broadcast, not Publish
		}
	}

	// Forward via PG NOTIFY for cross-replica delivery.
	// Other replicas that have frontend WebSocket clients subscribed to this
	// task will receive this message and broadcast it to their local clients.
	forwardMsg := LogStreamForwardMessage{
		TaskID:    uint(taskID),
		Type:      msgType,
		Line:      line,
		LineNum:   int(lineNum),
		Stage:     stage,
		Status:    status,
		SourcePod: h.podName,
	}
	// PG NOTIFY has an 8000 byte payload limit. Truncate line if necessary.
	msgBytes, err := json.Marshal(forwardMsg)
	if err == nil && len(msgBytes) > 7500 {
		// Truncate the line to fit within PG NOTIFY limits
		excess := len(msgBytes) - 7500
		if len(forwardMsg.Line) > excess {
			forwardMsg.Line = forwardMsg.Line[:len(forwardMsg.Line)-excess] + "...(truncated)"
		}
	}
	if err := pgpubsub.Notify(h.db, LogStreamForwardChannel, forwardMsg); err != nil {
		log.Printf("[LogStream] Failed to forward log via PG NOTIFY for task %d: %v", uint(taskID), err)
	}
}

// handleResourceStatusUpdate updates resource status in database (Agent mode)
func (h *RawAgentCCHandler) handleResourceStatusUpdate(taskID uint, jsonData string) {
	// Parse JSON data from line field
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
		log.Printf("[Raw] Failed to parse resource_status_update JSON: %v", err)
		return
	}

	resourceAddress, _ := data["resource_address"].(string)
	applyStatus, _ := data["apply_status"].(string)
	action, _ := data["action"].(string)

	if resourceAddress == "" || applyStatus == "" {
		log.Printf("[Raw] Invalid resource_status_update data: address=%s, status=%s", resourceAddress, applyStatus)
		return
	}

	// Update database
	now := time.Now()
	updates := map[string]interface{}{
		"apply_status": applyStatus,
		"updated_at":   now,
	}

	if applyStatus == "applying" {
		updates["apply_started_at"] = now
	} else if applyStatus == "completed" {
		updates["apply_completed_at"] = now
	}

	if err := h.db.Model(&models.WorkspaceTaskResourceChange{}).
		Where("task_id = ? AND resource_address = ?", taskID, resourceAddress).
		Updates(updates).Error; err != nil {
		log.Printf("[Raw] Failed to update resource status in DB: %v", err)
		return
	}

	log.Printf("[Raw] Updated resource %s status to %s (task %d, action=%s)", resourceAddress, applyStatus, taskID, action)
}

// SendTaskToAgent sends a task to agent via C&C channel
func (h *RawAgentCCHandler) SendTaskToAgent(agentID string, taskID uint, workspaceID string, action string) error {
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

// IsAgentAvailable checks if agent can accept new tasks
// Agent capacity: 3 plan tasks + 1 plan_and_apply task
// Plan tasks are completely independent and can run concurrently with plan_and_apply
// drift_check tasks use plan slots (they only run terraform plan)
func (h *RawAgentCCHandler) IsAgentAvailable(agentID string, taskType models.TaskType) bool {
	h.mu.RLock()
	agentConn, ok := h.agents[agentID]
	h.mu.RUnlock()

	if !ok {
		log.Printf("[Raw] IsAgentAvailable: agent %s not found in connections", agentID)
		return false
	}

	agentConn.statusMu.RLock()
	defer agentConn.statusMu.RUnlock()

	// Plan tasks and drift_check tasks: check if plan slots are available (up to 3)
	// These tasks can run even when apply is running
	// drift_check is essentially a plan operation (terraform plan only)
	if taskType == models.TaskTypePlan || taskType == models.TaskTypeDriftCheck {
		available := agentConn.Status.PlanRunning < agentConn.Status.PlanLimit
		log.Printf("[Raw] IsAgentAvailable: agent %s, task_type=%s, plan_running=%d, plan_limit=%d, apply_running=%v, available=%v",
			agentID, taskType, agentConn.Status.PlanRunning, agentConn.Status.PlanLimit, agentConn.Status.ApplyRunning, available)
		return available
	}

	// Plan_and_apply tasks: check if apply slot is available (only 1)
	// Can run even when plan tasks are running
	if taskType == models.TaskTypePlanAndApply {
		available := !agentConn.Status.ApplyRunning
		log.Printf("[Raw] IsAgentAvailable: agent %s, task_type=plan_and_apply, plan_running=%d, apply_running=%v, available=%v",
			agentID, agentConn.Status.PlanRunning, agentConn.Status.ApplyRunning, available)
		return available
	}

	log.Printf("[Raw] IsAgentAvailable: agent %s, unknown task_type=%s", agentID, taskType)
	return false
}

// GetConnectedAgents returns list of connected agent IDs
func (h *RawAgentCCHandler) GetConnectedAgents() []string {
	h.mu.RLock()
	defer h.mu.RUnlock()

	agents := make([]string, 0, len(h.agents))
	for agentID := range h.agents {
		agents = append(agents, agentID)
	}
	return agents
}

// StartStandalone starts the handler as a standalone HTTP server
// If TLS_CERT_FILE and TLS_KEY_FILE are set, it starts with TLS.
func (h *RawAgentCCHandler) StartStandalone(port string) error {
	mux := http.NewServeMux()
	mux.Handle("/api/v1/agents/control", h)

	tlsCert := os.Getenv("TLS_CERT_FILE")
	tlsKey := os.Getenv("TLS_KEY_FILE")

	if tlsCert != "" && tlsKey != "" {
		log.Printf("[Raw] Starting standalone WebSocket server on port %s (TLS)", port)
		srv := &http.Server{
			Addr:    ":" + port,
			Handler: mux,
			TLSConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
		}
		return srv.ListenAndServeTLS(tlsCert, tlsKey)
	}

	log.Printf("[Raw] Starting standalone WebSocket server on port %s", port)
	return http.ListenAndServe(":"+port, mux)
}

// CancelTaskOnAgent sends a cancel_task command to the agent running the task
func (h *RawAgentCCHandler) CancelTaskOnAgent(agentID string, taskID uint) error {
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

	log.Printf("[Raw] Sending cancel_task command to agent %s for task %d", agentID, taskID)
	return h.sendMessage(agentConn, msg)
}

// timePtr 返回时间指针
func timePtr(t time.Time) *time.Time {
	return &t
}

// cleanupAgentTasks cleans up tasks that were running on the disconnected agent
func (h *RawAgentCCHandler) cleanupAgentTasks(agentID string) {
	log.Printf("[Raw] Cleaning up tasks for disconnected agent %s", agentID)

	// Find all running tasks assigned to this agent
	var runningTasks []models.WorkspaceTask
	if err := h.db.Where("agent_id = ? AND status = ?", agentID, models.TaskStatusRunning).
		Find(&runningTasks).Error; err != nil {
		log.Printf("[Raw] Error querying tasks for agent %s: %v", agentID, err)
		return
	}

	if len(runningTasks) == 0 {
		log.Printf("[Raw] No running tasks found for agent %s", agentID)
		return
	}

	log.Printf("[Raw] Found %d running task(s) for disconnected agent %s", len(runningTasks), agentID)

	// Mark each task as failed
	for _, task := range runningTasks {
		// 保存当前的日志输出（如果有）
		if h.streamManager != nil {
			stream := h.streamManager.Get(task.ID)
			if stream != nil {
				bufferedLogs := stream.GetBufferedLogs()
				if bufferedLogs != "" {
					// 根据任务类型保存到对应字段
					if task.TaskType == models.TaskTypePlan || task.TaskType == models.TaskTypePlanAndApply {
						task.PlanOutput = bufferedLogs
					} else if task.TaskType == models.TaskTypeApply {
						task.ApplyOutput = bufferedLogs
					}
					log.Printf("[Raw] Saved %d bytes of logs for task %d before marking as failed", len(bufferedLogs), task.ID)
				}
				// 关闭 stream
				h.streamManager.Close(task.ID)
			}
		}

		// 更新任务状态
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = fmt.Sprintf("Agent %s disconnected unexpectedly", agentID)
		task.CompletedAt = timePtr(time.Now())

		if err := h.db.Save(&task).Error; err != nil {
			log.Printf("[Raw] Failed to update task %d: %v", task.ID, err)
			continue
		}

		log.Printf("[Raw] Marked task %d as failed due to agent %s disconnect", task.ID, agentID)
	}

	log.Printf("[Raw] Cleanup completed for agent %s: %d task(s) marked as failed", agentID, len(runningTasks))
}

// SetMetricsHub sets the metrics hub for broadcasting agent metrics
func (h *RawAgentCCHandler) SetMetricsHub(hub *websocket.AgentMetricsHub) {
	h.metricsHub = hub
	log.Println("[Raw] Agent Metrics Hub configured")
}

// processDriftCheckResult 处理 drift_check 任务结果
func (h *RawAgentCCHandler) processDriftCheckResult(taskID uint) {
	driftService := services.NewDriftCheckService(h.db)
	if err := driftService.ProcessDriftCheckResultByTaskID(taskID); err != nil {
		log.Printf("[Raw] Failed to process drift check result for task %d: %v", taskID, err)
	} else {
		log.Printf("[Raw] Successfully processed drift check result for task %d", taskID)
	}
}

// hasTargetParameter 检查任务是否使用了 --target 参数
// 通过检查任务的变量快照中的 TF_CLI_ARGS 环境变量来判断
func (h *RawAgentCCHandler) hasTargetParameter(task *models.WorkspaceTask) bool {
	// 1. 首先检查任务的变量快照（如果有）
	if task.SnapshotVariables != nil && len(task.SnapshotVariables) > 0 {
		// 尝试从 _array 格式中提取变量引用
		if arrayData, hasArray := task.SnapshotVariables["_array"]; hasArray {
			if variables, ok := arrayData.([]interface{}); ok {
				// 收集所有变量 ID
				var variableIDs []string
				for _, v := range variables {
					if varMap, ok := v.(map[string]interface{}); ok {
						if varID, ok := varMap["variable_id"].(string); ok {
							variableIDs = append(variableIDs, varID)
						}
					}
				}
				// 查询这些变量中是否有 TF_CLI_ARGS
				if len(variableIDs) > 0 {
					var tfCliArgsVar models.WorkspaceVariable
					if err := h.db.Where("variable_id IN ? AND key = ?", variableIDs, "TF_CLI_ARGS").
						First(&tfCliArgsVar).Error; err == nil {
						// 找到了 TF_CLI_ARGS，检查是否包含 --target
						if strings.Contains(tfCliArgsVar.Value, "--target") || strings.Contains(tfCliArgsVar.Value, "-target") {
							return true
						}
					}
				}
			}
		}
		// 如果有快照但没有找到包含 --target 的 TF_CLI_ARGS，说明没有使用 --target
		return false
	}

	// 2. 如果没有变量快照，检查 workspace 当前的变量
	var variable models.WorkspaceVariable
	err := h.db.Where("workspace_id = ? AND key = ? AND is_deleted = ?",
		task.WorkspaceID, "TF_CLI_ARGS", false).First(&variable).Error
	if err != nil {
		// 没有找到 TF_CLI_ARGS 变量，说明没有使用 --target
		return false
	}

	// 检查变量值中是否包含 --target 或 -target
	if strings.Contains(variable.Value, "--target") || strings.Contains(variable.Value, "-target") {
		return true
	}

	return false
}

// hasResourceChanges 检查任务是否有资源变更
func (h *RawAgentCCHandler) hasResourceChanges(task *models.WorkspaceTask) bool {
	// 检查 workspace_task_resource_changes 表中是否有该任务的变更记录
	var count int64
	if err := h.db.Model(&models.WorkspaceTaskResourceChange{}).
		Where("task_id = ? AND action != ?", task.ID, "no-op").
		Count(&count).Error; err != nil {
		log.Printf("[Drift] Failed to check resource changes for task %d: %v", task.ID, err)
		return true // 保守起见，假设有变更
	}
	return count > 0
}

// BroadcastCredentialsRefresh sends refresh_credentials command to all agents in a pool
func (h *RawAgentCCHandler) BroadcastCredentialsRefresh(poolID string) error {
	log.Printf("[Raw] Broadcasting credentials refresh to pool %s", poolID)

	// Get all agents in this pool
	var agents []models.Agent
	if err := h.db.Where("pool_id = ? AND status = ?", poolID, "online").Find(&agents).Error; err != nil {
		return fmt.Errorf("failed to query agents: %w", err)
	}

	if len(agents) == 0 {
		log.Printf("[Raw] No online agents found in pool %s", poolID)
		return nil
	}

	log.Printf("[Raw] Found %d online agent(s) in pool %s", len(agents), poolID)

	// Send refresh command to each connected agent
	successCount := 0
	for _, agent := range agents {
		h.mu.RLock()
		agentConn, ok := h.agents[agent.AgentID]
		h.mu.RUnlock()

		if !ok {
			log.Printf("[Raw] Agent %s not connected to C&C, skipping", agent.AgentID)
			continue
		}

		msg := CCMessage{
			Type: "refresh_credentials",
			Payload: map[string]interface{}{
				"pool_id":   poolID,
				"timestamp": time.Now().Unix(),
			},
		}

		if err := h.sendMessage(agentConn, msg); err != nil {
			log.Printf("[Raw] Failed to send refresh_credentials to agent %s: %v", agent.AgentID, err)
		} else {
			log.Printf("[Raw] Sent refresh_credentials to agent %s", agent.AgentID)
			successCount++
		}
	}

	log.Printf("[Raw] Credentials refresh broadcast completed: %d/%d agents notified", successCount, len(agents))
	return nil
}

// SetupTaskDispatchListener subscribes to the TaskDispatchChannel so that
// cross-replica task dispatch messages are handled. When the leader publishes
// a TaskDispatchMessage via PG NOTIFY, every replica receives it. Each replica
// checks whether the target agent is connected locally and, if so, delivers
// the task over its WebSocket connection.
func (h *RawAgentCCHandler) SetupTaskDispatchListener(pubsub *pgpubsub.PubSub) {
	podName := os.Getenv("POD_NAME")
	if podName == "" {
		podName, _ = os.Hostname()
	}

	pubsub.Subscribe(TaskDispatchChannel, func(payload string) {
		var msg TaskDispatchMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			log.Printf("[TaskDispatch] Failed to unmarshal task dispatch message: %v", err)
			return
		}

		// Skip messages that originated from this pod to avoid double-delivery.
		if msg.SourcePod == podName {
			return
		}

		// Check if the target agent is connected to this replica.
		h.mu.RLock()
		_, connected := h.agents[msg.AgentID]
		h.mu.RUnlock()

		if !connected {
			// Agent is not on this replica; nothing to do.
			return
		}

		log.Printf("[TaskDispatch] Received cross-replica dispatch: task=%d agent=%s action=%s from pod=%s",
			msg.TaskID, msg.AgentID, msg.Action, msg.SourcePod)

		if err := h.SendTaskToAgent(msg.AgentID, msg.TaskID, msg.WorkspaceID, msg.Action); err != nil {
			log.Printf("[TaskDispatch] Failed to deliver task %d to agent %s on this replica: %v",
				msg.TaskID, msg.AgentID, err)
		} else {
			log.Printf("[TaskDispatch] Successfully delivered task %d to agent %s on this replica",
				msg.TaskID, msg.AgentID)
		}
	})

	log.Printf("[TaskDispatch] Listener configured on pod %s", podName)
}

// SetupLogStreamListener subscribes to the LogStreamForwardChannel so that
// log lines forwarded from other replicas (via PG NOTIFY) are broadcast to
// local frontend WebSocket subscribers. This enables real-time log streaming
// in a multi-replica deployment where the agent WebSocket connects to one
// replica but the frontend may be connected to another.
func (h *RawAgentCCHandler) SetupLogStreamListener(pubsub *pgpubsub.PubSub) {
	pubsub.Subscribe(LogStreamForwardChannel, func(payload string) {
		var msg LogStreamForwardMessage
		if err := json.Unmarshal([]byte(payload), &msg); err != nil {
			log.Printf("[LogStream] Failed to unmarshal forwarded log message: %v", err)
			return
		}

		// Skip messages that originated from this pod — we already broadcast locally.
		if msg.SourcePod == h.podName {
			return
		}

		// Broadcast to local OutputStreamManager for any frontend clients on this replica.
		if h.streamManager != nil {
			stream := h.streamManager.GetOrCreate(msg.TaskID)
			if stream != nil {
				outputMsg := services.OutputMessage{
					Type:      msg.Type,
					Line:      msg.Line,
					Timestamp: time.Now(),
					LineNum:   msg.LineNum,
					Stage:     msg.Stage,
					Status:    msg.Status,
				}
				stream.Broadcast(outputMsg)
			}
		}
	})

	log.Printf("[LogStream] Cross-replica log stream listener configured on pod %s", h.podName)
}
