package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TaskQueueManagerInterface 任务队列管理器接口
type TaskQueueManagerInterface interface {
	TryExecuteNextTask(workspaceID string) error
}

// RegisterManifestRoutes 注册 Manifest 相关路由
func RegisterManifestRoutes(r *gin.RouterGroup, db *gorm.DB, queueManager TaskQueueManagerInterface, iamMiddleware *middleware.IAMPermissionMiddleware) {
	manifestHandler := handlers.NewManifestHandler(db)
	if queueManager != nil {
		manifestHandler.SetQueueManager(queueManager)
	}

	// Organization 级别的 Manifest 路由 - 使用SYSTEM_SETTINGS权限
	orgManifests := r.Group("/organizations/:org_id/manifests")
	orgManifests.Use(middleware.JWTAuth())
	{
		// Manifest CRUD
		orgManifests.GET("",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ListManifests,
		)
		orgManifests.POST("",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.CreateManifest,
		)
		orgManifests.GET("/:id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.GetManifest,
		)
		orgManifests.PUT("/:id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.UpdateManifest,
		)
		orgManifests.DELETE("/:id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			manifestHandler.DeleteManifest,
		)

		// 草稿管理
		orgManifests.PUT("/:id/draft",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.SaveManifestDraft,
		)

		// 版本管理
		orgManifests.GET("/:id/versions",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ListManifestVersions,
		)
		orgManifests.POST("/:id/versions",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.PublishManifestVersion,
		)
		orgManifests.GET("/:id/versions/:version_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.GetManifestVersion,
		)

		// 部署管理
		orgManifests.GET("/:id/deployments",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ListManifestDeployments,
		)
		orgManifests.POST("/:id/deployments",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.CreateManifestDeployment,
		)
		orgManifests.GET("/:id/deployments/:deployment_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.GetManifestDeployment,
		)
		orgManifests.PUT("/:id/deployments/:deployment_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.UpdateManifestDeployment,
		)
		orgManifests.DELETE("/:id/deployments/:deployment_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			manifestHandler.DeleteManifestDeployment,
		)
		orgManifests.GET("/:id/deployments/:deployment_id/resources",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.GetManifestDeploymentResources,
		)
		orgManifests.POST("/:id/deployments/:deployment_id/uninstall",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			manifestHandler.UninstallManifestDeployment,
		)

		// 导入导出
		orgManifests.GET("/:id/export",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ExportManifestHCL,
		)
		orgManifests.GET("/:id/export-zip",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ExportManifestZip,
		)
		orgManifests.POST("/import",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.ImportManifestHCL,
		)
		orgManifests.POST("/import-json",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			manifestHandler.ImportManifestJSON,
		)
	}

	// 注意：Workspace 视角的 Manifest 路由已在 router_workspace.go 中注册
	// 这里不再重复注册，避免路由冲突
}
