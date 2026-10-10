package router

import (
	"iac-platform/controllers"
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
	manifestHandler := handlers.NewManifestHandler(db, iamMiddleware)

	// ========== 新版 manifest (VS Code Web 工作区,软链接架构) ==========
	registerManifestV2Routes(r, db, iamMiddleware)
	// =================================================================

	// Organization 级别的 Manifest 顶层 CRUD - 组织级 MANIFESTS 权限
	//   READ: 列表/详情/导出;WRITE: 创建/更新;ADMIN: 删除(归档在 UpdateManifest 内要求 ADMIN)
	// (文件/版本/部署写操作全部走 registerManifestV2Routes)
	orgManifests := r.Group("/organizations/:org_id/manifests")
	orgManifests.Use(middleware.JWTAuth())
	{
		orgManifests.GET("",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			manifestHandler.ListManifests,
		)
		orgManifests.POST("",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			manifestHandler.CreateManifest,
		)
		orgManifests.GET("/:id",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			manifestHandler.GetManifest,
		)
		orgManifests.PUT("/:id",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			manifestHandler.UpdateManifest,
		)
		orgManifests.DELETE("/:id",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "ADMIN"),
			manifestHandler.DeleteManifest,
		)
		orgManifests.GET("/:id/export-zip",
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
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
	schemaH := handlers.NewManifestProviderSchemaHandler(db)

	g := r.Group("/organizations/:org_id/manifests/:id")
	g.Use(middleware.JWTAuth())
	// 本组 handler 只按 manifest_id 查询:每条路由在 RequirePermission(写入 auth_org_id)
	// 之后挂 ManifestInAuthOrg,path manifest 不属于该 org / 不存在 => 404。
	inOrg := manifestRouteChain(handlers.ManifestInAuthOrg(db))
	// git manifests: the editor is read-only (changes go through git)
	nativeOnly := handlers.ManifestNativeOnly(db)
	gitH := handlers.NewManifestGitHandler(db)
	{
		// === post_init 落库的 provider 类型目录（编辑器补全）===
		g.GET("/provider-schemas", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			schemaH.GetProviderSchemas,
		)...)

		// === 文件 CRUD (草稿区,作用于当前用户私有副本) ===
		g.GET("/files", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			filesH.ListFiles,
		)...)
		g.GET("/files/*path", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			filesH.ReadFile,
		)...)
		g.PUT("/files/*path", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			middleware.LimitRequestBodySize(handlers.ManifestMaxFileSize),
			nativeOnly,
			filesH.PutFile,
		)...)
		g.DELETE("/files/*path", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			nativeOnly,
			filesH.DeleteFile,
		)...)
		g.POST("/files/_move", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			nativeOnly,
			filesH.MoveFile,
		)...)
		g.POST("/files/_move_dir", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			nativeOnly,
			filesH.MoveDir,
		)...)
		g.POST("/files/_delete_dir", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			nativeOnly,
			filesH.DeleteDir,
		)...)
		g.POST("/draft/_reset_from", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			nativeOnly,
			filesH.ResetDraftFromVersion,
		)...)
		g.POST("/draft/_export", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			filesH.ExportDraft,
		)...)

		// === 版本(新设计:仅读 + 发布;旧版本走老 manifest_handler 直至 PR4) ===
		g.GET("/v2/versions", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.ListVersions,
		)...)
		g.GET("/v2/versions/:version_id", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.GetVersion,
		)...)
		g.POST("/v2/versions", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			versionsH.PublishVersion,
		)...)
		g.GET("/v2/versions/:version_id/diff", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.DiffVersions,
		)...)
		g.GET("/v2/versions/:version_id/workdirs", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.ListWorkdirs,
		)...)
		g.GET("/v2/draft/diff", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.DiffDraft,
		)...)
		g.POST("/v2/versions/:version_id/files/_export", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			versionsH.ExportVersion,
		)...)

		// === git 来源:发布用的 commit 选择器(按请求现签单仓库 contents:read token)===
		// WRITE:读私有仓库元信息只给能发布的人
		g.GET("/git/branches", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			gitH.ListBranches,
		)...)
		g.GET("/git/commits", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE"),
			gitH.ListCommits,
		)...)

		// === 部署(新设计 install/upgrade/uninstall,纯元信息) ===
		// 全部要求 MANIFESTS READ;install/upgrade/uninstall 另在 handler 内对目标 workspace
		// (body 或 deployment 记录)显式校验 WORKSPACE_RESOURCES WRITE;
		// get 校验 workspace 可读;variable-preview 校验 WORKSPACE_VARIABLES READ。
		// 列表按调用者可读 workspace 服务端过滤(RequireWorkspaceListAccess 放入 allow-list)
		g.GET("/v2/deployments", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			iamMiddleware.RequireWorkspaceListAccess(),
			deploysH.ListDeployments,
		)...)
		g.GET("/v2/deployments/:deployment_id", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.GetDeployment,
		)...)
		// 首次安装前预览(无 deployment):与 /:deployment_id/variable-preview 共用实现,
		// handler 内校验 workspace 属于本 org、version 已发布且属于本 manifest、
		// 目标 workspace WORKSPACE_VARIABLES READ、varset 可挂载
		g.POST("/v2/deployments/variable-preview", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.FirstInstallVariablePreview,
		)...)
		g.POST("/v2/deployments/install", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.Install,
		)...)
		g.POST("/v2/deployments/:deployment_id/upgrade", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.Upgrade,
		)...)
		g.POST("/v2/deployments/:deployment_id/uninstall", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.Uninstall,
		)...)
		g.POST("/v2/deployments/:deployment_id/variable-preview", inOrg(
			iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "READ"),
			deploysH.VariablePreview,
		)...)
	}

	// === Variable Set 反向关联 (用于 varset 详情页 "被以下 deployment 使用") ===
	// varset 须在调用组织内可见(与 /variable-sets/:varset_id 读路由同一守卫)
	r.GET("/variable-sets/:varset_id/manifest-deployments",
		append([]gin.HandlerFunc{middleware.JWTAuth()}, manifestRouteChain(
			controllers.NewVariableSetController(db).VarsetInAuthOrg(false))(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			deploysH.VarsetReverseLookup,
		)...)...,
	)

	// === Workspace 视角的 manifest 摘要 (资源页徽章 / 顶部 banner 共用) ===
	// 注意: 参数名必须用 :id,与 router_workspace.go 已注册的 /workspaces/:id 一致;
	// gin 不允许同一前缀下出现不同名通配符(:id vs :workspace_id 会 panic)。
	r.GET("/workspaces/:id/manifest-summary",
		middleware.JWTAuth(),
		iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
		}),
		// This route is registered outside setupWorkspaceRoutes, so it does
		// not inherit that group's tenant fence. Bind the path workspace after
		// IAM resolves auth_org_id; otherwise an org-level grant could read a
		// guessed workspace ID from another tenant.
		middleware.EnforceWorkspaceOrgBinding(db),
		deploysH.GetWorkspaceManifestSummary,
	)

	// === Manifest 编辑器 IntelliSense 用的只读 module/demo 摘要 (spec §7.6) ===
	editorH := handlers.NewManifestEditorHandler(db)
	editor := r.Group("/manifest-editor")
	editor.Use(middleware.JWTAuth())
	{
		editor.GET("/modules",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			editorH.ListModules,
		)
		editor.GET("/modules/:module_id/demos",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			editorH.ListDemos,
		)
		editor.GET("/modules/:module_id/inputs",
			iamMiddleware.RequirePermission("MODULES", "ORGANIZATION", "READ"),
			editorH.ListModuleInputs,
		)
	}
}

// manifestRouteChain 返回在路由级权限中间件之后插入 orgGuard 的构造器:
// inOrg(perm, h...) => perm, orgGuard, h...
func manifestRouteChain(orgGuard gin.HandlerFunc) func(perm gin.HandlerFunc, rest ...gin.HandlerFunc) []gin.HandlerFunc {
	return func(perm gin.HandlerFunc, rest ...gin.HandlerFunc) []gin.HandlerFunc {
		return append([]gin.HandlerFunc{perm, orgGuard}, rest...)
	}
}

// RegisterGitHubAppRoutes GitHub App installations (org ADMIN: list,
// connect, delete; manual registration is gone; MANIFESTS WRITE: read-only
// usable list and repositories), the App's setup callback
// (public; authenticated by its signed single-use state plus user-to-server
// OAuth proof) and the GitHub webhook (public; X-Hub-Signature-256).
func RegisterGitHubAppRoutes(api *gin.RouterGroup, protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	appH := handlers.NewGitHubAppHandler(db).WithPermissionChecker(iamMiddleware.Checker())
	orgAdmin := iamMiddleware.RequirePermission("ORGANIZATION", "ORGANIZATION", "ADMIN")
	manifestsWrite := iamMiddleware.RequirePermission("MANIFESTS", "ORGANIZATION", "WRITE")
	ghApp := protected.Group("/organizations/:org_id/github-app") // protected carries JWTAuth
	{
		// read-only for manifest authors: bound installations of the org
		// (id + account) and their repositories (metadata:read token)
		ghApp.GET("/available-installations", manifestsWrite, appH.ListUsableInstallations)
		ghApp.GET("/available-installations/:installation_id", manifestsWrite, appH.GetUsableInstallation)
		ghApp.GET("/installations/:installation_id/repositories", manifestsWrite, appH.ListInstallationRepositories)
		// org ADMIN
		ghApp.POST("/connect", orgAdmin, appH.Connect)
		ghApp.GET("/installations", orgAdmin, appH.ListInstallations)
		ghApp.POST("/installations", orgAdmin, appH.RegisterInstallation) // 410: removed
		ghApp.DELETE("/installations/:installation_id", orgAdmin, appH.DeleteInstallation)
	}
	api.GET("/github-app/setup/callback", appH.SetupCallback)
	api.POST("/webhooks/github", handlers.NewGitHubWebhookHandler(db).Receive)
}
