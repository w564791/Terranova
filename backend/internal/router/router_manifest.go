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

	// ========== 新版 manifest (VS Code Web 工作区,软链接架构) ==========
	registerManifestV2Routes(r, db, iamMiddleware)
	// =================================================================

	// Organization 级别的 Manifest 顶层 CRUD - 使用 SYSTEM_SETTINGS 权限
	// (文件/版本/部署写操作全部走 registerManifestV2Routes)
	orgManifests := r.Group("/organizations/:org_id/manifests")
	orgManifests.Use(middleware.JWTAuth())
	{
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
		orgManifests.GET("/:id/export-zip",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			manifestHandler.ExportManifestZip,
		)
	}

	// 注意：Workspace 视角的 Manifest 路由已在 router_workspace.go 中注册
	// 这里不再重复注册，避免路由冲突
}

// registerManifestV2Routes 注册 manifest 重构后的新版路由
//
//	/organizations/:org_id/manifests/:id/files                    草稿与版本文件 CRUD
//	/organizations/:org_id/manifests/:id/files/*path
//	/organizations/:org_id/manifests/:id/draft/_reset_from        重置草稿到指定 published 版本
//	/organizations/:org_id/manifests/:id/draft/_export            导出当前用户草稿为 zip
//	/organizations/:org_id/manifests/:id/v2/versions              新版本列表 / 发布
//	/organizations/:org_id/manifests/:id/v2/versions/:version_id  版本详情/diff/zip 导出
//	/organizations/:org_id/manifests/:id/v2/deployments           新部署 install/upgrade/uninstall
//	/organizations/:org_id/manifests/:id/v2/deployments/...
//
// 新版路由暂用 /v2 前缀避免与旧 manifest_handler 注册的路由冲突;PR4 切换时去掉前缀。
// 文件路径与 draft 路由因旧 handler 没有同名,直接注册不冲突。
func registerManifestV2Routes(r *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	filesH := handlers.NewManifestFilesHandler(db)
	versionsH := handlers.NewManifestVersionsHandler(db)
	deploysH := handlers.NewManifestDeploymentsV2Handler(db, iamMiddleware)

	g := r.Group("/organizations/:org_id/manifests/:id")
	g.Use(middleware.JWTAuth())
	{
		// === 文件 CRUD (草稿区,作用于当前用户私有副本) ===
		g.GET("/files",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			filesH.ListFiles,
		)
		g.GET("/files/*path",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			filesH.ReadFile,
		)
		g.PUT("/files/*path",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			middleware.LimitRequestBodySize(handlers.ManifestMaxFileSize),
			filesH.PutFile,
		)
		g.DELETE("/files/*path",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			filesH.DeleteFile,
		)
		g.POST("/files/_move",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			filesH.MoveFile,
		)
		g.POST("/files/_delete_dir",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			filesH.DeleteDir,
		)
		g.POST("/draft/_reset_from",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			filesH.ResetDraftFromVersion,
		)
		g.POST("/draft/_export",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			filesH.ExportDraft,
		)

		// === 版本(新设计:仅读 + 发布;旧版本走老 manifest_handler 直至 PR4) ===
		g.GET("/v2/versions",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.ListVersions,
		)
		g.GET("/v2/versions/:version_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.GetVersion,
		)
		g.POST("/v2/versions",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			versionsH.PublishVersion,
		)
		g.GET("/v2/versions/:version_id/diff",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.DiffVersions,
		)
		g.GET("/v2/versions/:version_id/workdirs",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.ListWorkdirs,
		)
		g.GET("/v2/draft/diff",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.DiffDraft,
		)
		g.POST("/v2/versions/:version_id/files/_export",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			versionsH.ExportVersion,
		)

		// === 部署(新设计 install/upgrade/uninstall,纯元信息) ===
		g.GET("/v2/deployments",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			deploysH.ListDeployments,
		)
		g.GET("/v2/deployments/:deployment_id",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			deploysH.GetDeployment,
		)
		g.POST("/v2/deployments/install",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			deploysH.Install,
		)
		g.POST("/v2/deployments/:deployment_id/upgrade",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			deploysH.Upgrade,
		)
		g.POST("/v2/deployments/:deployment_id/uninstall",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "WRITE"),
			deploysH.Uninstall,
		)
		g.POST("/v2/deployments/:deployment_id/variable-preview",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			deploysH.VariablePreview,
		)
	}

	// === Variable Set 反向关联 (用于 varset 详情页 "被以下 deployment 使用") ===
	r.GET("/variable-sets/:varset_id/manifest-deployments",
		middleware.JWTAuth(),
		iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
		deploysH.VarsetReverseLookup,
	)

	// === Workspace 视角的 manifest 摘要 (资源页徽章 / 顶部 banner 共用) ===
	// 注意: 参数名必须用 :id,与 router_workspace.go 已注册的 /workspaces/:id 一致;
	// gin 不允许同一前缀下出现不同名通配符(:id vs :workspace_id 会 panic)。
	r.GET("/workspaces/:id/manifest-summary",
		middleware.JWTAuth(),
		deploysH.GetWorkspaceManifestSummary,
	)

	// === Manifest 编辑器 IntelliSense 用的只读 module/demo 摘要 (spec §7.6) ===
	editorH := handlers.NewManifestEditorHandler(db)
	editor := r.Group("/manifest-editor")
	editor.Use(middleware.JWTAuth())
	{
		editor.GET("/modules",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			editorH.ListModules,
		)
		editor.GET("/modules/:module_id/demos",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			editorH.ListDemos,
		)
		editor.GET("/modules/:module_id/inputs",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			editorH.ListModuleInputs,
		)
	}
}
