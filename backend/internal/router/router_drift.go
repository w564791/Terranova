package router

import (
	"iac-platform/controllers"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterDriftRoutes 注册 Drift 检测相关路由
func RegisterDriftRoutes(r *gin.RouterGroup, db *gorm.DB, scheduler *services.DriftCheckScheduler) {
	driftController := controllers.NewDriftController(db, scheduler)

	// Drift 配置 - 使用 :id 与其他 workspace 路由保持一致
	r.GET("/workspaces/:id/drift-config", driftController.GetDriftConfig)
	r.PUT("/workspaces/:id/drift-config", driftController.UpdateDriftConfig)

	// Drift 状态
	r.GET("/workspaces/:id/drift-status", driftController.GetDriftStatus)

	// 手动触发检测
	r.POST("/workspaces/:id/drift-check", driftController.TriggerDriftCheck)

	// 取消检测
	r.DELETE("/workspaces/:id/drift-check", driftController.CancelDriftCheck)

	// 资源 drift 状态
	r.GET("/workspaces/:id/resources/drift", driftController.GetResourceDriftStatuses)
}
