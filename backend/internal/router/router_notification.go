package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupNotificationRoutes 设置通知相关路由
func SetupNotificationRoutes(r *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	notificationHandler := handlers.NewNotificationHandler(db)

	// 全局通知配置管理 API - 使用SYSTEM_SETTINGS权限
	notifications := r.Group("/notifications")
	{
		// 获取通知配置列表
		notifications.GET("",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			notificationHandler.ListNotifications,
		)
		// 获取可用的通知配置（用于 Workspace 添加通知时选择）
		notifications.GET("/available",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			notificationHandler.GetAvailableNotifications,
		)
		// 获取单个通知配置
		notifications.GET("/:notification_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			notificationHandler.GetNotification,
		)
		// 创建通知配置
		notifications.POST("",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			notificationHandler.CreateNotification,
		)
		// 更新通知配置
		notifications.PUT("/:notification_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			notificationHandler.UpdateNotification,
		)
		// 删除通知配置
		notifications.DELETE("/:notification_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			notificationHandler.DeleteNotification,
		)
		// 测试通知配置
		notifications.POST("/:notification_id/test",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			notificationHandler.TestNotification,
		)
	}
}
