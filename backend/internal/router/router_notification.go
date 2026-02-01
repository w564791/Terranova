package router

import (
	"iac-platform/internal/handlers"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupNotificationRoutes 设置通知相关路由
func SetupNotificationRoutes(r *gin.RouterGroup, db *gorm.DB) {
	notificationHandler := handlers.NewNotificationHandler(db)

	// 全局通知配置管理 API
	notifications := r.Group("/notifications")
	{
		// 获取通知配置列表
		notifications.GET("", notificationHandler.ListNotifications)
		// 获取可用的通知配置（用于 Workspace 添加通知时选择）
		notifications.GET("/available", notificationHandler.GetAvailableNotifications)
		// 获取单个通知配置
		notifications.GET("/:notification_id", notificationHandler.GetNotification)
		// 创建通知配置
		notifications.POST("", notificationHandler.CreateNotification)
		// 更新通知配置
		notifications.PUT("/:notification_id", notificationHandler.UpdateNotification)
		// 删除通知配置
		notifications.DELETE("/:notification_id", notificationHandler.DeleteNotification)
		// 测试通知配置
		notifications.POST("/:notification_id/test", notificationHandler.TestNotification)
	}
}
