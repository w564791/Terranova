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
		workspaces.GET("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.GetWorkspaces(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				workspaceController.GetWorkspaces(c)
			}
		})

		// Workspace basic operations - READ level
		workspaces.GET("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.GetWorkspace(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				workspaceController.GetWorkspace(c)
			}
		})

		workspaces.GET("/:id/overview", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.GetWorkspaceOverview(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				workspaceController.GetWorkspaceOverview(c)
			}
		})

		// Workspace basic operations - WRITE level
		workspaces.PUT("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.UpdateWorkspace(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE")(c)
			if !c.IsAborted() {
				workspaceController.UpdateWorkspace(c)
			}
		})

		workspaces.PATCH("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.UpdateWorkspace(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE")(c)
			if !c.IsAborted() {
				workspaceController.UpdateWorkspace(c)
			}
		})

		workspaces.POST("/:id/lock", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
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
				return
			}
			iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE")(c)
			if !c.IsAborted() {
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
			}
		})

		workspaces.POST("/:id/unlock", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceID := c.Param("id")
				if err := lifecycleService.UnlockWorkspace(workspaceID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Workspace unlocked successfully"})
				return
			}
			iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "WRITE")(c)
			if !c.IsAborted() {
				workspaceID := c.Param("id")
				if err := lifecycleService.UnlockWorkspace(workspaceID); err != nil {
					c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
					return
				}
				c.JSON(http.StatusOK, gin.H{"message": "Workspace unlocked successfully"})
			}
		})

		// Workspace basic operations - ADMIN level
		workspaces.DELETE("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.DeleteWorkspace(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "WORKSPACE", "ADMIN")(c)
			if !c.IsAborted() {
				workspaceController.DeleteWorkspace(c)
			}
		})

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

		workspaces.POST("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				workspaceController.CreateWorkspace(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				workspaceController.CreateWorkspace(c)
			}
		})
		// Task operations - READ level (精细化权限优先)
		workspaces.GET("/:id/tasks", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.GetTasks(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				taskController.GetTasks(c)
			}
		})

		workspaces.GET("/:id/tasks/:task_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.GetTask(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				taskController.GetTask(c)
			}
		})

		workspaces.GET("/:id/tasks/:task_id/logs", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.GetTaskLogs(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				taskController.GetTaskLogs(c)
			}
		})

		workspaces.GET("/:id/tasks/:task_id/comments", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.GetComments(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				taskController.GetComments(c)
			}
		})

		workspaces.GET("/:id/tasks/:task_id/resource-changes", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				controllers.GetTaskResourceChangesWithDB(c, db)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				controllers.GetTaskResourceChangesWithDB(c, db)
			}
		})

		workspaces.GET("/:id/tasks/:task_id/error-analysis", func(c *gin.Context) {
			aiController := controllers.NewAIController(db)
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.GetTaskAnalysis(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				aiController.GetTaskAnalysis(c)
			}
		})

		workspaces.GET("/:id/tasks/:task_id/state-backup", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.DownloadStateBackup(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				taskController.DownloadStateBackup(c)
			}
		})

		// Task operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/tasks/plan", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.CreatePlanTask(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				taskController.CreatePlanTask(c)
			}
		})

		workspaces.POST("/:id/tasks/:task_id/comments", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.CreateComment(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				taskController.CreateComment(c)
			}
		})

		// Task operations - ADMIN level (精细化权限优先)
		workspaces.POST("/:id/tasks/:task_id/cancel", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.CancelTask(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				taskController.CancelTask(c)
			}
		})

		workspaces.POST("/:id/tasks/:task_id/cancel-previous", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.CancelPreviousTasks(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				taskController.CancelPreviousTasks(c)
			}
		})

		workspaces.POST("/:id/tasks/:task_id/confirm-apply", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.ConfirmApply(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				taskController.ConfirmApply(c)
			}
		})

		workspaces.PATCH("/:id/tasks/:task_id/resource-changes/:resource_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				controllers.UpdateResourceApplyStatusWithDB(c, db)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				controllers.UpdateResourceApplyStatusWithDB(c, db)
			}
		})

		workspaces.POST("/:id/tasks/:task_id/retry-state-save", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				taskController.RetryStateSave(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				taskController.RetryStateSave(c)
			}
		})

		workspaces.POST("/:id/tasks/:task_id/parse-plan", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				controllers.ManualParsePlanWithDB(c, db)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				controllers.ManualParsePlanWithDB(c, db)
			}
		})
		// State operations - READ level (精细化权限优先)
		stateHandler := handlers.NewStateHandler(services.NewStateService(db))

		workspaces.GET("/:id/current-state", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.GetCurrentState(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateController.GetCurrentState(c)
			}
		})

		workspaces.GET("/:id/state-versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.GetStateVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateController.GetStateVersions(c)
			}
		})

		// New: Get state version history (with pagination)
		workspaces.GET("/:id/state/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.GetStateVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateHandler.GetStateVersions(c)
			}
		})

		// New: Get specific state version (metadata only, no content)
		workspaces.GET("/:id/state/versions/:version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.GetStateVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateHandler.GetStateVersion(c)
			}
		})

		// New: Retrieve state version content (requires WORKSPACE_STATE_SENSITIVE permission)
		// This endpoint returns the full state content including sensitive data
		workspaces.GET("/:id/state/versions/:version/retrieve", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.RetrieveStateVersion(c)
				return
			}
			// Requires WORKSPACE_STATE_SENSITIVE permission to access state content
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE_SENSITIVE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				stateHandler.RetrieveStateVersion(c)
			}
		})

		// New: Download state version
		workspaces.GET("/:id/state/versions/:version/download", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.DownloadStateVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateHandler.DownloadStateVersion(c)
			}
		})

		workspaces.GET("/:id/state-versions/compare", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.CompareVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateController.CompareVersions(c)
			}
		})

		workspaces.GET("/:id/state-versions/:version/metadata", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.GetStateVersionMetadata(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateController.GetStateVersionMetadata(c)
			}
		})

		workspaces.GET("/:id/state-versions/:version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.GetStateVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				stateController.GetStateVersion(c)
			}
		})

		// State operations - WRITE level (精细化权限优先)
		// New: Upload state (JSON)
		workspaces.POST("/:id/state/upload", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.UploadState(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				stateHandler.UploadState(c)
			}
		})

		// New: Upload state file (multipart/form-data)
		workspaces.POST("/:id/state/upload-file", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.UploadStateFile(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				stateHandler.UploadStateFile(c)
			}
		})

		// New: Rollback state
		workspaces.POST("/:id/state/rollback", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateHandler.RollbackState(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				stateHandler.RollbackState(c)
			}
		})

		workspaces.POST("/:id/state-versions/:version/rollback", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.RollbackState(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				stateController.RollbackState(c)
			}
		})

		// State operations - ADMIN level (精细化权限优先)
		workspaces.DELETE("/:id/state-versions/:version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				stateController.DeleteStateVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				stateController.DeleteStateVersion(c)
			}
		})

		// Variable operations - READ level (精细化权限优先)
		workspaces.GET("/:id/variables", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.ListVariables(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				variableController.ListVariables(c)
			}
		})

		workspaces.GET("/:id/variables/:var_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.GetVariable(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				variableController.GetVariable(c)
			}
		})

		// Variable operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/variables", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.CreateVariable(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				variableController.CreateVariable(c)
			}
		})

		workspaces.PUT("/:id/variables/:var_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.UpdateVariable(c)
				return
			}
			// 精细权限优先：先检查workspace_variables，再检查workspace_management
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				variableController.UpdateVariable(c)
			}
		})

		workspaces.DELETE("/:id/variables/:var_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.DeleteVariable(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				variableController.DeleteVariable(c)
			}
		})

		// Variable version history operations - READ level
		workspaces.GET("/:id/variables/:var_id/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.GetVariableVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				variableController.GetVariableVersions(c)
			}
		})

		workspaces.GET("/:id/variables/:var_id/versions/:version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				variableController.GetVariableVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_VARIABLES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				variableController.GetVariableVersion(c)
			}
		})
		// Resource operations - READ level (精细化权限优先)
		workspaces.GET("/:id/resources", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetResources(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetResources(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetResource(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetResourceVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetResourceVersions(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/versions/compare", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.CompareVersions(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.CompareVersions(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/versions/:version", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetResourceVersion(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetResourceVersion(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/dependencies", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetResourceDependencies(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetResourceDependencies(c)
			}
		})

		workspaces.GET("/:id/snapshots", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetSnapshots(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetSnapshots(c)
			}
		})

		workspaces.GET("/:id/snapshots/:snapshot_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetSnapshot(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetSnapshot(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/editing/status", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetEditingStatus(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetEditingStatus(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/drift", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.GetDrift(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				resourceController.GetDrift(c)
			}
		})

		// Export resources as HCL - ADMIN level (需要workspace admin权限)
		workspaces.GET("/:id/resources/export/hcl", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.ExportResourcesHCL(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			})(c)
			if !c.IsAborted() {
				resourceController.ExportResourcesHCL(c)
			}
		})

		// Resource operations - WRITE level (精细化权限优先)
		workspaces.POST("/:id/resources", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.AddResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.AddResource(c)
			}
		})

		workspaces.POST("/:id/resources/import", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.ImportResources(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.ImportResources(c)
			}
		})

		workspaces.POST("/:id/resources/deploy", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.DeployResources(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.DeployResources(c)
			}
		})

		workspaces.PUT("/:id/resources/:resource_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.UpdateResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.UpdateResource(c)
			}
		})

		workspaces.DELETE("/:id/resources/:resource_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.DeleteResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.DeleteResource(c)
			}
		})

		workspaces.PUT("/:id/resources/:resource_id/dependencies", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.UpdateDependencies(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.UpdateDependencies(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/restore", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.RestoreResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.RestoreResource(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/versions/:version/rollback", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.RollbackResource(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.RollbackResource(c)
			}
		})

		workspaces.POST("/:id/snapshots", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.CreateSnapshot(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.CreateSnapshot(c)
			}
		})

		workspaces.POST("/:id/snapshots/:snapshot_id/restore", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.RestoreSnapshot(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.RestoreSnapshot(c)
			}
		})

		workspaces.DELETE("/:id/snapshots/:snapshot_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.DeleteSnapshot(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.DeleteSnapshot(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/editing/start", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.StartEditing(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.StartEditing(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/editing/heartbeat", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.Heartbeat(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.Heartbeat(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/editing/end", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.EndEditing(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.EndEditing(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/drift/save", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.SaveDrift(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.SaveDrift(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/drift/takeover", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.TakeoverEditing(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.TakeoverEditing(c)
			}
		})

		workspaces.DELETE("/:id/resources/:resource_id/drift", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				resourceController.DeleteDrift(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				resourceController.DeleteDrift(c)
			}
		})

		// Takeover request operations - WRITE level
		takeoverHandler := handlers.NewTakeoverHandler(db, wsHub)

		workspaces.POST("/:id/resources/:resource_id/editing/takeover-request", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				takeoverHandler.RequestTakeover(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				takeoverHandler.RequestTakeover(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/editing/takeover-response", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				takeoverHandler.RespondToTakeover(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				takeoverHandler.RespondToTakeover(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/editing/pending-requests", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				takeoverHandler.GetPendingRequests(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				takeoverHandler.GetPendingRequests(c)
			}
		})

		workspaces.GET("/:id/resources/:resource_id/editing/request-status/:request_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				takeoverHandler.GetRequestStatus(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			})(c)
			if !c.IsAborted() {
				takeoverHandler.GetRequestStatus(c)
			}
		})

		workspaces.POST("/:id/resources/:resource_id/editing/force-takeover", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				takeoverHandler.ForceTakeover(c)
				return
			}
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			})(c)
			if !c.IsAborted() {
				takeoverHandler.ForceTakeover(c)
			}
		})

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
	workspaces.GET("/:id/project", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wpHandler.GetWorkspaceProject(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wpHandler.GetWorkspaceProject(c)
		}
	})

	// Set workspace project - WRITE level (使用 Organization WRITE 权限)
	workspaces.PUT("/:id/project", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wpHandler.SetWorkspaceProject(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wpHandler.SetWorkspaceProject(c)
		}
	})

	// Remove workspace from project - WRITE level (使用 Organization WRITE 权限)
	workspaces.DELETE("/:id/project", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wpHandler.RemoveWorkspaceFromProject(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wpHandler.RemoveWorkspaceFromProject(c)
		}
	})
}

// setupWorkspaceRunTaskRoutes sets up workspace run task routes
func setupWorkspaceRunTaskRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	wrtHandler := handlers.NewWorkspaceRunTaskHandler(db)

	// Override run tasks - ADMIN level
	workspaces.POST("/:id/tasks/:task_id/override-run-tasks", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.OverrideRunTasks(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.OverrideRunTasks(c)
		}
	})

	// Get task run task results - READ level
	workspaces.GET("/:id/tasks/:task_id/run-task-results", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.GetTaskRunTaskResults(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.GetTaskRunTaskResults(c)
		}
	})

	// List workspace run tasks - READ level
	workspaces.GET("/:id/run-tasks", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.ListWorkspaceRunTasks(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.ListWorkspaceRunTasks(c)
		}
	})

	// Add run task to workspace - WRITE level
	workspaces.POST("/:id/run-tasks", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.AddRunTaskToWorkspace(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.AddRunTaskToWorkspace(c)
		}
	})

	// Update workspace run task - WRITE level
	workspaces.PUT("/:id/run-tasks/:workspace_run_task_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.UpdateWorkspaceRunTask(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.UpdateWorkspaceRunTask(c)
		}
	})

	// Delete workspace run task - ADMIN level
	workspaces.DELETE("/:id/run-tasks/:workspace_run_task_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wrtHandler.DeleteWorkspaceRunTask(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			wrtHandler.DeleteWorkspaceRunTask(c)
		}
	})
}

// setupWorkspaceOutputRoutes sets up workspace output routes
func setupWorkspaceOutputRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	outputController := controllers.NewWorkspaceOutputController(db)

	// List outputs - READ level
	workspaces.GET("/:id/outputs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.ListOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			outputController.ListOutputs(c)
		}
	})

	// Get state outputs - READ level (WebUI使用，不返回sensitive数据)
	workspaces.GET("/:id/state-outputs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.GetStateOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_STATE", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			outputController.GetStateOutputs(c)
		}
	})

	// Note: /state-outputs/full is now handled by setupRemoteDataPublicRoutes
	// to support both JWT and temporary token authentication

	// Get resources for outputs - READ level
	workspaces.GET("/:id/outputs/resources", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.GetResourcesForOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			outputController.GetResourcesForOutputs(c)
		}
	})

	// Get available outputs (smart hints from module schema) - READ level
	workspaces.GET("/:id/available-outputs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.GetAvailableOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			outputController.GetAvailableOutputs(c)
		}
	})

	// Create output - WRITE level
	workspaces.POST("/:id/outputs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.CreateOutput(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			outputController.CreateOutput(c)
		}
	})

	// Update output - WRITE level
	workspaces.PUT("/:id/outputs/:output_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.UpdateOutput(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			outputController.UpdateOutput(c)
		}
	})

	// Delete output - ADMIN level
	workspaces.DELETE("/:id/outputs/:output_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.DeleteOutput(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			outputController.DeleteOutput(c)
		}
	})

	// Batch save outputs - WRITE level
	workspaces.POST("/:id/outputs/batch", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.BatchSaveOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			outputController.BatchSaveOutputs(c)
		}
	})
}

// setupWorkspaceNotificationRoutes sets up workspace notification routes
func setupWorkspaceNotificationRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	wnHandler := handlers.NewWorkspaceNotificationHandler(db)

	// List workspace notifications - READ level
	workspaces.GET("/:id/notifications", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.ListWorkspaceNotifications(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wnHandler.ListWorkspaceNotifications(c)
		}
	})

	// Add notification to workspace - WRITE level
	workspaces.POST("/:id/notifications", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.AddWorkspaceNotification(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wnHandler.AddWorkspaceNotification(c)
		}
	})

	// Update workspace notification - WRITE level
	workspaces.PUT("/:id/notifications/:workspace_notification_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.UpdateWorkspaceNotification(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			wnHandler.UpdateWorkspaceNotification(c)
		}
	})

	// Delete workspace notification - ADMIN level
	workspaces.DELETE("/:id/notifications/:workspace_notification_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.DeleteWorkspaceNotification(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			wnHandler.DeleteWorkspaceNotification(c)
		}
	})

	// List notification logs - READ level
	workspaces.GET("/:id/notification-logs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.ListNotificationLogs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wnHandler.ListNotificationLogs(c)
		}
	})

	// Get notification log detail - READ level
	workspaces.GET("/:id/notification-logs/:log_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.GetNotificationLogDetail(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wnHandler.GetNotificationLogDetail(c)
		}
	})

	// Get task notification logs - READ level
	workspaces.GET("/:id/tasks/:task_id/notification-logs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			wnHandler.GetTaskNotificationLogs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			wnHandler.GetTaskNotificationLogs(c)
		}
	})
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
	workspaces.GET("/:id/run-triggers", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.ListRunTriggers(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			rtHandler.ListRunTriggers(c)
		}
	})

	// List inbound triggers - READ level
	workspaces.GET("/:id/run-triggers/inbound", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.ListInboundTriggers(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			rtHandler.ListInboundTriggers(c)
		}
	})

	// Get available targets - READ level
	workspaces.GET("/:id/run-triggers/available-targets", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.GetAvailableTargets(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			rtHandler.GetAvailableTargets(c)
		}
	})

	// Get available sources - READ level
	workspaces.GET("/:id/run-triggers/available-sources", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.GetAvailableSources(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			rtHandler.GetAvailableSources(c)
		}
	})

	// Create inbound trigger - WRITE level
	workspaces.POST("/:id/run-triggers/inbound", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.CreateInboundTrigger(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			rtHandler.CreateInboundTrigger(c)
		}
	})

	// Create run trigger - WRITE level
	workspaces.POST("/:id/run-triggers", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.CreateRunTrigger(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			rtHandler.CreateRunTrigger(c)
		}
	})

	// Update run trigger - WRITE level
	workspaces.PUT("/:id/run-triggers/:trigger_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.UpdateRunTrigger(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			rtHandler.UpdateRunTrigger(c)
		}
	})

	// Delete run trigger - ADMIN level
	workspaces.DELETE("/:id/run-triggers/:trigger_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.DeleteRunTrigger(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			rtHandler.DeleteRunTrigger(c)
		}
	})

	// Get task trigger executions - READ level
	workspaces.GET("/:id/tasks/:task_id/trigger-executions", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.GetTaskTriggerExecutions(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "TASK_DATA_ACCESS", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			rtHandler.GetTaskTriggerExecutions(c)
		}
	})

	// Toggle trigger execution - WRITE level
	workspaces.POST("/:id/tasks/:task_id/trigger-executions/:execution_id/toggle", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			rtHandler.ToggleTriggerExecution(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			rtHandler.ToggleTriggerExecution(c)
		}
	})
}

// setupWorkspaceDriftRoutes sets up workspace drift detection routes
func setupWorkspaceDriftRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	driftController := controllers.NewDriftController(db, nil) // scheduler will be set later if needed

	// Get drift config - READ level
	workspaces.GET("/:id/drift-config", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.GetDriftConfig(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			driftController.GetDriftConfig(c)
		}
	})

	// Update drift config - WRITE level
	workspaces.PUT("/:id/drift-config", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.UpdateDriftConfig(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			driftController.UpdateDriftConfig(c)
		}
	})

	// Get drift status - READ level
	workspaces.GET("/:id/drift-status", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.GetDriftStatus(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			driftController.GetDriftStatus(c)
		}
	})

	// Trigger drift check - WRITE level
	workspaces.POST("/:id/drift-check", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.TriggerDriftCheck(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			driftController.TriggerDriftCheck(c)
		}
	})

	// Cancel drift check - WRITE level
	workspaces.DELETE("/:id/drift-check", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.CancelDriftCheck(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_EXECUTION", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			driftController.CancelDriftCheck(c)
		}
	})

	// Get resource drift statuses - READ level
	workspaces.GET("/:id/resources-drift", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			driftController.GetResourceDriftStatuses(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_RESOURCES", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			driftController.GetResourceDriftStatuses(c)
		}
	})
}

// setupWorkspaceRemoteDataRoutes sets up workspace remote data routes
func setupWorkspaceRemoteDataRoutes(workspaces *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	remoteDataController := controllers.NewWorkspaceRemoteDataController(db)

	// List remote data - READ level
	workspaces.GET("/:id/remote-data", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.ListRemoteData(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.ListRemoteData(c)
		}
	})

	// Get accessible workspaces - READ level
	workspaces.GET("/:id/remote-data/accessible-workspaces", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.GetAccessibleWorkspaces(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.GetAccessibleWorkspaces(c)
		}
	})

	// Get source workspace outputs - READ level
	workspaces.GET("/:id/remote-data/source-outputs", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.GetSourceWorkspaceOutputs(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.GetSourceWorkspaceOutputs(c)
		}
	})

	// Create remote data - WRITE level
	workspaces.POST("/:id/remote-data", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.CreateRemoteData(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.CreateRemoteData(c)
		}
	})

	// Update remote data - WRITE level
	workspaces.PUT("/:id/remote-data/:remote_data_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.UpdateRemoteData(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.UpdateRemoteData(c)
		}
	})

	// Delete remote data - ADMIN level
	workspaces.DELETE("/:id/remote-data/:remote_data_id", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.DeleteRemoteData(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.DeleteRemoteData(c)
		}
	})

	// Get outputs sharing settings - READ level
	workspaces.GET("/:id/outputs-sharing", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.GetOutputsSharing(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.GetOutputsSharing(c)
		}
	})

	// Update outputs sharing settings - WRITE level
	workspaces.PUT("/:id/outputs-sharing", func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			remoteDataController.UpdateOutputsSharing(c)
			return
		}
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
		})(c)
		if !c.IsAborted() {
			remoteDataController.UpdateOutputsSharing(c)
		}
	})
}
