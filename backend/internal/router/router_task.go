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
	// 全局任务日志：JWT + 加载 task 后按 workspace READ 鉴权（防水平越权）
	// 推荐客户端改用 /workspaces/{id}/tasks/{task_id}/logs
	taskLogController := controllers.NewTaskLogController(db, iamMiddleware)
	outputController := controllers.NewTerraformOutputController(streamManager, db, iamMiddleware)

	api.GET("/tasks/:task_id/output/stream", middleware.JWTAuth(), middleware.AuditLogger(db),
		outputController.StreamTaskOutput,
	)

	api.GET("/tasks/:task_id/logs", middleware.JWTAuth(), middleware.AuditLogger(db),
		taskLogController.GetTaskLogs,
	)

	api.GET("/tasks/:task_id/logs/download", middleware.JWTAuth(), middleware.AuditLogger(db),
		taskLogController.DownloadTaskLogs,
	)

	// 流统计为运维面，仍要求显式 org 级 TASK_LOGS（需 org_id 或单租户 flag）
	api.GET("/terraform/streams/stats", middleware.JWTAuth(), middleware.AuditLogger(db),
		iamMiddleware.RequirePermission("TASK_LOGS", "ORGANIZATION", "READ"),
		outputController.GetStreamStats,
	)
}
