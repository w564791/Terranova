package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupTaskRoutes sets up task log routes
func setupTaskRoutes(api *gin.RouterGroup, db *gorm.DB, streamManager *services.OutputStreamManager, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现task路由
	// 参考原router.go中的任务日志管理部分:
	// - api.GET("/tasks/:task_id/output/stream")
	// - api.GET("/tasks/:task_id/logs")
	// - api.GET("/tasks/:task_id/logs/download")
	// - api.GET("/terraform/streams/stats")
	// 任务日志管理（全局，不在workspaces组内）- 添加IAM权限检查
	taskLogController := controllers.NewTaskLogController(db)
	outputController := controllers.NewTerraformOutputController(streamManager)

	api.GET("/tasks/:task_id/output/stream", middleware.JWTAuth(), middleware.AuditLogger(db), func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.StreamTaskOutput(c)
			return
		}
		iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ")(c)
		if !c.IsAborted() {
			outputController.StreamTaskOutput(c)
		}
	})

	api.GET("/tasks/:task_id/logs", middleware.JWTAuth(), middleware.AuditLogger(db), func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			taskLogController.GetTaskLogs(c)
			return
		}
		iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ")(c)
		if !c.IsAborted() {
			taskLogController.GetTaskLogs(c)
		}
	})

	api.GET("/tasks/:task_id/logs/download", middleware.JWTAuth(), middleware.AuditLogger(db), func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			taskLogController.DownloadTaskLogs(c)
			return
		}
		iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ")(c)
		if !c.IsAborted() {
			taskLogController.DownloadTaskLogs(c)
		}
	})

	api.GET("/terraform/streams/stats", middleware.JWTAuth(), middleware.AuditLogger(db), func(c *gin.Context) {
		role, _ := c.Get("role")
		if role == "admin" {
			outputController.GetStreamStats(c)
			return
		}
		iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ")(c)
		if !c.IsAborted() {
			outputController.GetStreamStats(c)
		}
	})
}
