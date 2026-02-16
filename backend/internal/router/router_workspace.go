package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/application/service"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"
	"iac-platform/internal/websocket"
	"iac-platform/services"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupWorkspaceRoutes sets up workspace routes
// 包括: workspaces, tasks, state, variables, resources
func setupWorkspaceRoutes(api *gin.RouterGroup, db *gorm.DB, streamManager *services.OutputStreamManager, iamMiddleware *middleware.IAMPermissionMiddleware, wsHub *websocket.Hub, queueManager *services.TaskQueueManager, agentCCHandler *handlers.RawAgentCCHandler, permissionService service.PermissionService) {
	// TODO: 实现workspace路由
	// 参考原router.go中的 workspaces := api.Group("/workspaces") 部分
	workspaces := api.Group("/workspaces")
	workspaces.Use(middleware.JWTAuth())
	workspaces.Use(middleware.AuditLogger(db))
	{
		workspaceController := controllers.NewWorkspaceController(
			services.NewWorkspaceService(db),
			services.NewWorkspaceOverviewService(db),
			permissionService,
		)
		// helperController := controllers.NewWorkspaceHelperController(
		// 	services.NewTerraformVersionService(db),
		// 	services.NewAgentPoolService(db, services.NewAgentService(db)),
		// )
		taskController := controllers.NewWorkspaceTaskController(db, streamManager, queueManager, agentCCHandler)
		stateController := controllers.NewStateVersionController(db)
		variableController := controllers.NewWorkspaceVariableController(services.NewWorkspaceVariableService(db))
		resourceController := controllers.NewResourceController(db, streamManager)
		lifecycleService := services.NewWorkspaceLifecycleService(db)

		// 工作空间列表和详情 - Admin绕过检查，非admin需要IAM权限
		workspaces.GET("",
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ"),
			workspaceController.GetWorkspaces,
		)

		// Workspace basic operations - READ level
		workspaces.GET("/:id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			workspaceController.GetWorkspace,
		)

		workspaces.GET("/:id/overview",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			workspaceController.GetWorkspaceOverview,
		)

		// Workspace basic operations - WRITE level
		workspaces.PUT("/:id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			workspaceController.UpdateWorkspace,
		)

		workspaces.PATCH("/:id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			workspaceController.UpdateWorkspace,
		)

		workspaces.POST("/:id/lock",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			func(c *gin.Context) {
				workspaceID := c.Param("id")
				userID, _ := c.Get("user_id")
				var req struct {
					Reason string `json:"reason" binding:"required"`
				}
				if err := c.ShouldBindJSON(&req); err != nil {
					c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
					return
				}
				if err := lifecycleService.LockWorkspace(workspaceID, userID.(string), req.Reason); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Workspace locked successfully"})
			},
		)

		workspaces.POST("/:id/unlock",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			func(c *gin.Context) {
				workspaceID := c.Param("id")
				if err := lifecycleService.UnlockWorkspace(workspaceID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Workspace unlocked successfully"})
			},
		)

		// Workspace basic operations - ADMIN level
		workspaces.DELETE("/:id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			workspaceController.DeleteWorkspace,
		)

		// Other workspace operations - 添加IAM权限检查
		// workspaces.GET("/form-data", func(c *gin.Context) {
		// 	role, _ := c.Get("role")
		// 	if role == "admin" {
		// 		helperController.GetWorkspaceFormData(c)
		// 		return
		// 	}
		// 	iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ")(c)
		// 	if !c.IsAborted() {
		// 		helperController.GetWorkspaceFormData(c)
		// 	}
		// })

		workspaces.POST("",
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "WRITE"),
			workspaceController.CreateWorkspace,
		)
		// Task operations - READ level (精细化权限优先)
		workspaces.GET("/:id/tasks",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			taskController.GetTasks,
		)

		workspaces.GET("/:id/tasks/:task_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			taskController.GetTask,
		)

		workspaces.GET("/:id/tasks/:task_id/logs",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			taskController.GetTaskLogs,
		)

		workspaces.GET("/:id/tasks/:task_id/comments",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			taskController.GetComments,
		)

		workspaces.GET("/:id/tasks/:task_id/resource-changes",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			func(c *gin.Context) {
				controllers.GetTaskResourceChangesWithDB(c, db)
			},
		)

		aiController := controllers.NewAIController(db)
		workspaces.GET("/:id/tasks/:task_id/error-analysis",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			aiController.GetTaskAnalysis,
		)

		workspaces.GET("/:id/tasks/:task_id/state-backup",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			taskController.DownloadStateBackup,
		)

		// Task operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/tasks/plan",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			taskController.CreatePlanTask,
		)

		workspaces.POST("/:id/tasks/:task_id/comments",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			taskController.CreateComment,
		)

		// Task operations - ADMIN level (精细化权限优先)
		workspaces.POST("/:id/tasks/:task_id/cancel",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			taskController.CancelTask,
		)

		workspaces.POST("/:id/tasks/:task_id/cancel-previous",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			taskController.CancelPreviousTasks,
		)

		workspaces.POST("/:id/tasks/:task_id/confirm-apply",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			taskController.ConfirmApply,
		)

		workspaces.PATCH("/:id/tasks/:task_id/resource-changes/:resource_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			func(c *gin.Context) {
				controllers.UpdateResourceApplyStatusWithDB(c, db)
			},
		)

		workspaces.POST("/:id/tasks/:task_id/retry-state-save",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			taskController.RetryStateSave,
		)

		workspaces.POST("/:id/tasks/:task_id/parse-plan",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			func(c *gin.Context) {
				controllers.ManualParsePlanWithDB(c, db)
			},
		)
		// State operations - READ level (精细化权限优先)
		stateHandler := handlers.NewStateHandler(services.NewStateService(db))

		workspaces.GET("/:id/current-state",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateController.GetCurrentState,
		)

		workspaces.GET("/:id/state-versions",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateController.GetStateVersions,
		)

		// New: Get state version history (with pagination)
		workspaces.GET("/:id/state/versions",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateHandler.GetStateVersions,
		)

		// New: Get specific state version (metadata only, no content)
		workspaces.GET("/:id/state/versions/:version",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateHandler.GetStateVersion,
		)

		// New: Retrieve state version content (requires WORKSPACE_STATE_SENSITIVE permission)
		// This endpoint returns the full state content including sensitive data
		// Requires WORKSPACE_STATE_SENSITIVE permission to access state content
		workspaces.GET("/:id/state/versions/:version/retrieve",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_STATE_SENSITIVE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			stateHandler.RetrieveStateVersion,
		)

		// New: Download state version
		workspaces.GET("/:id/state/versions/:version/download",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateHandler.DownloadStateVersion,
		)

		workspaces.GET("/:id/state-versions/compare",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateController.CompareVersions,
		)

		workspaces.GET("/:id/state-versions/:version/metadata",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateController.GetStateVersionMetadata,
		)

		workspaces.GET("/:id/state-versions/:version",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			stateController.GetStateVersion,
		)

		// State operations - WRITE level (精细化权限优先)
		// New: Upload state (JSON)
		workspaces.POST("/:id/state/upload",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			stateHandler.UploadState,
		)

		// New: Upload state file (multipart/form-data)
		workspaces.POST("/:id/state/upload-file",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			stateHandler.UploadStateFile,
		)

		// New: Rollback state
		workspaces.POST("/:id/state/rollback",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			stateHandler.RollbackState,
		)

		workspaces.POST("/:id/state-versions/:version/rollback",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			stateController.RollbackState,
		)

		// State operations - ADMIN level (精细化权限优先)
		workspaces.DELETE("/:id/state-versions/:version",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			stateController.DeleteStateVersion,
		)

		// Variable operations - READ level (精细化权限优先)
		workspaces.GET("/:id/variables",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			variableController.ListVariables,
		)

		workspaces.GET("/:id/variables/:var_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			variableController.GetVariable,
		)

		// Variable operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/variables",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			variableController.CreateVariable,
		)

		// 精细权限优先：先检查workspace_variables，再检查workspace_management
		workspaces.PUT("/:id/variables/:var_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			variableController.UpdateVariable,
		)

		workspaces.DELETE("/:id/variables/:var_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			variableController.DeleteVariable,
		)

		// Variable version history operations - READ level
		workspaces.GET("/:id/variables/:var_id/versions",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			variableController.GetVariableVersions,
		)

		workspaces.GET("/:id/variables/:var_id/versions/:version",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			variableController.GetVariableVersion,
		)
		// Resource operations - READ level (精细化权限优先)
		workspaces.GET("/:id/resources",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetResources,
		)

		workspaces.GET("/:id/resources/:resource_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetResource,
		)

		workspaces.GET("/:id/resources/:resource_id/versions",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetResourceVersions,
		)

		workspaces.GET("/:id/resources/:resource_id/versions/compare",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.CompareVersions,
		)

		workspaces.GET("/:id/resources/:resource_id/versions/:version",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetResourceVersion,
		)

		workspaces.GET("/:id/resources/:resource_id/dependencies",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetResourceDependencies,
		)

		workspaces.GET("/:id/snapshots",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetSnapshots,
		)

		workspaces.GET("/:id/snapshots/:snapshot_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetSnapshot,
		)

		workspaces.GET("/:id/resources/:resource_id/editing/status",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetEditingStatus,
		)

		workspaces.GET("/:id/resources/:resource_id/drift",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			resourceController.GetDrift,
		)

		// Export resources as HCL - ADMIN level (需要workspace admin权限)
		workspaces.GET("/:id/resources/export/hcl",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			resourceController.ExportResourcesHCL,
		)

		// Resource operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/resources",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.AddResource,
		)

		workspaces.POST("/:id/resources/import",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.ImportResources,
		)

		workspaces.POST("/:id/resources/deploy",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.DeployResources,
		)

		workspaces.PUT("/:id/resources/:resource_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.UpdateResource,
		)

		workspaces.DELETE("/:id/resources/:resource_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.DeleteResource,
		)

		workspaces.PUT("/:id/resources/:resource_id/dependencies",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.UpdateDependencies,
		)

		workspaces.POST("/:id/resources/:resource_id/restore",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.RestoreResource,
		)

		workspaces.POST("/:id/resources/:resource_id/versions/:version/rollback",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.RollbackResource,
		)

		workspaces.POST("/:id/snapshots",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.CreateSnapshot,
		)

		workspaces.POST("/:id/snapshots/:snapshot_id/restore",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.RestoreSnapshot,
		)

		workspaces.DELETE("/:id/snapshots/:snapshot_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.DeleteSnapshot,
		)

		workspaces.POST("/:id/resources/:resource_id/editing/start",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.StartEditing,
		)

		workspaces.POST("/:id/resources/:resource_id/editing/heartbeat",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.Heartbeat,
		)

		workspaces.POST("/:id/resources/:resource_id/editing/end",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.EndEditing,
		)

		workspaces.POST("/:id/resources/:resource_id/drift/save",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.SaveDrift,
		)

		workspaces.POST("/:id/resources/:resource_id/drift/takeover",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.TakeoverEditing,
		)

		workspaces.DELETE("/:id/resources/:resource_id/drift",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			resourceController.DeleteDrift,
		)

		// Takeover request operations - WRITE level
		takeoverHandler := handlers.NewTakeoverHandler(db, wsHub)

		workspaces.POST("/:id/resources/:resource_id/editing/takeover-request",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			takeoverHandler.RequestTakeover,
		)

		workspaces.POST("/:id/resources/:resource_id/editing/takeover-response",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			takeoverHandler.RespondToTakeover,
		)

		workspaces.GET("/:id/resources/:resource_id/editing/pending-requests",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			takeoverHandler.GetPendingRequests,
		)

		workspaces.GET("/:id/resources/:resource_id/editing/request-status/:request_id",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			takeoverHandler.GetRequestStatus,
		)

		workspaces.POST("/:id/resources/:resource_id/editing/force-takeover",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			takeoverHandler.ForceTakeover,
		)

		// Setup workspace-agent authorization routes
		setupWorkspaceAgentRoutes(workspaces, db, iamMiddleware)

		// Setup workspace run task routes
		setupWorkspaceRunTaskRoutes(workspaces, db, iamMiddleware)

		// Setup workspace notification routes
		setupWorkspaceNotificationRoutes(workspaces, db, iamMiddleware)

		// Setup workspace output routes
		setupWorkspaceOutputRoutes(workspaces, db, iamMiddleware)

		// Setup workspace remote data routes
		setupWorkspaceRemoteDataRoutes(workspaces, db, iamMiddleware)

		// Setup workspace run trigger routes
		setupWorkspaceRunTriggerRoutes(workspaces, db, iamMiddleware)

		// Setup workspace drift detection routes
		setupWorkspaceDriftRoutes(workspaces, db, iamMiddleware)
	}
}

// setupWorkspaceProjectRoutes sets up workspace project routes
func setupWorkspaceProjectRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	wpHandler := handlers.NewWorkspaceProjectHandler(db)

	// Get workspace project - READ level (使用 Organization READ 权限)
	workspaces.GET("/:id/project",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wpHandler.GetWorkspaceProject,
	)

	// Set workspace project - WRITE level (使用 Organization WRITE 权限)
	workspaces.PUT("/:id/project",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wpHandler.SetWorkspaceProject,
	)

	// Remove workspace from project - WRITE level (使用 Organization WRITE 权限)
	workspaces.DELETE("/:id/project",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wpHandler.RemoveWorkspaceFromProject,
	)
}

// setupWorkspaceRunTaskRoutes sets up workspace run task routes
func setupWorkspaceRunTaskRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	wrtHandler := handlers.NewWorkspaceRunTaskHandler(db)

	// Override run tasks - ADMIN level
	workspaces.POST("/:id/tasks/:task_id/override-run-tasks",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		wrtHandler.OverrideRunTasks,
	)

	// Get task run task results - READ level
	workspaces.GET("/:id/tasks/:task_id/run-task-results",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wrtHandler.GetTaskRunTaskResults,
	)

	// List workspace run tasks - READ level
	workspaces.GET("/:id/run-tasks",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wrtHandler.ListWorkspaceRunTasks,
	)

	// Add run task to workspace - WRITE level
	workspaces.POST("/:id/run-tasks",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wrtHandler.AddRunTaskToWorkspace,
	)

	// Update workspace run task - WRITE level
	workspaces.PUT("/:id/run-tasks/:workspace_run_task_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wrtHandler.UpdateWorkspaceRunTask,
	)

	// Delete workspace run task - ADMIN level
	workspaces.DELETE("/:id/run-tasks/:workspace_run_task_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		wrtHandler.DeleteWorkspaceRunTask,
	)
}

// setupWorkspaceOutputRoutes sets up workspace output routes
func setupWorkspaceOutputRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	outputController := controllers.NewWorkspaceOutputController(db)

	// List outputs - READ level
	workspaces.GET("/:id/outputs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		outputController.ListOutputs,
	)

	// Get state outputs - READ level (WebUI使用，不返回sensitive数据)
	workspaces.GET("/:id/state-outputs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		outputController.GetStateOutputs,
	)

	// Note: /state-outputs/full is now handled by setupRemoteDataPublicRoutes
	// to support both JWT and temporary token authentication

	// Get resources for outputs - READ level
	workspaces.GET("/:id/outputs/resources",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		outputController.GetResourcesForOutputs,
	)

	// Get available outputs (smart hints from module schema) - READ level
	workspaces.GET("/:id/available-outputs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		outputController.GetAvailableOutputs,
	)

	// Create output - WRITE level
	workspaces.POST("/:id/outputs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		outputController.CreateOutput,
	)

	// Update output - WRITE level
	workspaces.PUT("/:id/outputs/:output_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		outputController.UpdateOutput,
	)

	// Delete output - ADMIN level
	workspaces.DELETE("/:id/outputs/:output_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		outputController.DeleteOutput,
	)

	// Batch save outputs - WRITE level
	workspaces.POST("/:id/outputs/batch",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		outputController.BatchSaveOutputs,
	)
}

// setupWorkspaceNotificationRoutes sets up workspace notification routes
func setupWorkspaceNotificationRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	wnHandler := handlers.NewWorkspaceNotificationHandler(db)

	// List workspace notifications - READ level
	workspaces.GET("/:id/notifications",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wnHandler.ListWorkspaceNotifications,
	)

	// Add notification to workspace - WRITE level
	workspaces.POST("/:id/notifications",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wnHandler.AddWorkspaceNotification,
	)

	// Update workspace notification - WRITE level
	workspaces.PUT("/:id/notifications/:workspace_notification_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		wnHandler.UpdateWorkspaceNotification,
	)

	// Delete workspace notification - ADMIN level
	workspaces.DELETE("/:id/notifications/:workspace_notification_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		wnHandler.DeleteWorkspaceNotification,
	)

	// List notification logs - READ level
	workspaces.GET("/:id/notification-logs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wnHandler.ListNotificationLogs,
	)

	// Get notification log detail - READ level
	workspaces.GET("/:id/notification-logs/:log_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wnHandler.GetNotificationLogDetail,
	)

	// Get task notification logs - READ level
	workspaces.GET("/:id/tasks/:task_id/notification-logs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		wnHandler.GetTaskNotificationLogs,
	)
}

// setupRemoteDataPublicRoutes sets up public routes for remote data token access
// These routes don't require JWT authentication, they use temporary tokens instead
func setupRemoteDataPublicRoutes(api *gin.RouterGroup, db *gorm.DB) {
	outputController := controllers.NewWorkspaceOutputController(db)
	remoteDataController := controllers.NewWorkspaceRemoteDataController(db)

	// Public route for remote data access using temporary token
	// This route is accessed by Terraform's http data source
	api.GET("/workspaces/:id/state-outputs/full", func(c *gin.Context) {
		// Check for Bearer Token (Terraform remote data access)
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" || len(authHeader) < 7 || authHeader[:7] != "Bearer " {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "Authorization required",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		tokenValue := authHeader[7:]
		// Validate temporary token
		token, err := remoteDataController.ValidateAndUseToken(tokenValue)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"code":      401,
				"message":   "Invalid token",
				"timestamp": time.Now(),
			})
			c.Abort()
			return
		}

		// Verify token is for the correct workspace
		workspaceID := c.Param("id")
		if token.WorkspaceID != workspaceID {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    403,
				"message": "Token not authorized for this workspace",
			})
			c.Abort()
			return
		}

		// Token validated, return full outputs
		outputController.GetStateOutputsFull(c)
	})
}

// setupWorkspaceRunTriggerRoutes sets up workspace run trigger routes
func setupWorkspaceRunTriggerRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	rtHandler := handlers.NewRunTriggerHandler(db)

	// List run triggers - READ level
	workspaces.GET("/:id/run-triggers",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		rtHandler.ListRunTriggers,
	)

	// List inbound triggers - READ level
	workspaces.GET("/:id/run-triggers/inbound",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		rtHandler.ListInboundTriggers,
	)

	// Get available targets - READ level
	workspaces.GET("/:id/run-triggers/available-targets",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		rtHandler.GetAvailableTargets,
	)

	// Get available sources - READ level
	workspaces.GET("/:id/run-triggers/available-sources",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		rtHandler.GetAvailableSources,
	)

	// Create inbound trigger - WRITE level
	workspaces.POST("/:id/run-triggers/inbound",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		rtHandler.CreateInboundTrigger,
	)

	// Create run trigger - WRITE level
	workspaces.POST("/:id/run-triggers",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		rtHandler.CreateRunTrigger,
	)

	// Update run trigger - WRITE level
	workspaces.PUT("/:id/run-triggers/:trigger_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		rtHandler.UpdateRunTrigger,
	)

	// Delete run trigger - ADMIN level
	workspaces.DELETE("/:id/run-triggers/:trigger_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		rtHandler.DeleteRunTrigger,
	)

	// Get task trigger executions - READ level
	workspaces.GET("/:id/tasks/:task_id/trigger-executions",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		rtHandler.GetTaskTriggerExecutions,
	)

	// Toggle trigger execution - WRITE level
	workspaces.POST("/:id/tasks/:task_id/trigger-executions/:execution_id/toggle",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		rtHandler.ToggleTriggerExecution,
	)
}

// setupWorkspaceDriftRoutes sets up workspace drift detection routes
func setupWorkspaceDriftRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	driftController := controllers.NewDriftController(db, nil) // scheduler will be set later if needed

	// Get drift config - READ level
	workspaces.GET("/:id/drift-config",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		driftController.GetDriftConfig,
	)

	// Update drift config - WRITE level
	workspaces.PUT("/:id/drift-config",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		driftController.UpdateDriftConfig,
	)

	// Get drift status - READ level
	workspaces.GET("/:id/drift-status",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		driftController.GetDriftStatus,
	)

	// Trigger drift check - WRITE level
	workspaces.POST("/:id/drift-check",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		driftController.TriggerDriftCheck,
	)

	// Cancel drift check - WRITE level
	workspaces.DELETE("/:id/drift-check",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		driftController.CancelDriftCheck,
	)

	// Get resource drift statuses - READ level
	workspaces.GET("/:id/resources-drift",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		driftController.GetResourceDriftStatuses,
	)
}

// setupWorkspaceRemoteDataRoutes sets up workspace remote data routes
func setupWorkspaceRemoteDataRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	remoteDataController := controllers.NewWorkspaceRemoteDataController(db)

	// List remote data - READ level
	workspaces.GET("/:id/remote-data",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		remoteDataController.ListRemoteData,
	)

	// Get accessible workspaces - READ level
	workspaces.GET("/:id/remote-data/accessible-workspaces",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		remoteDataController.GetAccessibleWorkspaces,
	)

	// Get source workspace outputs - READ level
	workspaces.GET("/:id/remote-data/source-outputs",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		remoteDataController.GetSourceWorkspaceOutputs,
	)

	// Create remote data - WRITE level
	workspaces.POST("/:id/remote-data",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		remoteDataController.CreateRemoteData,
	)

	// Update remote data - WRITE level
	workspaces.PUT("/:id/remote-data/:remote_data_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		remoteDataController.UpdateRemoteData,
	)

	// Delete remote data - ADMIN level
	workspaces.DELETE("/:id/remote-data/:remote_data_id",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		}),
		remoteDataController.DeleteRemoteData,
	)

	// Get outputs sharing settings - READ level
	workspaces.GET("/:id/outputs-sharing",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		remoteDataController.GetOutputsSharing,
	)

	// Update outputs sharing settings - WRITE level
	workspaces.PUT("/:id/outputs-sharing",
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		}),
		remoteDataController.UpdateOutputsSharing,
	)
}
