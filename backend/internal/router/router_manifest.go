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
func RegisterManifestRoutes(r *gin.RouterGroup, db *gorm.DB, queueManager TaskQueueManagerInterface) {
	manifestHandler := handlers.NewManifestHandler(db)
	if queueManager != nil {
		manifestHandler.SetQueueManager(queueManager)
	}

	// Organization 级别的 Manifest 路由
	orgManifests := r.Group("/organizations/:org_id/manifests")
	orgManifests.Use(middleware.JWTAuth())
	{
		// Manifest CRUD
		orgManifests.GET("", manifestHandler.ListManifests)         // 列表
		orgManifests.POST("", manifestHandler.CreateManifest)       // 创建
		orgManifests.GET("/:id", manifestHandler.GetManifest)       // 详情
		orgManifests.PUT("/:id", manifestHandler.UpdateManifest)    // 更新
		orgManifests.DELETE("/:id", manifestHandler.DeleteManifest) // 删除

		// 草稿管理
		orgManifests.PUT("/:id/draft", manifestHandler.SaveManifestDraft) // 保存草稿

		// 版本管理
		orgManifests.GET("/:id/versions", manifestHandler.ListManifestVersions)           // 版本列表
		orgManifests.POST("/:id/versions", manifestHandler.PublishManifestVersion)        // 发布版本
		orgManifests.GET("/:id/versions/:version_id", manifestHandler.GetManifestVersion) // 版本详情

		// 部署管理
		orgManifests.GET("/:id/deployments", manifestHandler.ListManifestDeployments)                                 // 部署列表
		orgManifests.POST("/:id/deployments", manifestHandler.CreateManifestDeployment)                               // 创建部署
		orgManifests.GET("/:id/deployments/:deployment_id", manifestHandler.GetManifestDeployment)                    // 部署详情
		orgManifests.PUT("/:id/deployments/:deployment_id", manifestHandler.UpdateManifestDeployment)                 // 更新部署
		orgManifests.DELETE("/:id/deployments/:deployment_id", manifestHandler.DeleteManifestDeployment)              // 删除部署
		orgManifests.GET("/:id/deployments/:deployment_id/resources", manifestHandler.GetManifestDeploymentResources) // 部署资源
		orgManifests.POST("/:id/deployments/:deployment_id/uninstall", manifestHandler.UninstallManifestDeployment)   // 卸载部署

		// 导入导出
		orgManifests.GET("/:id/export", manifestHandler.ExportManifestHCL)     // 导出 HCL
		orgManifests.GET("/:id/export-zip", manifestHandler.ExportManifestZip) // 导出 ZIP (包含 manifest.json 和 .tf)
		orgManifests.POST("/import", manifestHandler.ImportManifestHCL)        // 导入 HCL
		orgManifests.POST("/import-json", manifestHandler.ImportManifestJSON)  // 导入 manifest.json
	}

	// 注意：Workspace 视角的 Manifest 路由已在 router_workspace.go 中注册
	// 这里不再重复注册，避免路由冲突
}
