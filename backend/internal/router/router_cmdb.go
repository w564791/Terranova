package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupCMDBRoutes 设置CMDB路由
func SetupCMDBRoutes(r *gin.RouterGroup, db *gorm.DB) {
	// 创建服务和处理器
	cmdbService := services.NewCMDBService(db)
	cmdbHandler := handlers.NewCMDBHandler(cmdbService)

	// 创建外部数据源处理器
	externalSourceHandler := handlers.NewCMDBExternalSourceHandler(db)

	// 初始化IAM权限中间件
	iamMiddleware := middleware.NewIAMPermissionMiddleware(db)

	// CMDB路由组
	cmdb := r.Group("/cmdb")
	{
		// 搜索资源（只读，所有认证用户可访问）
		cmdb.GET("/search", cmdbHandler.SearchResources)

		// 搜索建议（只读，所有认证用户可访问）
		cmdb.GET("/suggestions", cmdbHandler.GetSearchSuggestions)

		// 获取统计信息（只读，所有认证用户可访问）
		cmdb.GET("/stats", cmdbHandler.GetCMDBStats)

		// 获取资源类型列表（只读，所有认证用户可访问）
		cmdb.GET("/resource-types", cmdbHandler.GetResourceTypes)

		// 获取所有workspace的资源数量（只读，所有认证用户可访问）
		cmdb.GET("/workspace-counts", cmdbHandler.GetWorkspaceResourceCounts)

		// Workspace相关（只读，所有认证用户可访问）
		cmdb.GET("/workspaces/:workspace_id/tree", cmdbHandler.GetWorkspaceResourceTree)
		cmdb.GET("/workspaces/:workspace_id/resources", cmdbHandler.GetResourceDetail)

		// 同步操作（需要cmdb:ADMIN权限，通常只有admin有此权限）
		cmdb.POST("/workspaces/:workspace_id/sync",
			iamMiddleware.RequirePermission("cmdb", "ORGANIZATION", "ADMIN"),
			cmdbHandler.SyncWorkspace)
		cmdb.POST("/sync-all",
			iamMiddleware.RequirePermission("cmdb", "ORGANIZATION", "ADMIN"),
			cmdbHandler.SyncAllWorkspaces)

		// ===== 外部数据源管理（需要cmdb:ADMIN权限） =====
		externalSources := cmdb.Group("/external-sources")
		externalSources.Use(iamMiddleware.RequirePermission("cmdb", "ORGANIZATION", "ADMIN"))
		{
			// 列出所有外部数据源
			externalSources.GET("", externalSourceHandler.ListExternalSources)
			// 创建外部数据源
			externalSources.POST("", externalSourceHandler.CreateExternalSource)
			// 获取外部数据源详情
			externalSources.GET("/:source_id", externalSourceHandler.GetExternalSource)
			// 更新外部数据源
			externalSources.PUT("/:source_id", externalSourceHandler.UpdateExternalSource)
			// 删除外部数据源
			externalSources.DELETE("/:source_id", externalSourceHandler.DeleteExternalSource)
			// 手动触发同步
			externalSources.POST("/:source_id/sync", externalSourceHandler.SyncExternalSource)
			// 测试连接
			externalSources.POST("/:source_id/test", externalSourceHandler.TestConnection)
			// 获取同步日志
			externalSources.GET("/:source_id/sync-logs", externalSourceHandler.GetSyncLogs)
		}
	}
}
