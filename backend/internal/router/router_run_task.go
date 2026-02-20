package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupRunTaskCallbackRoutes sets up Run Task callback routes (公开路由，不需要认证)
func setupRunTaskCallbackRoutes(api *gin.RouterGroup, db *gorm.DB, runTaskExecutor *services.RunTaskExecutor) {
	// Initialize callback handler
	callbackHandler := handlers.NewRunTaskCallbackHandler(db, runTaskExecutor)

	// Run Task Results routes (公开路由，供外部 Run Task 服务回调)
	runTaskResults := api.Group("/run-task-results")
	{
		// Callback endpoint - 外部 Run Task 服务调用此接口报告结果
		// 注意：同时支持 PATCH 和 POST 方法，因为某些环境下 PATCH 方法可能有问题
		runTaskResults.PATCH("/:result_id/callback", callbackHandler.HandleCallback)
		runTaskResults.POST("/:result_id/callback", callbackHandler.HandleCallback)

		// Get single result (可选：也可以设为公开，方便调试)
		runTaskResults.GET("/:result_id", callbackHandler.GetRunTaskResult)
	}
}

// setupRunTaskRoutes sets up Run Task management routes (需要 JWT 认证)
func setupRunTaskRoutes(adminProtected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// Initialize handlers
	runTaskHandler := handlers.NewRunTaskHandler(db)

	// ===== Run Task Management Routes (with IAM permissions) =====
	runTasks := adminProtected.Group("/run-tasks")
	{
		// Create run task
		runTasks.POST("",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE"),
			runTaskHandler.CreateRunTask,
		)

		// List run tasks
		runTasks.GET("",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ"),
			runTaskHandler.ListRunTasks,
		)

		// Get run task details
		runTasks.GET("/:run_task_id",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ"),
			runTaskHandler.GetRunTask,
		)

		// Update run task
		runTasks.PUT("/:run_task_id",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE"),
			runTaskHandler.UpdateRunTask,
		)

		// Delete run task
		runTasks.DELETE("/:run_task_id",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "ADMIN"),
			runTaskHandler.DeleteRunTask,
		)

		// Test run task connection (new configuration)
		runTasks.POST("/test",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE"),
			runTaskHandler.TestRunTask,
		)

		// Test existing run task connection
		runTasks.POST("/:run_task_id/test",
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ"),
			runTaskHandler.TestExistingRunTask,
		)
	}
}
