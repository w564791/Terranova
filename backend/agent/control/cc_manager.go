package control

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"iac-platform/services"

	"github.com/gorilla/websocket"
)

// CCManager manages C&C (Command & Control) WebSocket connection
type CCManager struct {
	AgentID       string
	PoolID        string
	protocol      string // http or https
	apiClient     *services.AgentAPIClient
	executor      *services.TerraformExecutor
	streamManager *services.OutputStreamManager

	conn      *websocket.Conn
	connMutex sync.Mutex
	writeChan chan CCMessage // Channel for serializing writes

	// Agent status
	planRunning  int
	planLimit    int
	applyRunning bool
	currentTasks []uint
	statusMutex  sync.RWMutex

	// Task cancellation support
	taskContexts map[uint]context.CancelFunc
	taskMutex    sync.RWMutex

	// Control
	ctx          context.Context
	cancel       context.CancelFunc
	shutdownChan chan struct{}
}

// CCMessage represents a C&C message
type CCMessage struct {
	Type    string                 `json:"type"`
	Payload map[string]interface{} `json:"payload"`
}

// NewCCManager creates a new C&C manager
func NewCCManager(
	apiClient *services.AgentAPIClient,
	executor *services.TerraformExecutor,
	streamManager *services.OutputStreamManager,
	protocol string,
) *CCManager {
	ctx, cancel := context.WithCancel(context.Background())

	return &CCManager{
		apiClient:     apiClient,
		executor:      executor,
		streamManager: streamManager,
		protocol:      protocol,
		planLimit:     3, // Default
		ctx:           ctx,
		cancel:        cancel,
		shutdownChan:  make(chan struct{}),
		writeChan:     make(chan CCMessage, 10), // Buffered channel for writes
	}
}

// Connect establishes C&C WebSocket connection with retry logic
func (m *CCManager) Connect() error {
	return m.connectWithRetry()
}

// connectWithRetry attempts to connect with exponential backoff
func (m *CCManager) connectWithRetry() error {
	backoff := 2 * time.Second
	maxBackoff := 60 * time.Second
	attempt := 0

	for {
		attempt++
		log.Printf("[Connect] Connection attempt #%d (backoff: %v)", attempt, backoff)

		err := m.tryConnect()
		if err == nil {
			log.Printf("[Connect] Successfully connected on attempt #%d", attempt)
			return nil
		}

		log.Printf("[Connect] Connection attempt #%d failed: %v", attempt, err)

		// Check if context is cancelled
		select {
		case <-m.ctx.Done():
			return fmt.Errorf("connection cancelled: %w", m.ctx.Err())
		case <-time.After(backoff):
			// Continue to next attempt
		}

		// Exponential backoff: 2s -> 4s -> 8s -> 16s -> 60s (capped)
		backoff *= 2
		if backoff > maxBackoff {
			backoff = maxBackoff
		}
	}
}

// tryConnect attempts a single connection
func (m *CCManager) tryConnect() error {
	// Get API endpoint (domain or IP, without port)
	apiEndpoint := os.Getenv("IAC_API_ENDPOINT")
	if apiEndpoint == "" {
		return fmt.Errorf("IAC_API_ENDPOINT not set")
	}

	// Get API server port (default: 8080)
	apiPort := 8080
	if serverPort := os.Getenv("SERVER_PORT"); serverPort != "" {
		if _, err := fmt.Sscanf(serverPort, "%d", &apiPort); err != nil {
			log.Printf("[Connect] Invalid SERVER_PORT '%s', using default: 8080", serverPort)
			apiPort = 8080
		}
	}

	// Get CC server port
	// Priority: CC_SERVER_PORT > (SERVER_PORT + 10)
	var ccPort int
	if ccServerPort := os.Getenv("CC_SERVER_PORT"); ccServerPort != "" {
		if _, err := fmt.Sscanf(ccServerPort, "%d", &ccPort); err != nil {
			log.Printf("[Connect] Invalid CC_SERVER_PORT '%s', using SERVER_PORT + 10", ccServerPort)
			ccPort = apiPort + 10
		}
	} else {
		// Default: API port + 10
		ccPort = apiPort + 10
	}

	// Build WebSocket URL
	wsScheme := "ws"
	if m.protocol == "https" {
		wsScheme = "wss"
	}

	ccURL := &url.URL{
		Scheme: wsScheme,
		Host:   fmt.Sprintf("%s:%d", apiEndpoint, ccPort),
		Path:   "/api/v1/agents/control",
	}
	q := ccURL.Query()
	q.Set("agent_id", m.AgentID)
	ccURL.RawQuery = q.Encode()
	ccEndpoint := ccURL.String()

	log.Printf("[Connect] Configuration:")
	log.Printf("[Connect]   - API Endpoint: %s", apiEndpoint)
	log.Printf("[Connect]   - API Port: %d", apiPort)
	log.Printf("[Connect]   - CC Port: %d", ccPort)
	log.Printf("[Connect]   - Protocol: %s", m.protocol)
	log.Printf("[Connect] WebSocket URL: %s", ccEndpoint)

	// Connect with authentication header
	log.Printf("[Connect] Connecting to C&C channel: %s", ccEndpoint)

	// Add Authorization header using http.Header
	token := os.Getenv("IAC_AGENT_TOKEN")
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+token)

	// Create a NEW dialer instance with pointer
	dialer := &websocket.Dialer{
		EnableCompression: false,
	}

	log.Printf("[Connect] Dialing WebSocket...")
	conn, _, err := dialer.Dial(ccEndpoint, headers)
	if err != nil {
		return fmt.Errorf("failed to connect to C&C channel: %w", err)
	}

	m.connMutex.Lock()
	m.conn = conn
	m.connMutex.Unlock()

	log.Printf("[Connect] WebSocket connected, starting handlers...")

	// Start write handler
	go m.writeLoop()

	// Start message handler
	go m.handleMessages()

	log.Printf("[Connect] Handlers started, Connect() returning")
	return nil
}

// HeartbeatLoop sends periodic heartbeat messages
func (m *CCManager) HeartbeatLoop() {
	log.Printf("[HeartbeatLoop] Starting heartbeat loop...")

	// Send first heartbeat immediately
	m.sendHeartbeat()

	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[HeartbeatLoop] Context done, exiting")
			return
		case <-ticker.C:
			m.sendHeartbeat()
		}
	}
}

// sendHeartbeat sends heartbeat message
func (m *CCManager) sendHeartbeat() {
	// Get CPU and memory usage
	cpuUsage, memUsage := m.getSystemMetrics()

	m.statusMutex.RLock()
	status := map[string]interface{}{
		"plan_running":  m.planRunning,
		"plan_limit":    m.planLimit,
		"apply_running": m.applyRunning,
		"current_tasks": m.currentTasks,
		"cpu_usage":     cpuUsage,
		"mem_usage":     memUsage,
		"status":        "ok",
		"timestamp":     time.Now().Unix(),
	}
	m.statusMutex.RUnlock()

	msg := CCMessage{
		Type:    "heartbeat",
		Payload: status,
	}

	// Write directly instead of using channel
	m.connMutex.Lock()
	defer m.connMutex.Unlock()

	if m.conn != nil {
		// Use WriteMessage with raw bytes to ensure data is sent
		data, err := json.Marshal(msg)
		if err != nil {
			log.Printf("[Agent->Server] Failed to marshal heartbeat: %v", err)
			return
		}

		err = m.conn.WriteMessage(websocket.TextMessage, data)
		if err != nil {
			log.Printf("[Agent->Server] Failed to write heartbeat: %v", err)
		}
		// Heartbeat sent silently - no log to reduce noise
	} else {
		log.Printf("[Agent->Server] Connection is nil, cannot send heartbeat")
	}
}

// handleMessages handles incoming messages from server
func (m *CCManager) handleMessages() {
	defer func() {
		m.connMutex.Lock()
		if m.conn != nil {
			m.conn.Close()
		}
		m.connMutex.Unlock()

		log.Printf("C&C connection closed")

		// Attempt reconnect
		go m.reconnect()
	}()

	for {
		select {
		case <-m.ctx.Done():
			return
		default:
			var msg CCMessage

			// Get connection without holding lock during read
			m.connMutex.Lock()
			conn := m.conn
			m.connMutex.Unlock()

			if conn == nil {
				return
			}

			log.Printf("[handleMessages] Calling ReadJSON...")
			err := conn.ReadJSON(&msg)
			log.Printf("[handleMessages] ReadJSON returned, err=%v", err)

			if err != nil {
				if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseAbnormalClosure) {
					log.Printf("WebSocket error: %v", err)
				}
				return
			}

			// Handle message
			m.handleMessage(msg)
		}
	}
}

// handleMessage handles a single message
func (m *CCManager) handleMessage(msg CCMessage) {
	log.Printf("[Server->Agent] Received message type: %s", msg.Type)

	switch msg.Type {
	case "run_task":
		m.handleRunTask(msg.Payload)
	case "cancel_task":
		m.handleCancelTask(msg.Payload)
	case "start_realtime_stream":
		m.handleStartStream(msg.Payload)
	case "stop_realtime_stream":
		m.handleStopStream(msg.Payload)
	case "refresh_credentials":
		m.handleRefreshCredentials(msg.Payload)
	default:
		log.Printf("[Server->Agent] Unknown message type: %s", msg.Type)
	}
}

// handleRunTask handles run_task command
func (m *CCManager) handleRunTask(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("Invalid task_id in run_task message")
		return
	}

	workspaceID, _ := payload["workspace_id"].(string)
	action, _ := payload["action"].(string)

	log.Printf("Received task %d (workspace: %s, action: %s)", uint(taskID), workspaceID, action)

	// Execute task in a goroutine
	go m.executeTask(uint(taskID), workspaceID, action)
}

// executeTask executes a task received from the server
func (m *CCManager) executeTask(taskID uint, workspaceID string, action string) {
	log.Printf("[Agent] Starting execution of task %d (action: %s)", taskID, action)

	// Add panic recovery to prevent agent crash
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[Agent] PANIC recovered in task %d: %v", taskID, r)
			// Send task failed notification
			errorMsg := fmt.Sprintf("Task panicked: %v", r)
			m.sendTaskFailedNotification(taskID, errorMsg)

			// Update task status to failed via API
			remoteAccessor := services.NewRemoteDataAccessor(m.apiClient)
			if err := remoteAccessor.LoadTaskData(taskID); err == nil {
				if task, err := remoteAccessor.GetTask(taskID); err == nil {
					task.Status = "failed"
					task.ErrorMessage = errorMsg
					completedAt := time.Now()
					task.CompletedAt = &completedAt
					if err := remoteAccessor.UpdateTask(task); err != nil {
						log.Printf("[Agent] Failed to update task status after panic: %v", err)
					}
				}
			}
		}
	}()

	// Start real-time log streaming for this task
	m.handleStartStream(map[string]interface{}{
		"task_id": float64(taskID),
	})

	// Load task data from server using RemoteDataAccessor
	remoteAccessor := services.NewRemoteDataAccessor(m.apiClient)
	remoteAccessor.SetStreamManager(m.streamManager) // 设置 streamManager 以支持 WebSocket 更新
	if err := remoteAccessor.LoadTaskData(taskID); err != nil {
		log.Printf("[Agent] Failed to load task %d data: %v", taskID, err)
		m.sendTaskFailedNotification(taskID, fmt.Sprintf("Failed to load task data: %v", err))
		return
	}

	// Get task object from loaded data
	task, err := remoteAccessor.GetTask(taskID)
	if err != nil {
		log.Printf("[Agent] Failed to get task %d object: %v", taskID, err)
		m.sendTaskFailedNotification(taskID, fmt.Sprintf("Failed to get task object: %v", err))
		return
	}

	log.Printf("[Agent] Loaded task %d data successfully", taskID)

	// Update agent status based on task type
	// Plan+apply tasks occupy the apply slot, not plan slots
	m.statusMutex.Lock()
	if task.TaskType == "plan_and_apply" {
		// Plan+apply tasks use the apply slot
		m.applyRunning = true
	} else if task.TaskType == "plan" {
		// Pure plan tasks use plan slots
		m.planRunning++
	}
	m.currentTasks = append(m.currentTasks, taskID)
	m.statusMutex.Unlock()

	log.Printf("[Agent] Task %d started: type=%s, plan_running=%d, apply_running=%v, current_tasks=%v",
		taskID, task.TaskType, m.planRunning, m.applyRunning, m.currentTasks)

	// Ensure status is updated when task completes
	defer func() {
		m.statusMutex.Lock()
		if task.TaskType == "plan_and_apply" {
			m.applyRunning = false
		} else if task.TaskType == "plan" {
			m.planRunning--
		}
		// Remove task from current tasks
		for i, tid := range m.currentTasks {
			if tid == taskID {
				m.currentTasks = append(m.currentTasks[:i], m.currentTasks[i+1:]...)
				break
			}
		}
		m.statusMutex.Unlock()

		log.Printf("[Agent]  Task %d completed: type=%s, plan_running=%d, apply_running=%v, current_tasks=%v",
			taskID, task.TaskType, m.planRunning, m.applyRunning, m.currentTasks)

		// Remove task context from map
		m.taskMutex.Lock()
		delete(m.taskContexts, taskID)
		m.taskMutex.Unlock()
	}()

	// Create cancellable context with timeout
	ctx, cancel := context.WithTimeout(m.ctx, 60*time.Minute)
	defer cancel()

	// Store cancel function for this task
	m.taskMutex.Lock()
	if m.taskContexts == nil {
		m.taskContexts = make(map[uint]context.CancelFunc)
	}
	m.taskContexts[taskID] = cancel
	m.taskMutex.Unlock()

	// Create a new TerraformExecutor with the loaded RemoteDataAccessor
	// This ensures the executor uses the same data accessor instance with loaded data
	taskExecutor := services.NewTerraformExecutorWithAccessor(remoteAccessor, m.streamManager)

	// Execute the task based on action
	var execErr error
	if action == "apply" {
		log.Printf("[Agent] Executing apply for task %d", taskID)
		execErr = taskExecutor.ExecuteApply(ctx, task)
	} else {
		log.Printf("[Agent] Executing plan for task %d", taskID)
		execErr = taskExecutor.ExecutePlan(ctx, task)
	}

	// Send completion notification
	if execErr != nil {
		log.Printf("[Agent] Task %d failed: %v", taskID, execErr)
		m.sendTaskFailedNotification(taskID, execErr.Error())
	} else {
		log.Printf("[Agent] Task %d completed successfully", taskID)
		m.sendTaskCompletedNotification(taskID)
	}
}

// sendTaskCompletedNotification sends task completion notification to server
func (m *CCManager) sendTaskCompletedNotification(taskID uint) {
	msg := CCMessage{
		Type: "task_completed",
		Payload: map[string]interface{}{
			"task_id": taskID,
		},
	}
	log.Printf("[Agent->Server] Sending task_completed for task %d", taskID)
	if err := m.sendMessage(msg); err != nil {
		log.Printf("[Agent] Failed to send task_completed notification for task %d: %v", taskID, err)
	}
}

// sendTaskFailedNotification sends task failure notification to server
func (m *CCManager) sendTaskFailedNotification(taskID uint, errorMsg string) {
	msg := CCMessage{
		Type: "task_failed",
		Payload: map[string]interface{}{
			"task_id": taskID,
			"error":   errorMsg,
		},
	}
	log.Printf("[Agent->Server] Sending task_failed for task %d: %s", taskID, errorMsg)
	if err := m.sendMessage(msg); err != nil {
		log.Printf("[Agent] Failed to send task_failed notification for task %d: %v", taskID, err)
	}
}

// handleCancelTask handles cancel_task command
func (m *CCManager) handleCancelTask(payload map[string]interface{}) {
	log.Printf("========================================")
	log.Printf("[CANCEL] Received cancel_task message from server")
	log.Printf("[CANCEL] Payload: %+v", payload)
	log.Printf("========================================")

	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("[CANCEL] ERROR: Invalid task_id in cancel_task message, payload: %+v", payload)
		return
	}

	tid := uint(taskID)
	log.Printf("[CANCEL]  Parsed task_id: %d", tid)

	// Log current running tasks
	m.taskMutex.RLock()
	log.Printf("[CANCEL] Current taskContexts map has %d entries", len(m.taskContexts))
	for taskID := range m.taskContexts {
		log.Printf("[CANCEL]   - Task %d is in taskContexts", taskID)
	}
	cancelFunc, exists := m.taskContexts[tid]
	m.taskMutex.RUnlock()

	if !exists {
		log.Printf("[CANCEL] ❌ Task %d NOT FOUND in running tasks (may have already completed)", tid)
		log.Printf("[CANCEL] This means the task is not currently executing or has finished")
		return
	}

	log.Printf("[CANCEL]  Task %d FOUND in running tasks, proceeding to cancel", tid)

	// Cancel the task context - this will cause the Terraform execution to stop
	log.Printf("[CANCEL] Calling cancelFunc() for task %d...", tid)
	cancelFunc()
	log.Printf("[CANCEL]  Successfully cancelled task %d execution ", tid)
	log.Printf("[CANCEL] The Terraform process should now terminate")
	log.Printf("========================================")
}

// handleStartStream handles start_realtime_stream command
func (m *CCManager) handleStartStream(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("Invalid task_id in start_realtime_stream message")
		return
	}

	log.Printf("[Agent] Starting real-time log stream for task %d", uint(taskID))

	// Get or create output stream for this task
	stream := m.streamManager.GetOrCreate(uint(taskID))
	if stream == nil {
		log.Printf("[Agent] Failed to create stream for task %d", uint(taskID))
		return
	}

	// Subscribe to the stream and forward logs to server via WebSocket
	clientID := fmt.Sprintf("agent-%s-task-%d", m.AgentID, uint(taskID))
	client, _ := stream.Subscribe(clientID)
	if client == nil {
		log.Printf("[Agent] Failed to subscribe to stream for task %d", uint(taskID))
		return
	}

	// Start goroutine to forward logs to server
	go m.forwardLogsToServer(uint(taskID), client, clientID, stream)

	log.Printf("[Agent] Real-time log stream started for task %d", uint(taskID))
}

// forwardLogsToServer forwards logs from local stream to server via WebSocket
func (m *CCManager) forwardLogsToServer(taskID uint, client *services.Client, clientID string, stream *services.OutputStream) {
	defer stream.Unsubscribe(clientID)

	log.Printf("[Agent] Starting log forwarding for task %d", taskID)

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[Agent] Context done, stopping log forwarding for task %d", taskID)
			return
		case msg, ok := <-client.Channel:
			if !ok {
				log.Printf("[Agent] Stream closed for task %d", taskID)
				return
			}

			// Forward log message to server
			ccMsg := CCMessage{
				Type: "log_stream",
				Payload: map[string]interface{}{
					"task_id":   taskID,
					"type":      msg.Type,
					"line":      msg.Line,
					"timestamp": msg.Timestamp,
					"line_num":  msg.LineNum,
					"stage":     msg.Stage,
					"status":    msg.Status,
				},
			}

			if err := m.sendMessage(ccMsg); err != nil {
				log.Printf("[Agent] Failed to forward log for task %d: %v", taskID, err)
				// Continue trying to send other logs
			}
		}
	}
}

// handleStopStream handles stop_realtime_stream command
func (m *CCManager) handleStopStream(payload map[string]interface{}) {
	taskID, ok := payload["task_id"].(float64)
	if !ok {
		log.Printf("Invalid task_id in stop_realtime_stream message")
		return
	}

	log.Printf("[Agent] Stopping real-time log stream for task %d", uint(taskID))

	// The forwardLogsToServer goroutine will stop when the stream is closed
	// or when context is cancelled. We don't need to do anything special here.
	// The stream will be closed automatically when the task completes.
}

// writeLoop handles all writes in a single goroutine
func (m *CCManager) writeLoop() {
	log.Printf("[writeLoop] Starting write loop")
	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[writeLoop] Context done, exiting")
			return
		case msg := <-m.writeChan:
			// Debug logging - only log non-heartbeat and non-log_stream messages to reduce noise
			if msg.Type != "heartbeat" && msg.Type != "log_stream" {
				log.Printf("[writeLoop] Got message from channel, type=%s", msg.Type)
			}
			m.connMutex.Lock()
			if m.conn != nil {
				err := m.conn.WriteJSON(msg)
				if err != nil {
					log.Printf("[writeLoop] Failed to send message: %v", err)
					// If write fails, close the connection to trigger reconnect
					m.conn.Close()
					m.conn = nil
				}
			} else {
				log.Printf("[writeLoop] Connection is nil, cannot write")
			}
			m.connMutex.Unlock()
		}
	}
}

// sendMessage sends a message to server via channel
func (m *CCManager) sendMessage(msg CCMessage) error {
	select {
	case m.writeChan <- msg:
		return nil
	case <-time.After(5 * time.Second):
		return fmt.Errorf("write channel full or blocked")
	}
}

// reconnect attempts to reconnect to C&C channel with exponential backoff
func (m *CCManager) reconnect() {
	backoff := 2 * time.Second
	maxBackoff := 60 * time.Second
	attempt := 0

	for {
		select {
		case <-m.ctx.Done():
			log.Printf("[Reconnect] Context cancelled, stopping reconnection attempts")
			return
		case <-time.After(backoff):
			attempt++
			log.Printf("[Reconnect] Reconnection attempt #%d (backoff: %v)", attempt, backoff)

			if err := m.tryConnect(); err != nil {
				log.Printf("[Reconnect] Attempt #%d failed: %v", attempt, err)

				// Exponential backoff: 2s -> 4s -> 8s -> 16s -> 60s (capped)
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
					log.Printf("[Reconnect] Backoff capped at %v, will retry every %v", maxBackoff, maxBackoff)
				}
			} else {
				log.Printf("[Reconnect] Successfully reconnected on attempt #%d", attempt)
				return
			}
		}
	}
}

// WaitForShutdown waits for shutdown signal
func (m *CCManager) WaitForShutdown() {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
	case <-m.shutdownChan:
		log.Printf("Shutdown requested")
	}

	// Graceful shutdown
	log.Printf("Shutting down agent...")
	m.cancel()

	// Close connection
	m.connMutex.Lock()
	if m.conn != nil {
		m.conn.Close()
	}
	m.connMutex.Unlock()

	log.Printf("Agent shutdown complete")
}

// Shutdown triggers graceful shutdown
func (m *CCManager) Shutdown() {
	close(m.shutdownChan)
}

// handleRefreshCredentials handles refresh_credentials command from server
func (m *CCManager) handleRefreshCredentials(payload map[string]interface{}) {
	log.Printf("[Server->Agent] Received refresh_credentials command")

	// Trigger credentials refresh in a goroutine to avoid blocking
	go func() {
		log.Printf("[HCP Credentials] Starting immediate refresh triggered by server...")

		// Call API to get latest secrets
		respBody, err := m.apiClient.GetPoolSecrets()
		if err != nil {
			log.Printf("[HCP Credentials] Failed to fetch pool secrets: %v", err)
			return
		}

		// Extract credentials from response
		credentials, ok := respBody["credentials"].(map[string]interface{})
		if !ok {
			log.Printf("[HCP Credentials] Invalid response format")
			return
		}

		// Generate credentials file
		if err := m.generateCredentialsFile(credentials); err != nil {
			log.Printf("[HCP Credentials] Failed to generate credentials file: %v", err)
			return
		}

		log.Printf("[HCP Credentials] Credentials refreshed successfully (triggered by server)")

		// Send acknowledgment back to server
		ackMsg := CCMessage{
			Type: "credentials_refreshed",
			Payload: map[string]interface{}{
				"agent_id":  m.AgentID,
				"pool_id":   m.PoolID,
				"timestamp": time.Now().Unix(),
				"count":     len(credentials),
			},
		}
		if err := m.sendMessage(ackMsg); err != nil {
			log.Printf("[Agent->Server] Failed to send credentials_refreshed acknowledgment: %v", err)
		}
	}()
}

// getSystemMetrics gets current CPU and memory usage
func (m *CCManager) getSystemMetrics() (cpuUsage float64, memUsage float64) {
	// Simple implementation using /proc/stat for CPU and /proc/meminfo for memory
	// This works on Linux without external dependencies

	// Get memory usage
	memUsage = m.getMemoryUsage()

	// Get CPU usage (simplified - returns approximate usage)
	cpuUsage = m.getCPUUsage()

	return cpuUsage, memUsage
}

// getMemoryUsage gets memory usage percentage
func (m *CCManager) getMemoryUsage() float64 {
	// Read /proc/meminfo
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		log.Printf("[Metrics] Failed to read /proc/meminfo: %v", err)
		return 0.0
	}

	var memTotal, memAvailable float64
	var memTotalFound, memAvailableFound bool

	// Parse line by line
	lines := string(data)
	for i := 0; i < len(lines); {
		// Find end of line
		end := i
		for end < len(lines) && lines[end] != '\n' {
			end++
		}

		line := lines[i:end]

		// Parse MemTotal
		if !memTotalFound {
			var value float64
			var unit string
			if n, _ := fmt.Sscanf(line, "MemTotal: %f %s", &value, &unit); n >= 1 {
				memTotal = value
				memTotalFound = true
			}
		}

		// Parse MemAvailable
		if !memAvailableFound {
			var value float64
			var unit string
			if n, _ := fmt.Sscanf(line, "MemAvailable: %f %s", &value, &unit); n >= 1 {
				memAvailable = value
				memAvailableFound = true
			}
		}

		// Move to next line
		i = end + 1

		// Break early if we found both values
		if memTotalFound && memAvailableFound {
			break
		}
	}

	if memTotal > 0 && memAvailable > 0 {
		used := memTotal - memAvailable
		percentage := (used / memTotal) * 100.0
		log.Printf("[Metrics] Memory: Total=%.0f KB, Available=%.0f KB, Used=%.0f KB, Usage=%.2f%%",
			memTotal, memAvailable, used, percentage)
		return percentage
	}

	log.Printf("[Metrics] Failed to parse memory info: MemTotal=%v (found=%v), MemAvailable=%v (found=%v)",
		memTotal, memTotalFound, memAvailable, memAvailableFound)
	return 0.0
}

// getCPUUsage gets CPU usage percentage (simplified)
func (m *CCManager) getCPUUsage() float64 {
	// For simplicity, return a mock value between 10-50%
	// In production, you would implement proper CPU monitoring
	// using /proc/stat or gopsutil library

	// Simple mock: return random-ish value based on current tasks
	m.statusMutex.RLock()
	taskCount := len(m.currentTasks)
	m.statusMutex.RUnlock()

	// Base CPU: 10% + 15% per task
	baseCPU := 10.0
	taskCPU := float64(taskCount) * 15.0

	return baseCPU + taskCPU
}

// generateCredentialsFile generates the credentials.tfrc.json file
func (m *CCManager) generateCredentialsFile(credentials map[string]interface{}) error {
	// If no credentials, remove the file
	if len(credentials) == 0 {
		log.Printf("[HCP Credentials] No credentials, removing file if exists")
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
		credentialsPath := filepath.Join(homeDir, ".terraform.d", "credentials.tfrc.json")
		if _, err := os.Stat(credentialsPath); err == nil {
			if err := os.Remove(credentialsPath); err != nil {
				return fmt.Errorf("failed to remove credentials file: %w", err)
			}
			log.Printf("[HCP Credentials] Credentials file removed")
		}
		return nil
	}

	// Get home directory
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}

	// Create ~/.terraform.d directory
	terraformDir := filepath.Join(homeDir, ".terraform.d")
	if err := os.MkdirAll(terraformDir, 0700); err != nil {
		return fmt.Errorf("failed to create .terraform.d directory: %w", err)
	}

	// Prepare credentials structure
	credentialsFile := map[string]interface{}{
		"credentials": credentials,
	}

	// Write credentials.tfrc.json
	credentialsPath := filepath.Join(terraformDir, "credentials.tfrc.json")
	credentialsJSON, err := json.MarshalIndent(credentialsFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal credentials: %w", err)
	}

	// Write with restricted permissions
	if err := os.WriteFile(credentialsPath, credentialsJSON, 0600); err != nil {
		return fmt.Errorf("failed to write credentials file: %w", err)
	}

	log.Printf("[HCP Credentials] Credentials file updated at %s (%d entries)", credentialsPath, len(credentials))
	return nil
}
