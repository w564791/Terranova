package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"
	"iac-platform/internal/websocket"
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupAgentAPIRoutes sets up Agent API routes (使用 Pool Token 认证，不需要 JWT)
func setupAgentAPIRoutes(api *gin.RouterGroup, db *gorm.DB, streamManager *services.OutputStreamManager, agentMetricsHub *websocket.AgentMetricsHub, runTaskExecutor *services.RunTaskExecutor, queueManager *services.TaskQueueManager) {
	// Initialize handlers
	agentHandler := handlers.NewAgentHandler(db, streamManager, agentMetricsHub)

	// 注入 Run Task 执行器（用于 Agent 模式下的 post_plan Run Tasks）
	if runTaskExecutor != nil {
		agentHandler.SetRunTaskExecutor(runTaskExecutor)
	}

	// 注入任务队列管理器（用于 Agent 模式下 CMDB 同步等 server 侧逻辑）
	if queueManager != nil {
		agentHandler.SetTaskQueueManager(queueManager)
	}
	agentPoolSecretsHandler := handlers.NewAgentPoolSecretsHandler(db)

	// ===== Agent Management Routes (with Pool Token auth for v3.2) =====
	agents := api.Group("/agents")
	{
		// Agent registration and management (requires Pool Token)
		agents.POST("/register", middleware.PoolTokenAuthMiddleware(db), agentHandler.RegisterAgent)

		// Get pool HCP secrets (for generating credentials.tfrc.json on agent side)
		agents.GET("/pool/secrets", middleware.PoolTokenAuthMiddleware(db), agentPoolSecretsHandler.GetPoolSecrets)
		// agents.POST("/:agent_id/ping", middleware.PoolTokenAuthMiddleware(db), agentHandler.PingAgent)
		agents.GET("/:agent_id", middleware.PoolTokenAuthMiddleware(db), agentHandler.GetAgent)
		agents.DELETE("/:agent_id", middleware.PoolTokenAuthMiddleware(db), agentHandler.UnregisterAgent)

		// C&C WebSocket endpoint has been moved to a standalone server
		// Agent should connect to port 8091 instead of 8080
		// See: backend/internal/handlers/agent_cc_handler_raw.go
		// This endpoint is kept for backward compatibility but will return an error
		agents.GET("/control", func(c *gin.Context) {
			c.JSON(http.StatusGone, gin.H{
				"error":        "WebSocket endpoint has moved",
				"message":      "Please connect to port 8091 for Agent C&C WebSocket",
				"new_endpoint": "ws://server:8091/api/v1/agents/control",
			})
		})

		// Note: Agent C&C status API has been deprecated
		// C&C functionality is now handled by the standalone WebSocket server (rawCCHandler)
		// If you need to debug C&C connections, check the server logs
	}

	// ===== Agent Task API Routes (for Agent v3.2) =====
	// These routes now include workspace authorization check to ensure agents can only access
	// tasks from workspaces they are authorized to access
	agentTasks := api.Group("/agents/tasks")
	{
		// Get task execution data
		agentTasks.GET("/:task_id/data", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.GetTaskData)

		// Upload log chunk
		agentTasks.POST("/:task_id/logs/chunk", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.UploadTaskLogChunk)

		// Update task status
		agentTasks.PUT("/:task_id/status", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.UpdateTaskStatus)

		// Save state version
		agentTasks.POST("/:task_id/state", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.SaveTaskState)

		// New endpoints for Agent Mode refactoring
		agentTasks.GET("/:task_id/plan-task", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.GetPlanTask)
		agentTasks.POST("/:task_id/plan-data", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.UploadPlanData)
		agentTasks.POST("/:task_id/plan-json", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.UploadPlanJSON)
		agentTasks.POST("/:task_id/parse-plan-changes", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.ParsePlanChanges)
		agentTasks.GET("/:task_id/logs", middleware.PoolTokenAuthWithTaskCheck(db), agentHandler.GetTaskLogs)
	}

	// ===== Agent Workspace API Routes (for Agent v3.2) =====
	// These routes now include workspace authorization check
	agentWorkspaces := api.Group("/agents/workspaces")
	{
		// Workspace locking
		agentWorkspaces.POST("/:workspace_id/lock", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.LockWorkspace)
		agentWorkspaces.POST("/:workspace_id/unlock", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.UnlockWorkspace)

		// State version management
		agentWorkspaces.GET("/:workspace_id/state/max-version", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.GetMaxStateVersion)

		// Update workspace fields (for init optimization - last_init_hash, etc.)
		agentWorkspaces.PATCH("/:workspace_id/fields", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.UpdateWorkspaceFields)

		// Terraform lock hcl management (for init optimization - .terraform.lock.hcl)
		agentWorkspaces.GET("/:workspace_id/terraform-lock-hcl", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.GetTerraformLockHCL)
		agentWorkspaces.PUT("/:workspace_id/terraform-lock-hcl", middleware.PoolTokenAuthWithWorkspaceCheck(db), agentHandler.SaveTerraformLockHCL)
	}

	// ===== Agent Terraform Version API Routes =====
	// These routes allow agents to query terraform version configurations
	agentTerraformVersions := api.Group("/agents/terraform-versions")
	{
		// Get default terraform version
		agentTerraformVersions.GET("/default", middleware.PoolTokenAuthMiddleware(db), agentHandler.GetDefaultTerraformVersion)

		// Get specific terraform version by version string
		agentTerraformVersions.GET("/:version", middleware.PoolTokenAuthMiddleware(db), agentHandler.GetTerraformVersionByVersion)
	}
}

// setupAgentPoolRoutes sets up Agent Pool management routes (需要 JWT 认证)
func setupAgentPoolRoutes(adminProtected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// Initialize handlers
	agentPoolHandler := handlers.NewAgentPoolHandler(db)
	poolAuthHandler := handlers.NewPoolAuthorizationHandler(db)

	// ===== Agent Pool Management Routes (with IAM permissions) =====
	agentPools := adminProtected.Group("/agent-pools")
	{
		// Create agent pool
		agentPools.POST("",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.CreateAgentPool,
		)

		// List agent pools
		agentPools.GET("",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "READ"),
			agentPoolHandler.ListAgentPools,
		)

		// Get agent pool details
		agentPools.GET("/:pool_id",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "READ"),
			agentPoolHandler.GetAgentPool,
		)

		// Update agent pool
		agentPools.PUT("/:pool_id",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.UpdateAgentPool,
		)

		// Delete agent pool
		agentPools.DELETE("/:pool_id",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "ADMIN"),
			agentPoolHandler.DeleteAgentPool,
		)

		// Pool authorization - Pool side
		agentPools.POST("/:pool_id/allow-workspaces",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			poolAuthHandler.AllowWorkspaces,
		)

		agentPools.GET("/:pool_id/allowed-workspaces",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "READ"),
			poolAuthHandler.GetAllowedWorkspaces,
		)

		agentPools.DELETE("/:pool_id/allowed-workspaces/:workspace_id",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			poolAuthHandler.RevokeWorkspaceAccess,
		)

		// Pool Token Management
		agentPools.POST("/:pool_id/tokens",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.CreatePoolToken,
		)

		agentPools.GET("/:pool_id/tokens",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "READ"),
			agentPoolHandler.ListPoolTokens,
		)

		agentPools.DELETE("/:pool_id/tokens/:token_name",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.RevokePoolToken,
		)

		// Rotate pool token (for K8s pools)
		agentPools.POST("/:pool_id/tokens/:token_name/rotate",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.RotatePoolToken,
		)

		// Sync deployment config (for K8s pools)
		agentPools.POST("/:pool_id/sync-deployment",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.SyncDeploymentConfig,
		)

		// Activate one-time unfreeze (for K8s pools)
		agentPools.POST("/:pool_id/one-time-unfreeze",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.ActivateOneTimeUnfreeze,
		)

		// K8s Configuration Management
		agentPools.PUT("/:pool_id/k8s-config",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "WRITE"),
			agentPoolHandler.UpdateK8sConfig,
		)

		agentPools.GET("/:pool_id/k8s-config",
			iamMiddleware.RequirePermission("AGENT_POOLS", "ORGANIZATION", "READ"),
			agentPoolHandler.GetK8sConfig,
		)
	}
}

// setupWorkspaceAgentRoutes sets up workspace-pool authorization routes
// These routes are added to workspace routes for workspace-side pool management
func setupWorkspaceAgentRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	poolAuthHandler := handlers.NewPoolAuthorizationHandler(db)

	// Note: Agent-level authorization routes have been removed.
	// The system now uses Pool-level authorization.

	// Pool-level authorization - Workspace side
	workspaces.GET("/:id/available-pools",
		func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "workspace_id", Value: c.Param("id")})
			c.Next()
		},
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		poolAuthHandler.GetAvailablePools,
	)

	workspaces.POST("/:id/set-current-pool",
		func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "workspace_id", Value: c.Param("id")})
			c.Next()
		},
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		poolAuthHandler.SetCurrentPool,
	)

	workspaces.GET("/:id/current-pool",
		func(c *gin.Context) {
			c.Params = append(c.Params, gin.Param{Key: "workspace_id", Value: c.Param("id")})
			c.Next()
		},
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		poolAuthHandler.GetCurrentPool,
	)
}
