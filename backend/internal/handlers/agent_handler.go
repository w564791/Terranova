package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"
	"iac-platform/internal/websocket"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AgentHandler handles agent-related HTTP requests
type AgentHandler struct {
	agentService          *service.AgentService
	db                    *gorm.DB
	streamManager         *services.OutputStreamManager
	hcpCredentialsService *services.HCPCredentialsService
	metricsHub            *websocket.AgentMetricsHub
	runTaskExecutor       *services.RunTaskExecutor    // Run Task 执行器
	taskQueueManager      *services.TaskQueueManager   // 任务队列管理器（用于 CMDB 同步等 server 侧逻辑）
}

// NewAgentHandler creates a new agent handler
func NewAgentHandler(db *gorm.DB, streamManager *services.OutputStreamManager, metricsHub *websocket.AgentMetricsHub) *AgentHandler {
	return &AgentHandler{
		agentService:          service.NewAgentService(db),
		db:                    db,
		streamManager:         streamManager,
		hcpCredentialsService: services.NewHCPCredentialsService(db),
		metricsHub:            metricsHub,
		runTaskExecutor:       nil, // 延迟初始化
	}
}

// SetRunTaskExecutor sets the Run Task executor
func (h *AgentHandler) SetRunTaskExecutor(executor *services.RunTaskExecutor) {
	h.runTaskExecutor = executor
}

// SetTaskQueueManager sets the task queue manager (for CMDB sync after agent task completion)
func (h *AgentHandler) SetTaskQueueManager(qm *services.TaskQueueManager) {
	h.taskQueueManager = qm
}

// RegisterAgent handles agent registration
// @Summary Register a new agent
// @Description Register a new agent instance with Pool Token
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Param request body models.AgentRegisterRequest true "Registration request"
// @Success 200 {object} models.AgentRegisterResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/register [post]
func (h *AgentHandler) RegisterAgent(c *gin.Context) {
	// Get pool_id from context (set by PoolTokenAuthMiddleware)
	poolID, exists := c.Get("pool_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "pool_id not found in context",
		})
		return
	}

	// Parse request body
	var req models.AgentRegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Get client IP
	ipAddress := c.ClientIP()
	poolIDStr := poolID.(string)
	now := time.Now()

	// For Pool Token mode, we need an application_id
	// Try to get any existing application, or create a default one
	var appID int
	var createdAgent *models.Agent

	// Use transaction to ensure atomicity
	err := h.db.Transaction(func(tx *gorm.DB) error {
		// Try to get existing application
		err := tx.Table("applications").Select("id").Limit(1).Scan(&appID).Error
		if err != nil || appID == 0 {
			// No application exists, create a default one
			// First, ensure org_id=1 exists (or use any existing org)
			var orgID int
			tx.Table("organizations").Select("id").Limit(1).Scan(&orgID)
			if orgID == 0 {
				orgID = 1 // Fallback
			}

			// Create default application
			result := tx.Exec(`
				INSERT INTO applications (app_key, app_secret, name, is_active, org_id)
				VALUES ('pool-token-default', 'not-used', 'Pool Token Default', true, ?)
				ON CONFLICT (app_key) DO UPDATE SET id = applications.id
				RETURNING id
			`, orgID)

			if result.Error != nil {
				return result.Error
			}

			// Get the created application ID
			tx.Table("applications").Where("app_key = ?", "pool-token-default").Select("id").Scan(&appID)
		}

		// Generate unique agent ID with retry logic to handle collisions
		var agentID string
		maxRetries := 5
		for i := 0; i < maxRetries; i++ {
			// Use UnixNano for nanosecond precision to avoid collisions
			agentID = fmt.Sprintf("agent-%s-%d", poolIDStr, time.Now().UnixNano())

			// Check if this ID already exists
			var count int64
			tx.Model(&models.Agent{}).Where("agent_id = ?", agentID).Count(&count)
			if count == 0 {
				break // ID is unique
			}

			// If we've exhausted retries, return error
			if i == maxRetries-1 {
				return fmt.Errorf("failed to generate unique agent ID after %d attempts", maxRetries)
			}

			// Small delay before retry
			time.Sleep(time.Millisecond)
		}

		// Create agent
		agentName := req.Name
		if agentName == "" {
			// Use agent_id as name if not provided
			agentName = agentID
		}

		agent := &models.Agent{
			AgentID:       agentID,
			ApplicationID: appID,
			PoolID:        &poolIDStr,
			Name:          agentName,
			TokenHash:     "", // Pool Token mode doesn't store token_hash in agents table
			Status:        "online",
			IPAddress:     &ipAddress,
			RegisteredAt:  now,
			LastPingAt:    &now,
		}

		// Set version if provided
		if req.Version != "" {
			agent.Version = &req.Version
		}

		if err := tx.Create(agent).Error; err != nil {
			return err
		}

		createdAgent = agent
		return nil
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to register agent: " + err.Error(),
		})
		return
	}

	// Generate HCP credentials file for this agent pool
	// This will create ~/.terraform.d/credentials.tfrc.json if HCP secrets exist
	generated, err := h.hcpCredentialsService.GenerateCredentialsFile(poolIDStr)
	if err != nil {
		// Log error but don't fail registration
		// The agent can still work without HCP credentials
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "agent registered but failed to generate HCP credentials: " + err.Error(),
		})
		return
	}

	// Return response using the created agent from transaction
	response := gin.H{
		"agent_id":                  createdAgent.AgentID,
		"pool_id":                   createdAgent.PoolID,
		"status":                    createdAgent.Status,
		"registered_at":             createdAgent.RegisteredAt,
		"hcp_credentials_generated": generated,
	}

	c.JSON(http.StatusOK, response)
}

// PingAgent handles agent heartbeat
// @Summary Agent heartbeat
// @Description Update agent heartbeat and status
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param agent_id path string true "Agent ID"
// @Param request body models.AgentPingRequest true "Ping request"
// @Success 200 {object} models.AgentPingResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id}/ping [post]
func (h *AgentHandler) PingAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agent_id is required",
		})
		return
	}

	// Parse request body
	var req models.AgentPingRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Get agent info to get pool_id and name
	var agent models.Agent
	if err := h.db.Where("agent_id = ?", agentID).First(&agent).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "agent not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Update ping
	err := h.agentService.PingAgent(agentID, req.Status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Broadcast metrics to WebSocket if metricsHub is available
	if h.metricsHub != nil && agent.PoolID != nil {
		// Convert models.RunningTask to websocket.RunningTask
		var runningTasks []websocket.RunningTask
		for _, task := range req.RunningTasks {
			runningTasks = append(runningTasks, websocket.RunningTask{
				TaskID:      task.TaskID,
				TaskType:    task.TaskType,
				WorkspaceID: task.WorkspaceID,
				StartedAt:   task.StartedAt,
			})
		}

		metrics := &websocket.AgentMetrics{
			AgentID:        agentID,
			AgentName:      agent.Name,
			CPUUsage:       req.CPUUsage,
			MemoryUsage:    req.MemoryUsage,
			RunningTasks:   runningTasks,
			LastUpdateTime: time.Now(),
			Status:         req.Status,
		}
		h.metricsHub.BroadcastMetrics(*agent.PoolID, metrics)
	}

	// Return response
	c.JSON(http.StatusOK, models.AgentPingResponse{
		Message:    "ping received",
		LastPingAt: time.Now(),
	})
}

// GetAgent retrieves agent information
// @Summary Get agent information
// @Description Get detailed information about an agent
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} models.Agent
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id} [get]
func (h *AgentHandler) GetAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agent_id is required",
		})
		return
	}

	// Get agent
	agent, err := h.agentService.GetAgent(agentID)
	if err != nil {
		if err.Error() == "agent not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, agent)
}

// UnregisterAgent handles agent unregistration
// @Summary Unregister an agent
// @Description Remove an agent from the system
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param agent_id path string true "Agent ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/{agent_id} [delete]
func (h *AgentHandler) UnregisterAgent(c *gin.Context) {
	agentID := c.Param("agent_id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "agent_id is required",
		})
		return
	}

	// Unregister agent
	err := h.agentService.UnregisterAgent(agentID)
	if err != nil {
		if err.Error() == "agent not found" {
			c.JSON(http.StatusNotFound, gin.H{
				"error": err.Error(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "agent unregistered successfully",
	})
}

// GetTaskData retrieves all data needed for task execution
// @Summary Get task execution data
// @Description Get complete task data including workspace config, resources, variables, and state
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/data [get]
func (h *AgentHandler) GetTaskData(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}

	// Convert task_id to uint
	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid task_id format",
		})
		return
	}

	// Get task
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Get workspace
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get workspace: " + err.Error(),
		})
		return
	}

	// Get workspace resources
	var resources []models.WorkspaceResource
	h.db.Where("workspace_id = ? AND is_active = true", workspace.WorkspaceID).Find(&resources)

	// Load current version for each resource
	for i := range resources {
		if resources[i].CurrentVersionID != nil {
			var version models.ResourceCodeVersion
			if err := h.db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
				resources[i].CurrentVersion = &version
			}
		}
	}

	// Get workspace variables (只获取每个变量的最新版本)
	var variables []models.WorkspaceVariable
	h.db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		INNER JOIN (
			SELECT variable_id, MAX(version) as max_version
			FROM workspace_variables
			WHERE workspace_id = ? AND is_deleted = false
			GROUP BY variable_id
		) latest ON wv.variable_id = latest.variable_id AND wv.version = latest.max_version
		WHERE wv.workspace_id = ? AND wv.is_deleted = false
	`, workspace.WorkspaceID, workspace.WorkspaceID).Scan(&variables)

	// Get workspace outputs
	var outputs []models.WorkspaceOutput
	h.db.Where("workspace_id = ?", workspace.WorkspaceID).Find(&outputs)

	// Get module versions for version fallback (Agent mode needs this to add version to tf_code)
	var modules []models.Module
	h.db.Select("name, provider, version").Find(&modules)
	moduleVersions := make(map[string]string)
	for _, m := range modules {
		if m.Version != "" {
			key := fmt.Sprintf("%s_%s", m.Provider, m.Name)
			moduleVersions[key] = m.Version
		}
	}

	// Get workspace remote data references
	var remoteDataList []models.WorkspaceRemoteData
	h.db.Where("workspace_id = ?", workspace.WorkspaceID).Find(&remoteDataList)

	// Generate remote data config with tokens for agent
	var remoteDataConfig []gin.H
	if len(remoteDataList) > 0 {
		// Get platform base URL
		platformConfigService := services.NewPlatformConfigService(h.db)
		baseURL := platformConfigService.GetBaseURL()

		// Create RemoteDataTFGenerator to generate tokens
		generator := services.NewRemoteDataTFGenerator(h.db, baseURL)

		for _, rd := range remoteDataList {
			// Generate token for each remote data reference
			token, err := generator.GenerateTokenForAgent(workspace.WorkspaceID, rd.SourceWorkspaceID, &taskID)
			if err != nil {
				// Log error but continue
				fmt.Printf("[GetTaskData] Failed to generate token for remote data %s: %v\n", rd.RemoteDataID, err)
				continue
			}

			remoteDataConfig = append(remoteDataConfig, gin.H{
				"remote_data_id":      rd.RemoteDataID,
				"source_workspace_id": rd.SourceWorkspaceID,
				"data_name":           rd.DataName,
				"token":               token,
				"url":                 fmt.Sprintf("%s/api/v1/workspaces/%s/state-outputs/full", baseURL, rd.SourceWorkspaceID),
			})
		}
	}

	// Get latest state version
	var stateVersion models.WorkspaceStateVersion
	err := h.db.Where("workspace_id = ?", workspace.WorkspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	// 只有在成功找到state version时才包含在响应中
	// 如果是ErrRecordNotFound，说明没有state，不应该返回空的state对象
	hasStateVersion := (err == nil)

	if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get state version: " + err.Error(),
		})
		return
	}

	// Build response
	response := gin.H{
		"task": gin.H{
			"id":           task.ID,
			"workspace_id": task.WorkspaceID,
			"task_type":    task.TaskType,
			"action":       task.TaskType,
			"context":      task.Context,
			"created_at":   task.CreatedAt,
			"plan_task_id": task.PlanTaskID, // 【修复】添加 plan_task_id 字段
			"agent_id":     task.AgentID,    // 【Phase 1优化】添加 agent_id 字段
		},
		"workspace": gin.H{
			"workspace_id":       workspace.WorkspaceID,
			"name":               workspace.Name,
			"terraform_version":  workspace.TerraformVersion,
			"execution_mode":     workspace.ExecutionMode,
			"provider_config":    workspace.ProviderConfig,
			"tf_code":            workspace.TFCode,
			"system_variables":   workspace.SystemVariables,
			"terraform_lock_hcl": workspace.TerraformLockHCL, // 用于恢复 .terraform.lock.hcl 文件
		},
		"resources":       resources,
		"variables":       variables,
		"outputs":         outputs,          // 【新增】添加 outputs 配置，用于生成 outputs.tf.json
		"remote_data":     remoteDataConfig, // 【新增】添加 remote_data 配置，用于生成 remote_data.tf.json
		"module_versions": moduleVersions,   // 【新增】添加 module_versions，用于 Agent 模式下补充 tf_code 中缺失的 version 字段
	}

	// Add state version ONLY if it actually exists in database
	if hasStateVersion {
		response["state_version"] = gin.H{
			"version":  stateVersion.Version,
			"content":  stateVersion.Content,
			"checksum": stateVersion.Checksum,
			"size":     stateVersion.SizeBytes,
		}
	}

	c.JSON(http.StatusOK, response)
}

// UploadTaskLogChunk handles incremental log upload
// @Summary Upload task log chunk
// @Description Upload a chunk of task log data incrementally
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Log chunk data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/logs/chunk [post]
func (h *AgentHandler) UploadTaskLogChunk(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}

	// Convert task_id to uint
	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid task_id format",
		})
		return
	}

	// Parse request body
	var req struct {
		Phase    string `json:"phase" binding:"required"`   // "plan" or "apply"
		Content  string `json:"content" binding:"required"` // Log content
		Offset   int64  `json:"offset"`                     // Current offset
		Checksum string `json:"checksum"`                   // SHA256 checksum
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	// Verify task exists
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Save log chunk to task_logs table
	taskLog := &models.TaskLog{
		TaskID:  taskID,
		Phase:   req.Phase,
		Content: req.Content,
		Level:   "info",
	}

	if err := h.db.Create(taskLog).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save log: " + err.Error(),
		})
		return
	}

	// Also update task's output field
	if req.Phase == "plan" {
		task.PlanOutput += req.Content
	} else if req.Phase == "apply" {
		task.ApplyOutput += req.Content
	}
	h.db.Save(&task)

	// IMPORTANT: Feed logs into OutputStreamManager for real-time WebSocket streaming
	// This allows frontend to see agent logs in real-time via WebSocket
	// 使用 BroadcastLocal：agent 模式已通过 C&C WebSocket 路径处理跨副本转发
	if h.streamManager != nil {
		stream := h.streamManager.GetOrCreate(taskID)
		if stream != nil {
			// Split content into lines and broadcast each line
			lines := strings.Split(req.Content, "\n")
			for _, line := range lines {
				if line != "" {
					stream.BroadcastLocal(services.OutputMessage{
						Type:      "output",
						Line:      line,
						Timestamp: time.Now(),
					})
				}
			}
		}
	}

	// Return success with next offset
	c.JSON(http.StatusOK, gin.H{
		"status":      "ok",
		"next_offset": req.Offset + int64(len(req.Content)),
		"saved_bytes": len(req.Content),
	})
}

// UpdateTaskStatus updates task status
// @Summary Update task status
// @Description Update the status of a running task
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Status update"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/status [put]
func (h *AgentHandler) UpdateTaskStatus(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}

	// Convert task_id to uint
	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid task_id format",
		})
		return
	}

	// Parse request body
	var req struct {
		Status         models.TaskStatus      `json:"status" binding:"required"`
		Stage          string                 `json:"stage"`
		ErrorMessage   string                 `json:"error_message"`
		ChangesAdd     int                    `json:"changes_add"`
		ChangesChange  int                    `json:"changes_change"`
		ChangesDestroy int                    `json:"changes_destroy"`
		Duration       int                    `json:"duration"`
		Context        map[string]interface{} `json:"context"`
		PlanHash       string                 `json:"plan_hash"`    // 【Phase 1优化】
		PlanTaskID     *uint                  `json:"plan_task_id"` // 【Phase 1优化】
		PlanOutput     string                 `json:"plan_output"`  // Plan 输出
		ApplyOutput    string                 `json:"apply_output"` // Apply 输出
		CompletedAt    *time.Time             `json:"completed_at"` // 完成时间
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	// Get task
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// 【防御】拒绝覆盖已取消的任务状态
	// 竞态场景：用户取消任务(→cancelled)后，Agent仍可能发来状态更新(→apply_pending)
	if task.Status == models.TaskStatusCancelled {
		log.Printf("[UpdateTaskStatus] Rejected status update for task %d: task already cancelled, agent tried to set %s", taskID, req.Status)
		c.JSON(http.StatusConflict, gin.H{
			"error":          "task has been cancelled, status update rejected",
			"current_status": string(task.Status),
		})
		return
	}

	// Build updates map to avoid overwriting other fields (like plan_data, plan_json)
	updates := map[string]interface{}{
		"status": req.Status,
	}

	if req.Stage != "" {
		updates["stage"] = req.Stage
	}
	if req.ErrorMessage != "" {
		updates["error_message"] = req.ErrorMessage
	}
	if req.ChangesAdd > 0 || req.ChangesChange > 0 || req.ChangesDestroy > 0 {
		updates["changes_add"] = req.ChangesAdd
		updates["changes_change"] = req.ChangesChange
		updates["changes_destroy"] = req.ChangesDestroy
	}
	if req.Duration > 0 {
		updates["duration"] = req.Duration
	}
	if req.Context != nil {
		updates["context"] = req.Context
	}

	// 【Phase 1优化】Add plan_hash if provided
	if req.PlanHash != "" {
		updates["plan_hash"] = req.PlanHash
	}

	// Add plan_task_id if provided
	if req.PlanTaskID != nil {
		updates["plan_task_id"] = *req.PlanTaskID
	}

	// Add plan_output if provided
	if req.PlanOutput != "" {
		updates["plan_output"] = req.PlanOutput
	}

	// Add apply_output if provided
	if req.ApplyOutput != "" {
		updates["apply_output"] = req.ApplyOutput
	}

	// Set completed_at if task is finished or if provided in request
	if req.CompletedAt != nil {
		updates["completed_at"] = req.CompletedAt
	} else if req.Status == models.TaskStatusSuccess ||
		req.Status == models.TaskStatusFailed ||
		req.Status == models.TaskStatusApplied {
		now := time.Now()
		updates["completed_at"] = &now
	}

	// Use Updates() instead of Save() to avoid overwriting other fields
	if err := h.db.Model(&task).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to update task: " + err.Error(),
		})
		return
	}

	// 注意：post_plan Run Tasks 在 UploadPlanData 中执行（Plan 完成后）
	// apply_pending 状态是 pre_apply 的时机，但 pre_apply 需要在 Agent 开始 Apply 前执行
	// 目前 pre_apply 在 terraform_executor.go 中处理（Local 模式）
	// Agent 模式下的 pre_apply 需要在 Agent 请求 Apply 数据时执行

	// 如果任务成功完成（applied），执行 Run Triggers
	// 注意：使用 ExecuteTriggersCreateOnly 只创建任务，不调用 TryExecuteNextTask
	// 这样可以避免在没有完整初始化的 TaskQueueManager 中执行任务导致崩溃
	// 创建的任务会被现有的任务队列机制自动执行
	if req.Status == models.TaskStatusApplied {
		// 执行 Run Triggers
		go func() {
			log.Printf("[RunTrigger] Task %d completed with status applied, executing run triggers", taskID)
			runTriggerService := services.NewRunTriggerService(h.db)
			ctx := context.Background()
			if err := runTriggerService.ExecuteTriggersCreateOnly(ctx, &task); err != nil {
				log.Printf("[RunTrigger] Failed to execute run triggers for task %d: %v", taskID, err)
			} else {
				log.Printf("[RunTrigger] Successfully executed run triggers for task %d", taskID)
			}
		}()
	}

	// Apply 完成后（无论成功还是失败）同步 CMDB
	// 失败的 apply 中可能有部分资源已创建，也需要同步
	if h.taskQueueManager != nil &&
		task.TaskType == models.TaskTypePlanAndApply &&
		(req.Status == models.TaskStatusApplied || req.Status == models.TaskStatusFailed) {
		// 使用 req.Status 而非 task.Status，因为 DB 已更新但内存对象未刷新
		taskCopy := task
		taskCopy.Status = req.Status
		go h.taskQueueManager.SyncCMDBAfterApply(&taskCopy)
	}

	// Drift Check 结果处理（Agent 模式补齐）
	if task.TaskType == models.TaskTypeDriftCheck &&
		(req.Status == models.TaskStatusSuccess ||
			req.Status == models.TaskStatusFailed ||
			req.Status == models.TaskStatusApplied ||
			req.Status == models.TaskStatusPlannedAndFinished ||
			req.Status == models.TaskStatusCancelled) {
		taskCopy := task
		taskCopy.Status = req.Status
		go services.NewDriftCheckService(h.db).ProcessDriftCheckResult(&taskCopy)
	}

	// 任务到达终态后，立即触发下个任务执行（Agent 模式补齐）
	// 此前依赖 checkAndRetryPendingTasks 定时轮询，延迟可达 30-60s
	if h.taskQueueManager != nil &&
		(req.Status == models.TaskStatusSuccess ||
			req.Status == models.TaskStatusApplied ||
			req.Status == models.TaskStatusPlannedAndFinished ||
			req.Status == models.TaskStatusFailed ||
			req.Status == models.TaskStatusCancelled) {
		go h.taskQueueManager.TryExecuteNextTask(task.WorkspaceID)
	}

	// K8s Slot 释放 — 任务到达终态后释放 pod slot（Agent 模式补齐）
	if h.taskQueueManager != nil &&
		(req.Status == models.TaskStatusSuccess ||
			req.Status == models.TaskStatusApplied ||
			req.Status == models.TaskStatusPlannedAndFinished ||
			req.Status == models.TaskStatusFailed ||
			req.Status == models.TaskStatusCancelled) {
		go h.taskQueueManager.ReleaseTaskSlot(taskID)
	}

	// K8s Slot 预留 — 转入 apply_pending 时预留 slot 防 scale-down（Agent 模式补齐）
	if h.taskQueueManager != nil && req.Status == models.TaskStatusApplyPending {
		go h.taskQueueManager.ReserveSlotForApplyPending(taskID)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "task status updated",
		"task_id": taskID,
		"status":  req.Status,
	})
}

// SaveTaskState saves task state version
// @Summary Save task state
// @Description Save a new state version for the task
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "State data"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/state [post]
func (h *AgentHandler) SaveTaskState(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "task_id is required",
		})
		return
	}

	// Convert task_id to uint
	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid task_id format",
		})
		return
	}

	// Parse request body
	var req struct {
		Content  map[string]interface{} `json:"content" binding:"required"`
		Checksum string                 `json:"checksum" binding:"required"`
		Size     int                    `json:"size" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body: " + err.Error(),
		})
		return
	}

	// Get task
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "task not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	// Get workspace
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get workspace: " + err.Error(),
		})
		return
	}

	// Get max version
	var maxVersion int
	h.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspace.WorkspaceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion)

	newVersion := maxVersion + 1

	// Create new state version in transaction
	err := h.db.Transaction(func(tx *gorm.DB) error {
		stateVersion := &models.WorkspaceStateVersion{
			WorkspaceID: workspace.WorkspaceID,
			Version:     newVersion,
			Content:     req.Content,
			Checksum:    req.Checksum,
			SizeBytes:   req.Size,
			TaskID:      &task.ID,
			CreatedBy:   task.CreatedBy,
		}

		if err := tx.Create(stateVersion).Error; err != nil {
			return err
		}

		// Update workspace's tf_state
		return tx.Model(&workspace).Update("tf_state", req.Content).Error
	})

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save state: " + err.Error(),
		})
		return
	}

	// 【修复】从 State 中提取资源 ID（Agent 模式下在服务端执行）
	// 这是之前在 Local 模式下执行但在 Agent 模式下被跳过的逻辑
	go func() {
		log.Printf("[SaveTaskState] Extracting resource IDs from state for task %d (Agent mode)", taskID)

		// 创建一个简单的 logger（用于 ApplyParserService）
		stream := h.streamManager.GetOrCreate(taskID)
		logger := services.NewTerraformLoggerWithLevelAndMode(stream, "info", true)

		applyParserService := services.NewApplyParserService(h.db, h.streamManager)
		if err := applyParserService.ExtractResourceDetailsFromState(taskID, req.Content, logger); err != nil {
			log.Printf("[SaveTaskState] Failed to extract resource IDs for task %d: %v", taskID, err)
		} else {
			log.Printf("[SaveTaskState] Successfully extracted resource IDs for task %d", taskID)
		}
	}()

	c.JSON(http.StatusOK, gin.H{
		"message": "state saved successfully",
		"version": newVersion,
	})
}

// ============================================================================
// New handlers for Agent Mode refactoring
// ============================================================================

// GetPlanTask retrieves a plan task by ID
// @Summary Get plan task
// @Description Get plan task information for agent execution, including plan_data for apply tasks
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/plan-task [get]
func (h *AgentHandler) GetPlanTask(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 【调试】打印 agent_id 的值
	if task.AgentID != nil {
		fmt.Printf("[GetPlanTask] Task %d agent_id from DB: %s\n", taskID, *task.AgentID)
	} else {
		fmt.Printf("[GetPlanTask] Task %d agent_id from DB: nil\n", taskID)
	}

	// 【新增】根据快照中的版本号，从数据库加载完整的资源数据
	var snapshotResources []gin.H
	if task.SnapshotResourceVersions != nil {
		for resourceID, versionInfo := range task.SnapshotResourceVersions {
			versionMap, ok := versionInfo.(map[string]interface{})
			if !ok {
				continue
			}

			// 使用 version 字段（版本号）而不是 version_id
			version, ok := versionMap["version"].(float64)
			if !ok {
				continue
			}

			// 查询资源基本信息
			var resource models.WorkspaceResource
			if err := h.db.Where("resource_id = ?", resourceID).First(&resource).Error; err != nil {
				continue // 跳过不存在的资源
			}

			// 查询指定版本的代码
			var codeVersion models.ResourceCodeVersion
			if err := h.db.Where("resource_id = ? AND version = ?", resource.ID, int(version)).
				First(&codeVersion).Error; err != nil {
				continue // 跳过不存在的版本
			}

			// 构造资源数据
			snapshotResources = append(snapshotResources, gin.H{
				"id":           resource.ID,
				"workspace_id": resource.WorkspaceID,
				"resource_id":  resource.ResourceID,
				"is_active":    resource.IsActive,
				"current_version": gin.H{
					"id":      codeVersion.ID,
					"version": codeVersion.Version,
					"tf_code": codeVersion.TFCode,
				},
			})
		}
	}

	// 构造 task 响应，处理指针字段
	taskResponse := gin.H{
		"id":           task.ID,
		"workspace_id": task.WorkspaceID,
		"task_type":    task.TaskType,
		"context":      task.Context,
		"created_at":   task.CreatedAt, // 添加 created_at
	}

	// 处理指针字段 - 如果是 nil 则不包含在响应中（而不是返回 null）
	if task.PlanTaskID != nil {
		taskResponse["plan_task_id"] = *task.PlanTaskID
	}
	if task.AgentID != nil {
		taskResponse["agent_id"] = *task.AgentID

		// 【新增】同时返回 agent 的 name（hostname），用于 Apply 阶段比较
		var agent models.Agent
		if err := h.db.Where("agent_id = ?", *task.AgentID).First(&agent).Error; err == nil {
			taskResponse["agent_name"] = agent.Name
		}
	}
	if task.PlanHash != "" {
		taskResponse["plan_hash"] = task.PlanHash
	}
	if task.SnapshotCreatedAt != nil {
		taskResponse["snapshot_created_at"] = task.SnapshotCreatedAt
	}
	if task.SnapshotResourceVersions != nil {
		taskResponse["snapshot_resource_versions"] = task.SnapshotResourceVersions
	}
	// 【修复】解析快照变量:如果是旧格式(只有引用),需要从数据库查询完整数据
	if task.SnapshotVariables != nil {
		// JSONB.Scan 将 DB 中的 JSON 数组包装为 {"_array": [...]},
		// 使用 UnwrapArray() 还原为原始数组 JSON
		snapshotVarsJSON, _ := task.SnapshotVariables.UnwrapArray()
		var snapshotVars []map[string]interface{}
		if err := json.Unmarshal(snapshotVarsJSON, &snapshotVars); err == nil && len(snapshotVars) > 0 {
			// 检查第一个元素是否包含key字段
			firstVar := snapshotVars[0]
			if _, hasKey := firstVar["key"]; !hasKey {
				// 旧格式(只有引用),需要查询完整数据
				var fullVariables []gin.H
				for _, snapVar := range snapshotVars {
					varID, _ := snapVar["variable_id"].(string)
					version, _ := snapVar["version"].(float64)

					// 从数据库查询完整变量数据
					var variable models.WorkspaceVariable
					if err := h.db.Where("variable_id = ? AND version = ?", varID, int(version)).
						First(&variable).Error; err == nil {
						fullVariables = append(fullVariables, gin.H{
							"workspace_id":  variable.WorkspaceID,
							"variable_id":   variable.VariableID,
							"version":       variable.Version,
							"variable_type": variable.VariableType,
							"key":           variable.Key,
							"value":         variable.Value,
							"sensitive":     variable.Sensitive,
							"description":   variable.Description,
							"value_format":  variable.ValueFormat,
						})
					}
				}
				taskResponse["snapshot_variables"] = fullVariables
			} else {
				// 新格式(已包含完整数据),直接返回
				taskResponse["snapshot_variables"] = task.SnapshotVariables
			}
		} else {
			// 无法解析或为空,直接返回原始数据
			taskResponse["snapshot_variables"] = task.SnapshotVariables
		}
	}
	if task.SnapshotProviderConfig != nil {
		taskResponse["snapshot_provider_config"] = task.SnapshotProviderConfig
	}

	// 【修复】将snapshot_variables放入task.context中,供RemoteDataAccessor缓存
	if len(taskResponse) > 0 {
		// 确保 context 存在且是 map 类型
		var contextMap map[string]interface{}
		if taskResponse["context"] != nil {
			if cm, ok := taskResponse["context"].(map[string]interface{}); ok {
				contextMap = cm
			}
		}
		// 如果 context 为 nil 或不是 map，创建新的 map
		if contextMap == nil {
			contextMap = make(map[string]interface{})
			taskResponse["context"] = contextMap
		}

		// 将snapshot_resources放入context
		contextMap["_snapshot_resources"] = snapshotResources

		// 将snapshot_variables放入context
		if snapVars, hasSnapVars := taskResponse["snapshot_variables"]; hasSnapVars {
			contextMap["_snapshot_variables"] = snapVars
		}
	}

	response := gin.H{
		"task":               taskResponse,
		"snapshot_resources": snapshotResources, // 【新增】返回快照资源的完整数据
	}

	// Include plan_data if it exists
	// IMPORTANT: Encode binary data to base64 for API transmission
	// Database stores binary, API transmits base64
	if len(task.PlanData) > 0 {
		encodedData := base64.StdEncoding.EncodeToString(task.PlanData)
		response["plan_data"] = encodedData
	}

	c.JSON(http.StatusOK, response)
}

// UploadPlanData handles plan data upload from agent
// @Summary Upload plan data
// @Description Upload base64-encoded plan data from agent after plan execution
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Plan data (base64 encoded)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/plan-data [post]
func (h *AgentHandler) UploadPlanData(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	// Parse request body
	var req struct {
		PlanData string `json:"plan_data" binding:"required"` // base64 encoded
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Verify task exists
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// IMPORTANT: Decode base64 to binary before storing
	// This ensures consistency with Local mode which stores binary directly
	// Database stores binary data, API transmits base64-encoded data
	decodedData, err := base64.StdEncoding.DecodeString(req.PlanData)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid base64 encoding: " + err.Error(),
		})
		return
	}

	// Store the decoded binary data (same as Local mode)
	if err := h.db.Model(&task).Update("plan_data", decodedData).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save plan_data: " + err.Error(),
		})
		return
	}

	// 【Run Task 集成】Plan 数据上传完成后，执行 post_plan Run Tasks
	// post_plan 在 Plan 完成后执行，无论是 plan 还是 plan_and_apply 任务类型
	if h.runTaskExecutor != nil {
		go func() {
			// 重新加载任务以获取最新数据
			var taskForRunTask models.WorkspaceTask
			if err := h.db.First(&taskForRunTask, taskID).Error; err != nil {
				fmt.Printf("[RunTask] Failed to reload task %d for post_plan Run Tasks: %v\n", taskID, err)
				return
			}

			fmt.Printf("[RunTask] Executing post_plan Run Tasks for task %d (Agent mode, after plan_data upload)\n", taskID)

			// 执行 post_plan Run Tasks
			// 【重要】使用 context.Background() 而不是 c.Request.Context()
			// 因为 goroutine 是异步执行的，HTTP 请求完成后 c.Request.Context() 会被取消
			// 这会导致 webhook 请求失败（context canceled）
			ctx := context.Background()
			passed, err := h.runTaskExecutor.ExecuteRunTasksForStage(ctx, &taskForRunTask, models.RunTaskStagePostPlan)
			if err != nil {
				fmt.Printf("[RunTask] post_plan Run Tasks execution error for task %d: %v\n", taskID, err)
				// 更新任务状态为失败
				h.db.Model(&taskForRunTask).Updates(map[string]interface{}{
					"status":        models.TaskStatusFailed,
					"error_message": fmt.Sprintf("Post-plan Run Task execution error: %v", err),
				})
				return
			}

			if !passed {
				fmt.Printf("[RunTask] post_plan Run Tasks blocked execution for task %d (mandatory task failed)\n", taskID)
				// 更新任务状态为失败
				h.db.Model(&taskForRunTask).Updates(map[string]interface{}{
					"status":        models.TaskStatusFailed,
					"error_message": "Post-plan Run Task failed (mandatory)",
				})
				return
			}

			fmt.Printf("[RunTask] post_plan Run Tasks completed successfully for task %d\n", taskID)
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "plan_data uploaded successfully",
		"size":    len(decodedData),
	})
}

// UploadPlanJSON handles plan JSON upload from agent
// @Summary Upload plan JSON
// @Description Upload plan JSON from agent after plan execution
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Plan JSON"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/plan-json [post]
func (h *AgentHandler) UploadPlanJSON(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	// Parse request body
	var req struct {
		PlanJSON map[string]interface{} `json:"plan_json" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Verify task exists
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Store the plan_json
	if err := h.db.Model(&task).Update("plan_json", req.PlanJSON).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save plan_json: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "plan_json uploaded successfully",
	})
}

// LockWorkspace locks a workspace
// @Summary Lock workspace
// @Description Lock a workspace for exclusive access
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Lock request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/lock [post]
func (h *AgentHandler) LockWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var req struct {
		UserID string `json:"user_id" binding:"required"`
		Reason string `json:"reason"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	updates := map[string]interface{}{
		"is_locked": true,
		"locked_by": req.UserID,
		"locked_at": time.Now(),
	}

	if req.Reason != "" {
		updates["lock_reason"] = req.Reason
	}

	if err := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to lock workspace"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace locked"})
}

// UnlockWorkspace unlocks a workspace
// @Summary Unlock workspace
// @Description Unlock a workspace
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/unlock [post]
func (h *AgentHandler) UnlockWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	updates := map[string]interface{}{
		"is_locked":   false,
		"locked_by":   nil,
		"locked_at":   nil,
		"lock_reason": nil,
	}

	if err := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unlock workspace"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace unlocked"})
}

// ParsePlanChanges parses plan changes
// @Summary Parse plan changes
// @Description Receive parsed resource changes from agent and store in database
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Parsed resource changes"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/parse-plan-changes [post]
func (h *AgentHandler) ParsePlanChanges(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	// Expect agent to send parsed resource changes
	var req struct {
		ResourceChanges []struct {
			ResourceAddress string                 `json:"resource_address"`
			ResourceType    string                 `json:"resource_type"`
			ResourceName    string                 `json:"resource_name"`
			ModuleAddress   string                 `json:"module_address"`
			Action          string                 `json:"action"`
			ChangesBefore   map[string]interface{} `json:"changes_before"`
			ChangesAfter    map[string]interface{} `json:"changes_after"`
		} `json:"resource_changes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Get task
	var task models.WorkspaceTask
	if err := h.db.First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Delete old resource changes for this task
	h.db.Where("task_id = ?", taskID).Delete(&models.WorkspaceTaskResourceChange{})

	// Insert new resource changes
	for _, rc := range req.ResourceChanges {
		change := &models.WorkspaceTaskResourceChange{
			TaskID:          taskID,
			WorkspaceID:     task.WorkspaceID,
			ResourceAddress: rc.ResourceAddress,
			ResourceType:    rc.ResourceType,
			ResourceName:    rc.ResourceName,
			ModuleAddress:   rc.ModuleAddress,
			Action:          rc.Action,
			ChangesBefore:   rc.ChangesBefore,
			ChangesAfter:    rc.ChangesAfter,
			ApplyStatus:     "pending",
		}

		if err := h.db.Create(change).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "failed to save resource change: " + err.Error(),
			})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "resource changes saved",
		"count":   len(req.ResourceChanges),
	})
}

// GetTaskLogs retrieves task logs
// @Summary Get task logs
// @Description Get all logs for a task
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/logs [get]
func (h *AgentHandler) GetTaskLogs(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	var logs []models.TaskLog
	if err := h.db.Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"logs": logs})
}

// GetMaxStateVersion gets the maximum state version for a workspace
// @Summary Get max state version
// @Description Get the maximum state version number for a workspace
// @Tags Agent
// @Accept json
// @Produce json
// @Param X-App-Key header string true "Application Key"
// @Param X-App-Secret header string true "Application Secret"
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/state/max-version [get]
func (h *AgentHandler) GetMaxStateVersion(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var maxVersion int
	err := h.db.Model(&models.WorkspaceStateVersion{}).
		Where("workspace_id = ?", workspaceID).
		Select("COALESCE(MAX(version), 0)").
		Scan(&maxVersion).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get max version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"max_version": maxVersion})
}

// GetDefaultTerraformVersion gets the default terraform version
// @Summary Get default terraform version
// @Description Get the default terraform version configuration for agent download
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/terraform-versions/default [get]
func (h *AgentHandler) GetDefaultTerraformVersion(c *gin.Context) {
	var version models.TerraformVersion
	err := h.db.Where("is_default = ? AND enabled = ?", true, true).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": "no default terraform version configured"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get default version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"version": version})
}

// GetTerraformVersionByVersion gets a specific terraform version by version string
// @Summary Get terraform version by version string
// @Description Get terraform version configuration by version string for agent download
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Param version path string true "Version string (e.g., 1.5.0)"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/terraform-versions/{version} [get]
func (h *AgentHandler) GetTerraformVersionByVersion(c *gin.Context) {
	versionStr := c.Param("version")
	if versionStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "version is required"})
		return
	}

	var version models.TerraformVersion
	err := h.db.Where("version = ? AND enabled = ?", versionStr, true).First(&version).Error
	if err == gorm.ErrRecordNotFound {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("terraform version %s not found or not enabled", versionStr)})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get version"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"version": version})
}

// UpdateWorkspaceFields updates specific fields of a workspace
// @Summary Update workspace fields
// @Description Update specific fields of a workspace (used by agent to update last_init_hash, etc.)
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Fields to update"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/fields [patch]
func (h *AgentHandler) UpdateWorkspaceFields(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	// Parse request body - allow any fields
	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	// Whitelist of allowed fields to update (security measure)
	allowedFields := map[string]bool{
		"last_init_hash":              true,
		"last_init_terraform_version": true,
		"terraform_lock_hcl":          true, // 用于保存 .terraform.lock.hcl 文件内容
	}

	// Filter updates to only allowed fields
	filteredUpdates := make(map[string]interface{})
	for key, value := range updates {
		if allowedFields[key] {
			filteredUpdates[key] = value
		}
	}

	if len(filteredUpdates) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no valid fields to update"})
		return
	}

	// Update workspace
	if err := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(filteredUpdates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workspace: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":        "workspace fields updated",
		"updated_fields": filteredUpdates,
	})
}

// GetTerraformLockHCL gets the terraform lock hcl content for a workspace
// @Summary Get terraform lock hcl
// @Description Get the .terraform.lock.hcl file content for a workspace
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/terraform-lock-hcl [get]
func (h *AgentHandler) GetTerraformLockHCL(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var workspace models.Workspace
	if err := h.db.Select("terraform_lock_hcl").
		Where("workspace_id = ?", workspaceID).
		First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get workspace: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"terraform_lock_hcl": workspace.TerraformLockHCL,
	})
}

// SaveTerraformLockHCL saves the terraform lock hcl content for a workspace
// @Summary Save terraform lock hcl
// @Description Save the .terraform.lock.hcl file content for a workspace
// @Tags Agent
// @Accept json
// @Produce json
// @Param Authorization header string true "Bearer {pool_token}"
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Lock HCL content"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/terraform-lock-hcl [put]
func (h *AgentHandler) SaveTerraformLockHCL(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var req struct {
		TerraformLockHCL string `json:"terraform_lock_hcl" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}

	if err := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("terraform_lock_hcl", req.TerraformLockHCL).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save terraform lock hcl: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "terraform lock hcl saved successfully",
	})
}
