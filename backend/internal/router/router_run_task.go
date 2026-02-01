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
		runTasks.POST("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.CreateRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				runTaskHandler.CreateRunTask(c)
			}
		})

		// List run tasks
		runTasks.GET("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.ListRunTasks(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				runTaskHandler.ListRunTasks(c)
			}
		})

		// Get run task details
		runTasks.GET("/:run_task_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.GetRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				runTaskHandler.GetRunTask(c)
			}
		})

		// Update run task
		runTasks.PUT("/:run_task_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.UpdateRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				runTaskHandler.UpdateRunTask(c)
			}
		})

		// Delete run task
		runTasks.DELETE("/:run_task_id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.DeleteRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				runTaskHandler.DeleteRunTask(c)
			}
		})

		// Test run task connection (new configuration)
		runTasks.POST("/test", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.TestRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				runTaskHandler.TestRunTask(c)
			}
		})

		// Test existing run task connection
		runTasks.POST("/:run_task_id/test", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				runTaskHandler.TestExistingRunTask(c)
				return
			}
			iamMiddleware.RequirePermission("RUN_TASKS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				runTaskHandler.TestExistingRunTask(c)
			}
		})
	}
}
