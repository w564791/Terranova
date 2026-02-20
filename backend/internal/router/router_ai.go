package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// embeddingWorker 全局 embedding worker 实例
var embeddingWorker *services.EmbeddingWorker

// GetEmbeddingWorker 获取全局 embedding worker 实例
func GetEmbeddingWorker() *services.EmbeddingWorker {
	return embeddingWorker
}

// setupAIRoutes sets up AI analysis routes
func setupAIRoutes(api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// AI分析 - 使用AI_ANALYSIS权限，允许WRITE和ADMIN级别访问
	// AI分析路由 - 使用IAM权限控制
	ai := api.Group("/ai")
	ai.Use(middleware.JWTAuth())
	ai.Use(middleware.AuditLogger(db))
	{
		aiController := controllers.NewAIController(db)

		ai.POST("/analyze-error",
			// 使用AI_ANALYSIS权限，WRITE和ADMIN级别都可以访问
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiController.AnalyzeError,
		)

		// AI 表单助手路由
		aiFormService := services.NewAIFormService(db)
		aiFormController := controllers.NewAIFormController(aiFormService)

		// 生成表单配置 - 使用AI_ANALYSIS权限
		ai.POST("/form/generate",
			// 使用AI_ANALYSIS权限，WRITE和ADMIN级别都可以访问
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiFormController.GenerateConfig,
		)

		// AI + CMDB 集成路由
		aiCMDBController := controllers.NewAICMDBController(db)

		// 带 CMDB 查询的配置生成 - 使用AI_ANALYSIS权限
		ai.POST("/form/generate-with-cmdb",
			// 使用AI_ANALYSIS权限，WRITE和ADMIN级别都可以访问
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiCMDBController.GenerateConfigWithCMDB,
		)

		// AI + CMDB + Skill 集成路由（新版 Skill 模式）
		aiCMDBSkillController := controllers.NewAICMDBSkillController(db)

		// 使用 Skill 模式的配置生成 - 使用AI_ANALYSIS权限
		ai.POST("/form/generate-with-cmdb-skill",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiCMDBSkillController.GenerateConfigWithCMDBSkill,
		)

		// 使用 SSE 实时推送进度的配置生成 - 使用AI_ANALYSIS权限
		// 使用 POST 方法，参数通过 body 传递
		ai.POST("/form/generate-with-cmdb-skill-sse",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiCMDBSkillController.GenerateConfigWithCMDBSkillSSE,
		)

		// 预览组装后的 Prompt（调试用）
		ai.POST("/skill/preview-prompt",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			aiCMDBSkillController.PreviewAssembledPrompt,
		)

		// Embedding 相关路由
		// 初始化 embedding worker（如果还没有初始化）
		if embeddingWorker == nil {
			embeddingWorker = services.NewEmbeddingWorker(db)
		}
		embeddingController := controllers.NewEmbeddingController(db, embeddingWorker)

		// 获取 embedding 配置状态
		ai.GET("/embedding/config-status",
			iamMiddleware.RequirePermission("AI_ANALYSIS", "ORGANIZATION", "READ"),
			embeddingController.GetConfigStatus,
		)

		// 向量搜索
		ai.POST("/cmdb/vector-search",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			embeddingController.VectorSearch,
		)
	}

	// Admin 路由 - embedding 管理
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth())
	admin.Use(middleware.AuditLogger(db))
	admin.Use(iamMiddleware.RequirePermission("AI_ANALYSIS", "ORGANIZATION", "ADMIN"))
	{
		if embeddingWorker == nil {
			embeddingWorker = services.NewEmbeddingWorker(db)
		}
		embeddingController := controllers.NewEmbeddingController(db, embeddingWorker)

		// 获取 worker 状态
		admin.GET("/embedding/status", embeddingController.GetWorkerStatus)

		// 全量同步所有 Workspace
		admin.POST("/embedding/sync-all", embeddingController.SyncAllWorkspaces)

		// ========== Skill 管理 API ==========
		skillController := controllers.NewSkillController(db)
		skills := admin.Group("/skills")
		{
			skills.GET("", skillController.ListSkills)
			// 预览 Domain Skill 自动发现（必须在 /:id 之前）
			skills.GET("/preview-discovery", skillController.PreviewDomainSkillDiscovery)
			skills.GET("/:id", skillController.GetSkill)
			skills.POST("", skillController.CreateSkill)
			skills.PUT("/:id", skillController.UpdateSkill)
			skills.DELETE("/:id", skillController.DeleteSkill)
			skills.POST("/:id/activate", skillController.ActivateSkill)
			skills.POST("/:id/deactivate", skillController.DeactivateSkill)
			skills.GET("/:id/usage-stats", skillController.GetSkillUsageStats)
		}

		// ========== Module Skill API ==========
		moduleSkillController := controllers.NewModuleSkillController(db)
		admin.GET("/modules/:module_id/skill", moduleSkillController.GetModuleSkill)
		admin.POST("/modules/:module_id/skill/generate", moduleSkillController.GenerateModuleSkill)
		admin.PUT("/modules/:module_id/skill", moduleSkillController.UpdateModuleSkill)
		admin.GET("/modules/:module_id/skill/preview", moduleSkillController.PreviewModuleSkill)

		// ========== Module Version Skill API ==========
		moduleVersionSkillController := controllers.NewModuleVersionSkillController(db)
		admin.GET("/module-versions/:id/skill", moduleVersionSkillController.GetSkill)
		admin.POST("/module-versions/:id/skill/generate", moduleVersionSkillController.GenerateFromSchema)
		admin.PUT("/module-versions/:id/skill", moduleVersionSkillController.UpdateCustomContent)
		admin.POST("/module-versions/:id/skill/inherit", moduleVersionSkillController.InheritFromVersion)
		admin.DELETE("/module-versions/:id/skill", moduleVersionSkillController.DeleteSkill)

		// ========== Embedding Cache API ==========
		embeddingCacheController := controllers.NewEmbeddingCacheController(db)
		embeddingCache := admin.Group("/embedding-cache")
		{
			embeddingCache.POST("/warmup", embeddingCacheController.WarmUp)
			embeddingCache.GET("/warmup/progress", embeddingCacheController.GetWarmupProgress)
			embeddingCache.GET("/stats", embeddingCacheController.GetStats)
			embeddingCache.DELETE("/clear", embeddingCacheController.ClearCache)
			embeddingCache.POST("/cleanup", embeddingCacheController.CleanupLowHit)
		}
	}

	// Workspace 级别的 embedding 路由
	// 注意：使用 :id 而不是 :workspace_id，与现有路由保持一致
	workspaces := api.Group("/workspaces")
	workspaces.Use(middleware.JWTAuth())
	workspaces.Use(middleware.AuditLogger(db))
	{
		if embeddingWorker == nil {
			embeddingWorker = services.NewEmbeddingWorker(db)
		}
		embeddingController := controllers.NewEmbeddingController(db, embeddingWorker)

		// 获取 Workspace 的 embedding 状态
		workspaces.GET("/:id/embedding-status",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "READ"},
			}),
			embeddingController.GetWorkspaceEmbeddingStatus,
		)

		// 同步指定 Workspace 的 embedding
		workspaces.POST("/:id/embedding/sync",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "WRITE"},
			}),
			embeddingController.SyncWorkspace,
		)

		// 重建指定 Workspace 的 embedding
		workspaces.POST("/:id/embedding/rebuild",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "WORKSPACES", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
				{ResourceType: "WORKSPACE_MANAGEMENT", ScopeType: "WORKSPACE", RequiredLevel: "ADMIN"},
			}),
			embeddingController.RebuildWorkspace,
		)
	}
}
