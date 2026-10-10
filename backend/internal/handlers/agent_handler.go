package handlers

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/manifestbundle"
	"iac-platform/internal/middleware"
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
	runTaskExecutor       *services.RunTaskExecutor   // Run Task 执行器
	taskQueueManager      *services.TaskQueueManager  // 任务队列管理器（用于 CMDB 同步等 server 侧逻辑）
	stateTokenService     *services.StateTokenService // HTTP state backend token service
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

// SetStateTokenService sets the state token service for HTTP state backend.
func (h *AgentHandler) SetStateTokenService(ts *services.StateTokenService) {
	h.stateTokenService = ts
}

// SetRunTaskExecutor sets the Run Task executor
func (h *AgentHandler) SetRunTaskExecutor(executor *services.RunTaskExecutor) {
	h.runTaskExecutor = executor
}

// SetTaskQueueManager sets the task queue manager (for CMDB sync after agent task completion)
func (h *AgentHandler) SetTaskQueueManager(qm *services.TaskQueueManager) {
	h.taskQueueManager = qm
}

// agentCapabilitiesJSON the agents.capabilities value of reported capabilities
// (known ones only, JSON array).
func agentCapabilitiesJSON(reported []string) *string {
	caps := models.KnownAgentCapabilities(reported)
	if caps == nil {
		caps = []string{}
	}
	b, _ := json.Marshal(caps)
	s := string(b)
	return &s
}

// RegisterAgent handles agent registration
// @Summary Register a new agent
// @Description Register a new agent instance with Pool Token authentication (the pool token is accepted here only; every later call uses the returned agent token). The agent reports its version and capabilities (manifest_bundle_v1, task_data_overrides_v1, agent_token_v1); tasks of manifest-bound workspaces are only dispatched to agents reporting all three, otherwise they fail with error_code agent_upgrade_required. The response carries agent_token (per-agent JWT, typ agent, claims agent_id / pool_id / gen, header kid; lifetime AGENT_TOKEN_TTL, default 15m) and agent_token_expires_at; renew it with POST /api/v1/agents/token before it expires. Deregistering or revoking the agent, or revoking the pool token it registered with, invalidates it immediately (checked against the database on every use).
// @Tags Agent
// @Accept json
// @Produce json
// @Security PoolTokenAuth
// @Param request body models.AgentRegisterRequest true "Registration request"
// @Success 200 {object} map[string]interface{}
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
		// Capabilities (known ones only); older agents report none and
		// are not dispatched tasks of manifest-bound workspaces
		agent.Capabilities = agentCapabilitiesJSON(req.Capabilities)
		// the pool token this agent registered with: its agent tokens die
		// with it
		if pt, ok := c.Get("pool_token"); ok {
			if poolToken, ok := pt.(models.PoolToken); ok {
				hash := poolToken.TokenHash
				agent.PoolTokenHash = &hash
			}
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
	// Per-agent token. Without one (no SIGNING_ROOT_KEY in development
	// legacy mode) the agent keeps using the pool token, so it must not be
	// recorded as agent_token_v1 (pool-token calls on its tasks would be
	// refused).
	if tok, exp, err := services.IssueAgentToken(createdAgent.AgentID, poolIDStr, createdAgent.TokenGeneration); err == nil {
		response["agent_token"] = tok
		response["agent_token_expires_at"] = exp.UTC().Format(time.RFC3339)
	} else {
		log.Printf("[Agent] [WARN] no agent token for %s (%v); agent falls back to the pool token", createdAgent.AgentID, err)
		if createdAgent.HasCapability(models.AgentCapabilityAgentTokenV1) {
			var kept []string
			for _, capability := range models.KnownAgentCapabilities(req.Capabilities) {
				if capability != models.AgentCapabilityAgentTokenV1 {
					kept = append(kept, capability)
				}
			}
			h.db.Model(&models.Agent{}).Where("agent_id = ?", createdAgent.AgentID).
				Update("capabilities", agentCapabilitiesJSON(kept))
		}
	}

	c.JSON(http.StatusOK, response)
}

// PingAgent handles agent heartbeat.
// NOTE: Route is currently disabled (commented out in router). Not published in Swagger.
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
// @Security AgentTokenAuth
// @Security PoolTokenAuth
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
// @Security AgentTokenAuth
// @Security PoolTokenAuth
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

	// Unregister agent (deleting the row invalidates its agent token: every
	// use is checked against the agents row)
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

	// its run tokens too (also refused because their issuing agent is gone)
	if services.AgentRevocationHook != nil {
		if err := services.AgentRevocationHook(c.Request.Context(), h.db, agentID); err != nil {
			log.Printf("[Agent] revoking run tokens of unregistered agent %s: %v", agentID, err)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "agent unregistered successfully",
	})
}

// RenewAgentToken issues a fresh agent token
// @Summary Renew the agent token
// @Description Exchange a valid agent token (Authorization: Bearer <agent token>; pool tokens are refused) for a new one with a fresh expiry. Refused (401) once the agent was deregistered or revoked or its pool token revoked; the agent then registers again with the pool token.
// @Tags Agent
// @Produce json
// @Security AgentTokenAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/agents/token [post]
func (h *AgentHandler) RenewAgentToken(c *gin.Context) {
	agentID := middleware.TokenAgentID(c)
	if agentID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent token required"})
		return
	}
	tok, exp, err := services.IssueAgentToken(agentID, c.GetString("pool_id"), middleware.TokenAgentGeneration(c))
	if err != nil {
		log.Printf("[Agent] renew token for %s: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to issue agent token"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"agent_id":               agentID,
		"agent_token":            tok,
		"agent_token_expires_at": exp.UTC().Format(time.RFC3339),
	})
}

// GetTaskData retrieves all data needed for task execution
// @Summary Get task execution data
// @Description Get complete task data including workspace config (with manifest_deployment_id / manifest_active_tag / manifest_subpath), resources, variables, the task variable override snapshot (task.variable_overrides, task.override_sensitive_keys), Manifest Run files (task.external_files), the verified manifest bundle hand-off (manifest_bundle: archive_b64 + bundle_hash, or manifest_bundle_error when the version must be republished), and state
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
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

	// Load variables from snapshot (not live resolution)
	var variables []models.WorkspaceVariable
	if task.VariableSnapshotID != nil {
		snapshotSvc := services.NewVariableSnapshotService(h.db)
		vars, err := snapshotSvc.LoadFromSnapshot(*task.VariableSnapshotID)
		if err != nil {
			log.Printf("[Agent] Failed to load snapshot variables: %v", err)
			variables = []models.WorkspaceVariable{}
		} else {
			variables = vars
		}
	} else {
		variables = []models.WorkspaceVariable{}
	}

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

	// Build remote data config for agent (no tokens needed - reuses TF_HTTP_* env vars)
	var remoteDataConfig []gin.H
	for _, rd := range remoteDataList {
		remoteDataConfig = append(remoteDataConfig, gin.H{
			"remote_data_id":      rd.RemoteDataID,
			"source_workspace_id": rd.SourceWorkspaceID,
			"data_name":           rd.DataName,
		})
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

	// Dynamically resolve provider config from global templates if applicable
	providerConfig := workspace.ProviderConfig
	instances := workspace.ProviderInstances.GetProviderInstances()
	if len(instances) > 0 {
		ptService := services.NewProviderTemplateService(h.db)
		resolved, err := ptService.ResolveProviderConfigFromInstances(instances)
		if err != nil {
			log.Printf("[Agent] Failed to resolve provider config for workspace %s: %v", workspace.WorkspaceID, err)
		} else if resolved != nil {
			providerConfig = models.JSONB(resolved)
		}
	}

	// Build response
	taskData := gin.H{
		"id":           task.ID,
		"workspace_id": task.WorkspaceID,
		"task_type":    task.TaskType,
		"action":       task.TaskType,
		"context":      task.Context,
		"created_at":   task.CreatedAt,
		"plan_task_id": task.PlanTaskID, // 【修复】添加 plan_task_id 字段
		"agent_id":     task.AgentID,    // 【Phase 1优化】添加 agent_id 字段
	}
	workspaceData := gin.H{
		"workspace_id":       workspace.WorkspaceID,
		"name":               workspace.Name,
		"terraform_version":  workspace.TerraformVersion,
		"execution_mode":     workspace.ExecutionMode,
		"provider_config":    providerConfig,
		"tf_code":            workspace.TFCode,
		"system_variables":   workspace.SystemVariables,
		"terraform_lock_hcl": workspace.TerraformLockHCL, // 用于恢复 .terraform.lock.hcl 文件
	}
	response := gin.H{
		"task":            taskData,
		"workspace":       workspaceData,
		"resources":       resources,
		"variables":       variables,
		"outputs":         outputs,          // 【新增】添加 outputs 配置，用于生成 outputs.tf.json
		"remote_data":     remoteDataConfig, // 【新增】添加 remote_data 配置，用于生成 remote_data.tf.json
		"module_versions": moduleVersions,   // 【新增】添加 module_versions，用于 Agent 模式下补充 tf_code 中缺失的 version 字段
	}

	// Manifest: override snapshot, Run draft files, manifest fields and the
	// verified bundle hand-off (or the reason it was refused).
	services.AddManifestTaskData(c.Request.Context(), h.db, &task, &workspace, taskData, workspaceData, response)

	// Add state version ONLY if it actually exists in database
	if hasStateVersion {
		response["state_version"] = gin.H{
			"version":  stateVersion.Version,
			"content":  stateVersion.Content,
			"checksum": stateVersion.Checksum,
			"size":     stateVersion.SizeBytes,
		}
	}

	// Generate state backend token for Agent/K8s mode HTTP state backend
	if h.stateTokenService != nil {
		stateToken, tokenErr := h.stateTokenService.GenerateToken(workspace.WorkspaceID, task.ID)
		if tokenErr != nil {
			log.Printf("WARNING: Failed to generate state token for task %d: %v", task.ID, tokenErr)
		} else {
			platformConfigService := services.NewPlatformConfigService(h.db)
			serverURL := platformConfigService.GetBaseURL()
			response["state_backend"] = gin.H{
				"url":   fmt.Sprintf("%s/api/v1/terraform/state/%s", serverURL, workspace.WorkspaceID),
				"token": stateToken,
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// UploadTaskLogChunk handles incremental log upload
// @Summary Upload task log chunk
// @Description Upload a chunk of task log data incrementally
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Log chunk data with phase, content, offset, checksum"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
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
	h.db.Omit("state_token_hash").Save(&task)

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
// @Description Update the status of a running task, triggers run triggers and CMDB sync on completion
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Status update with status, stage, error_message, error_code (only known structured codes such as bundle_republish_required, plan_expired, bundle_hash_mismatch are stored), error_reason (short rule name stored with a known error_code), changes, duration, etc. A bundle_hash_mismatch report (the bundle the executor received did not hash to bundle_hash) is audited as version.bundle_hash_mismatch with source agent; the platform re-verifies the stored files and marks the version hash_mismatch only if its own check fails."
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
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
		ErrorCode      string                 `json:"error_code"`   // structured failure code; only models.KnownTaskErrorCode values are stored
		ErrorReason    string                 `json:"error_reason"` // short rule name next to a known error_code (e.g. hash_mismatch); tokens only
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

	// Only a running task changes state. Once the agent has ended it
	// (RequireTaskAgent lets the task's agent report for a grace period), only
	// a retry of that same final status is accepted.
	if task.Status != models.TaskStatusRunning && req.Status != task.Status {
		c.JSON(http.StatusConflict, gin.H{
			"error":          "task is no longer running, status update rejected",
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
	if models.KnownTaskErrorCode(req.ErrorCode) {
		updates["error_code"] = req.ErrorCode
		if manifestbundle.IsReasonToken(req.ErrorReason) {
			updates["error_reason"] = req.ErrorReason
		}
		// agent-side hand-off integrity failure (bundle_hash_mismatch; older
		// agents: bundle_republish_required + hash_mismatch). The agent is not
		// trusted to mark the version: audit (source agent) and re-verify the
		// stored files; the version is marked only if that check fails. Only
		// on the first report (not on status retries).
		if task.Status == models.TaskStatusRunning && (req.ErrorCode == models.TaskErrorCodeBundleHashMismatch ||
			(req.ErrorCode == models.TaskErrorCodeBundleRepublishRequired && req.ErrorReason == manifestbundle.ReasonHashMismatch)) {
			agentID := middleware.TokenAgentID(c)
			if agentID == "" && task.AgentID != nil {
				agentID = *task.AgentID
			}
			if _, err := services.RecordAgentBundleHashMismatch(c.Request.Context(), h.db, &task, agentID); err != nil {
				log.Printf("[UpdateTaskStatus] task %d: bundle mismatch re-check: %v", taskID, err)
			}
		}
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
	// Agent 可能运行在不同时区（如 UTC），而 DB 列是 timestamp without time zone，
	// pgx 使用 wall clock 值存储。因此必须将 Agent 发来的时间转为服务端本地时区，
	// 确保存储的 wall clock 与 created_at/started_at 一致。
	if req.CompletedAt != nil {
		localTime := req.CompletedAt.In(time.Local)
		updates["completed_at"] = &localTime
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

	// Revoke state token on terminal status
	task.Status = req.Status
	if task.IsTerminal() {
		// a terminal task never applies its plan: delete plan_data now
		// (after a successful apply in particular)
		if err := services.ClearPlanData(h.db, task.ID); err != nil {
			log.Printf("WARNING: Failed to delete plan_data of task %d: %v", task.ID, err)
		}
	}
	if task.IsTerminal() && h.stateTokenService != nil {
		if err := h.stateTokenService.RevokeToken(task.ID); err != nil {
			log.Printf("WARNING: Failed to revoke state token for task %d: %v", task.ID, err)
		}
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

// SaveTaskState - REMOVED: State is now managed via HTTP state backend (POST /state/{workspace_id})

// ============================================================================
// New handlers for Agent Mode refactoring
// ============================================================================

// GetPlanTask retrieves a plan task by ID
// @Summary Get plan task
// @Description Get plan task information for agent execution, including plan_data for apply tasks (decrypted for the executing agent only; omitted when expired) and snapshot resources/variables
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	if task.AgentID != nil {
		log.Printf("[Agent] Task %d agent_id: %s", taskID, *task.AgentID)
	} else {
		log.Printf("[Agent] Task %d agent_id: nil", taskID)
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
	// Load variables from snapshot table
	if task.VariableSnapshotID != nil {
		snapshotSvc := services.NewVariableSnapshotService(h.db)
		vars, err := snapshotSvc.LoadFromSnapshot(*task.VariableSnapshotID)
		if err != nil {
			log.Printf("[Agent] Failed to load snapshot variables for apply: %v", err)
		} else {
			varList := make([]gin.H, 0, len(vars))
			for _, v := range vars {
				varList = append(varList, gin.H{
					"workspace_id":  task.WorkspaceID,
					"variable_id":   v.VariableID,
					"version":       v.Version,
					"variable_type": v.VariableType,
					"key":           v.Key,
					"value":         v.Value,
					"sensitive":     v.Sensitive,
					"description":   v.Description,
					"value_format":  v.ValueFormat,
				})
			}
			taskResponse["snapshot_variables"] = varList
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

	// Include plan_data if it exists. At rest it is an envelope
	// (services.SealTaskPlanData); it is decrypted only here, for the agent
	// executing this task (pool token + task check), and sent base64.
	// Expired / undecryptable plans are omitted: the agent's apply then fails
	// with "plan data is empty" (plan_expired).
	if len(task.PlanData) > 0 {
		if plain, err := services.OpenTaskPlanData(&task); err != nil {
			log.Printf("[WARN] Task %d: plan_data not handed to agent: %v", task.ID, err)
		} else {
			response["plan_data"] = base64.StdEncoding.EncodeToString(plain)
		}
	}

	c.JSON(http.StatusOK, response)
}

// UploadPlanData handles plan data upload from agent
// @Summary Upload plan data
// @Description Upload base64-encoded plan data from agent after plan execution; stored encrypted at rest (envelope, per-plan key) and deleted after apply / on expiry. Triggers post_plan Run Tasks.
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Plan data (base64 encoded in plan_data field)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	// Store the plan encrypted at rest (same envelope as Local mode)
	sealed, err := services.SealTaskPlanData(task.ID, decodedData)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to encrypt plan_data"})
		return
	}
	if err := h.db.Model(&task).UpdateColumn("plan_data", sealed).Error; err != nil {
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
				log.Printf("[RunTask] Failed to reload task %d for post_plan Run Tasks: %v", taskID, err)
				return
			}

			log.Printf("[RunTask] Executing post_plan Run Tasks for task %d (Agent mode, after plan_data upload)", taskID)

			// 执行 post_plan Run Tasks
			// 【重要】使用 context.Background() 而不是 c.Request.Context()
			// 因为 goroutine 是异步执行的，HTTP 请求完成后 c.Request.Context() 会被取消
			// 这会导致 webhook 请求失败（context canceled）
			ctx := context.Background()
			passed, err := h.runTaskExecutor.ExecuteRunTasksForStage(ctx, &taskForRunTask, models.RunTaskStagePostPlan)
			if err != nil {
				log.Printf("[RunTask] post_plan Run Tasks execution error for task %d: %v", taskID, err)
				// 更新任务状态为失败
				h.db.Model(&taskForRunTask).Updates(map[string]interface{}{
					"status":        models.TaskStatusFailed,
					"error_message": fmt.Sprintf("Post-plan Run Task execution error: %v", err),
				})
				return
			}

			if !passed {
				log.Printf("[RunTask] post_plan Run Tasks blocked execution for task %d (mandatory task failed)", taskID)
				// 更新任务状态为失败
				h.db.Model(&taskForRunTask).Updates(map[string]interface{}{
					"status":        models.TaskStatusFailed,
					"error_message": "Post-plan Run Task failed (mandatory)",
				})
				return
			}

			log.Printf("[RunTask] post_plan Run Tasks completed successfully for task %d", taskID)
		}()
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "plan_data uploaded successfully",
		"size":    len(decodedData),
	})
}

// UploadPlanJSON handles plan JSON upload from agent
// @Summary Upload plan JSON
// @Description Upload plan JSON from agent after plan execution. Sensitive values are redacted (services.RedactPlanJSON) before the plan is stored, and the task's resource changes are derived from the redacted plan by the platform. Only the task's agent may call this, while the task is running (403 / 409 otherwise).
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} true "Plan JSON in plan_json field"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	// Store the plan_json — redacted on the platform side as well, so an agent
	// that uploads a raw plan (older build) never gets sensitive values stored
	// with the platform-side sensitive set of this task (snapshot + overrides)
	ps, psErr := services.PlanSensitivityForTask(h.db, &task)
	if psErr != nil {
		log.Printf("[WARN] plan redaction for task %d: platform sensitivity incomplete: %v", task.ID, psErr)
	}
	redacted := services.RedactPlanJSON(req.PlanJSON, ps)
	if err := h.db.Model(&task).Update("plan_json", models.JSONB(redacted)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save plan_json: " + err.Error(),
		})
		return
	}

	// Resource changes are derived here, by the platform, from the redacted
	// plan (agents no longer upload them; see ParsePlanChanges).
	task.PlanJSON = redacted
	if n, err := services.NewPlanParserService(h.db).StoreResourceChangesFromPlanJSON(&task); err != nil {
		log.Printf("[WARN] task %d: deriving resource changes from plan_json failed: %v", task.ID, err)
	} else {
		log.Printf("[UploadPlanJSON] task %d: derived %d resource changes from the redacted plan_json", task.ID, n)
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "plan_json uploaded successfully",
	})
}

// LockWorkspace locks a workspace
// @Summary Lock workspace
// @Description Lock a workspace for exclusive access during task execution
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Lock request with user_id and optional reason"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	lockID := fmt.Sprintf("%d", time.Now().UnixNano())
	lockInfo := models.JSONB{
		"ID":          lockID,
		"operation":   "agent_lock",
		"who":         req.UserID,
		"who_display": req.UserID,
		"info":        req.Reason,
		"created":     time.Now().Format(time.RFC3339),
	}

	updates := map[string]interface{}{
		"lock_id":   lockID,
		"lock_info": lockInfo,
	}

	// Atomic lock: only succeed if workspace is currently unlocked
	result := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ? AND lock_id IS NULL", workspaceID).
		Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to lock workspace"})
		return
	}
	if result.RowsAffected == 0 {
		// Already locked - read current lock for error message
		var ws models.Workspace
		h.db.Select("lock_info").Where("workspace_id = ?", workspaceID).First(&ws)
		c.JSON(http.StatusConflict, gin.H{
			"error":     "workspace is already locked",
			"lock_info": ws.LockInfo,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace locked"})
}

// UnlockWorkspace unlocks a workspace
// @Summary Unlock workspace
// @Description Unlock a workspace after task execution completes
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/unlock [post]
func (h *AgentHandler) UnlockWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	updates := map[string]interface{}{
		"lock_id":   nil,
		"lock_info": nil,
	}

	if err := h.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to unlock workspace"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace unlocked"})
}

// ParsePlanChanges derives the task's resource changes on the platform
// @Summary Derive resource changes
// @Description Resource changes are derived by the platform from the task's stored, redacted plan_json (the same plan parser path as local execution; also done when the plan JSON is uploaded). Any resource_changes in the body are accepted and discarded: agent-sent before/after values are never stored. When the task has no plan_json yet (an older agent whose plan upload failed), only address/type/name/module/action of the uploaded entries are stored, with before/after/after_unknown NULL. Only the task's agent may call this (403 otherwise; 409 when the task is not running or just ended).
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Param request body map[string]interface{} false "Ignored (older agents send resource_changes; their values are discarded)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/tasks/{task_id}/parse-plan-changes [post]
func (h *AgentHandler) ParsePlanChanges(c *gin.Context) {
	var taskID uint
	if _, err := fmt.Sscanf(c.Param("task_id"), "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	// Older agents upload their own parse; only the identifying fields are
	// read (and used only when there is no plan_json), never the values.
	var req struct {
		ResourceChanges []struct {
			ResourceAddress string `json:"resource_address"`
			ResourceType    string `json:"resource_type"`
			ResourceName    string `json:"resource_name"`
			ModuleAddress   string `json:"module_address"`
			Action          string `json:"action"`
		} `json:"resource_changes"`
	}
	if c.Request.ContentLength != 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
			return
		}
	}

	var task models.WorkspaceTask
	if err := h.db.Omit("plan_data").First(&task, taskID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	parser := services.NewPlanParserService(h.db)
	if len(task.PlanJSON) > 0 {
		n, err := parser.StoreResourceChangesFromPlanJSON(&task)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to derive resource changes: " + err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"message":                    "resource changes derived from plan_json",
			"source":                     "plan_json",
			"count":                      n,
			"uploaded_changes_discarded": len(req.ResourceChanges),
		})
		return
	}

	metas := make([]services.ResourceChangeMeta, 0, len(req.ResourceChanges))
	for _, rc := range req.ResourceChanges {
		metas = append(metas, services.ResourceChangeMeta{
			ResourceAddress: rc.ResourceAddress, ResourceType: rc.ResourceType, ResourceName: rc.ResourceName,
			ModuleAddress: rc.ModuleAddress, Action: rc.Action,
		})
	}
	n, err := parser.StoreResourceChangeMetadata(&task, metas)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save resource changes: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"message": "no plan_json: resource changes stored without values",
		"source":  "metadata_only",
		"count":   n,
	})
}

// GetTaskLogs retrieves task logs
// @Summary Get task logs
// @Description Get all logs for a task ordered by creation time
// @Tags Agent Task
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Tags Agent Terraform Version
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Tags Agent Terraform Version
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param version path string true "Version string (e.g., 1.5.0)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Description Update specific whitelisted fields of a workspace (last_init_hash, last_init_terraform_version, terraform_lock_hcl)
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Fields to update (only whitelisted fields accepted)"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Lock HCL content with terraform_lock_hcl field"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

// GetManifestProviderSchemaMeta returns version fingerprint for workspace's manifest+subpath schema cache
// @Summary Get manifest provider schema meta
// @Tags Agent Workspace
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/manifest-provider-schema/meta [get]
func (h *AgentHandler) GetManifestProviderSchemaMeta(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	accessor := services.NewLocalDataAccessor(h.db)
	meta, err := accessor.GetManifestProviderSchemaMetaByWorkspace(workspaceID)
	if err != nil {
		// 区分「非 manifest 托管」与其它错误，避免 Agent 误跑 capture
		if strings.Contains(err.Error(), "not manifest-managed") {
			c.JSON(http.StatusOK, gin.H{"exists": false, "managed": false})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if meta == nil {
		c.JSON(http.StatusOK, gin.H{"exists": false, "managed": true})
		return
	}
	// ProviderVersionsKey 为空 = 托管但尚无缓存行
	exists := meta.ProviderVersionsKey != ""
	c.JSON(http.StatusOK, gin.H{
		"exists":                exists,
		"managed":               true,
		"manifest_id":           meta.ManifestID,
		"subpath":               meta.Subpath,
		"schema_kind":           meta.SchemaKind,
		"provider_versions_key": meta.ProviderVersionsKey,
		"content_hash":          meta.ContentHash,
	})
}

// UpsertManifestProviderSchema saves types catalog from agent post_init
// @Summary Upsert manifest provider schema
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/manifest-provider-schema [put]
func (h *AgentHandler) UpsertManifestProviderSchema(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var req struct {
		SchemaKind          string          `json:"schema_kind"`
		Providers           json.RawMessage `json:"providers"`
		ProviderVersionsKey string          `json:"provider_versions_key" binding:"required"`
		Resources           json.RawMessage `json:"resources"`
		DataSources         json.RawMessage `json:"data_sources"`
		ContentHash         string          `json:"content_hash"`
		TerraformVersion    string          `json:"terraform_version"`
		SourceTaskID        *uint           `json:"source_task_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body: " + err.Error()})
		return
	}
	if req.SchemaKind == "" {
		req.SchemaKind = models.ManifestProviderSchemaKindTypes
	}

	row := &models.ManifestProviderSchema{
		SchemaKind:          req.SchemaKind,
		Providers:           req.Providers,
		ProviderVersionsKey: req.ProviderVersionsKey,
		Resources:           req.Resources,
		DataSources:         req.DataSources,
		ContentHash:         req.ContentHash,
		TerraformVersion:    req.TerraformVersion,
		SourceWorkspaceID:   workspaceID,
		SourceTaskID:        req.SourceTaskID,
		CapturedAt:          time.Now().UTC(),
	}
	if len(row.Providers) == 0 {
		row.Providers = json.RawMessage("[]")
	}

	accessor := services.NewLocalDataAccessor(h.db)
	if err := accessor.UpsertManifestProviderSchemaByWorkspace(workspaceID, row); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to upsert provider schema: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "manifest provider schema saved"})
}

// UpsertTempState handles temp state upsert from agent
// @Summary Upsert temporary state
// @Description Create or update a temporary state version for a workspace (used by state file watcher)
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Temp state data with content, checksum, size_bytes, version, task_id, created_by"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/state/temp [put]
func (h *AgentHandler) UpsertTempState(c *gin.Context) {
	log.Printf("DEPRECATED: UpsertTempState called - this endpoint will be removed. State is now managed via HTTP state backend.")
	workspaceID := c.Param("workspace_id")

	var req struct {
		Content   map[string]interface{} `json:"content" binding:"required"`
		Checksum  string                 `json:"checksum"`
		SizeBytes int                    `json:"size_bytes"`
		Version   int                    `json:"version"`
		TaskID    *uint                  `json:"task_id"`
		CreatedBy *string                `json:"created_by"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version := &models.WorkspaceStateVersion{
		WorkspaceID: workspaceID,
		Content:     req.Content,
		Checksum:    req.Checksum,
		SizeBytes:   req.SizeBytes,
		Version:     req.Version,
		TaskID:      req.TaskID,
		CreatedBy:   req.CreatedBy,
		IsTemp:      true,
	}

	accessor := services.NewLocalDataAccessor(h.db)
	if err := accessor.UpsertTempState(version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"id": version.ID, "message": "temp state upserted"})
}

// PromoteTempState handles temp state promotion from agent
// @Summary Promote temporary state
// @Description Promote a temporary state version to a permanent state version
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Param request body map[string]interface{} true "Promotion request with record_id"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/state/promote [post]
func (h *AgentHandler) PromoteTempState(c *gin.Context) {
	log.Printf("DEPRECATED: PromoteTempState called - this endpoint will be removed. State is now managed via HTTP state backend.")
	workspaceID := c.Param("workspace_id")

	var req struct {
		RecordID uint `json:"record_id" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	accessor := services.NewLocalDataAccessor(h.db)
	if err := accessor.PromoteTempState(workspaceID, req.RecordID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "temp state promoted"})
}

// CleanupOrphanedTempStates handles orphaned temp state cleanup from agent
// @Summary Cleanup orphaned temporary states
// @Description Delete orphaned temporary state versions for a workspace
// @Tags Agent Workspace
// @Accept json
// @Produce json
// @Security AgentTokenAuth
// @Security PoolTokenAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/agents/workspaces/{workspace_id}/state/temp [delete]
func (h *AgentHandler) CleanupOrphanedTempStates(c *gin.Context) {
	log.Printf("DEPRECATED: CleanupOrphanedTempStates called - this endpoint will be removed. State is now managed via HTTP state backend.")
	workspaceID := c.Param("workspace_id")

	accessor := services.NewLocalDataAccessor(h.db)
	if err := accessor.CleanupOrphanedTempStates(workspaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "orphaned temp states cleaned up"})
}

// IssueRunToken gives the calling agent the token of a manifest run assigned to it
// @Summary Obtain a manifest run token
// @Description Agent token only (pool tokens are refused). Returns a run token (JWT typ run, signing purpose run; claims run_id, workspace_id, purpose, session_id, agent_id) for a runner=agent manifest run in pending/running state assigned to the calling agent. It authenticates the Terraform HTTP state backend of the run's workspace only; preview tokens are read-only (POST/LOCK/UNLOCK refused). Expiry = min(run created_at + MANIFEST_RUN_TIMEOUT, session expiry). Every use is checked against the database (refused on DB error). Revoked when the run ends, its session ends or expires, or the agent is revoked or deregistered.
// @Tags Agent
// @Produce json
// @Security AgentTokenAuth
// @Param run_id path string true "Manifest run ID"
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 503 {object} map[string]interface{}
// @Router /api/v1/agents/runs/{run_id}/token [post]
func (h *AgentHandler) IssueRunToken(c *gin.Context) {
	agentID := middleware.TokenAgentID(c)
	if agentID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "agent token required"})
		return
	}
	if h.stateTokenService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "state token service not configured"})
		return
	}
	runID := c.Param("run_id")
	tok, exp, err := h.stateTokenService.IssueRunToken(c.Request.Context(), runID, agentID)
	if err != nil {
		if errors.Is(err, services.ErrRunNotAssigned) {
			c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
			return
		}
		log.Printf("[Agent] run token for run %s (agent %s): %v", runID, agentID, err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "run token could not be issued"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"run_id":     runID,
		"run_token":  tok,
		"expires_at": exp.UTC().Format(time.RFC3339),
	})
}
