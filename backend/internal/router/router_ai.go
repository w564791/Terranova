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

// postSyncWorker 全局 post-sync worker 实例
var postSyncWorker *services.PostSyncWorker

// GetPostSyncWorker 获取全局 post-sync worker 实例
func GetPostSyncWorker() *services.PostSyncWorker {
	return postSyncWorker
}

// assessmentWorker 全局 assessment worker 实例
var assessmentWorker *services.AssessmentWorker

// SetAssessmentWorker 设置全局 assessment worker 实例（在 main.go 中调用）
func SetAssessmentWorker(w *services.AssessmentWorker) {
	assessmentWorker = w
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
		aiCMDBSkillController := controllers.NewAICMDBSkillController(db, assessmentWorker)

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

		// ========== Manifest AI 路由（生成/修复 + 检查）==========
		manifestAIController := controllers.NewManifestAIController(db)

		// manifest 资源生成/修复（SSE）- 使用AI_ANALYSIS权限
		ai.POST("/manifest/generate-resource-sse",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			manifestAIController.GenerateResourceSSE,
		)

		// manifest 草稿检查（SSE）- 使用AI_ANALYSIS权限
		ai.POST("/manifest/check-sse",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			manifestAIController.CheckDraftSSE,
		)

		// manifest AI 会话(按用户隔离,READ 即可访问自己的会话)
		manifestSessionPerm := iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
		})
		ai.GET("/manifest/sessions", manifestSessionPerm, manifestAIController.ListSessions)
		ai.POST("/manifest/sessions", manifestSessionPerm, manifestAIController.CreateSession)
		ai.GET("/manifest/sessions/:sid/messages", manifestSessionPerm, manifestAIController.GetSessionMessages)
		ai.DELETE("/manifest/sessions/:sid", manifestSessionPerm, manifestAIController.DeleteSession)

		// 预览组装后的 Prompt（调试用）
		ai.POST("/skill/preview-prompt",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			// Skill definitions are platform-global. Tenant AI administrators can
			// operate their own AI features but must not inspect the globally
			// assembled prompts or embedded skill content.
			middleware.RequireSystemAdmin(),
			aiCMDBSkillController.PreviewAssembledPrompt,
		)

		// Skill 使用行为上报
		skillController := controllers.NewSkillController(db, assessmentWorker)
		ai.PUT("/skill-usage/:id/action",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			skillController.UpdateSkillUsageAction,
		)
		ai.PUT("/skill-usage/by-capability",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			skillController.UpdateSkillUsageByCapability,
		)
		ai.GET("/skill-usage/pending-feedback",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			skillController.GetPendingFeedback,
		)
		ai.PUT("/skill-usage/:id/feedback",
			iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
				{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
			}),
			skillController.SubmitFeedback,
		)

		// Embedding 相关路由
		// 初始化 embedding worker（如果还没有初始化）
		if embeddingWorker == nil {
			embeddingWorker = services.NewEmbeddingWorker(db)
		}
		if postSyncWorker == nil {
			postSyncWorker = services.NewPostSyncWorker(db)
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

		// CMDB 搜索结果 AI 解读（cmdb_search_summary capability，与 cmdb_query_plan 业务查询计划分离）
		searchSummaryPerm := iamMiddleware.RequireAnyPermission([]middleware.PermissionRequirement{
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "READ"},
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "WRITE"},
			{ResourceType: "AI_ANALYSIS", ScopeType: "ORGANIZATION", RequiredLevel: "ADMIN"},
		})
		ai.POST("/cmdb/search-summary", searchSummaryPerm, embeddingController.SearchSummary)
		// 进度 SSE（推荐前端使用）
		ai.POST("/cmdb/search-summary-sse", searchSummaryPerm, embeddingController.SearchSummarySSE)
	}

	// Full embedding sync has no tenant filter in the worker, so it is a
	// platform operation rather than an organization-level AI admin operation.
	if embeddingWorker == nil {
		embeddingWorker = services.NewEmbeddingWorker(db)
	}
	systemEmbedding := api.Group("/admin")
	systemEmbedding.Use(middleware.JWTAuth())
	systemEmbedding.Use(middleware.AuditLogger(db))
	systemEmbedding.Use(middleware.RequireSystemAdmin())
	systemEmbedding.POST("/embedding/sync-all", controllers.NewEmbeddingController(db, embeddingWorker).SyncAllWorkspaces)

	// Admin 路由 - embedding 管理
	admin := api.Group("/admin")
	admin.Use(middleware.JWTAuth())
	admin.Use(middleware.AuditLogger(db))
	admin.Use(iamMiddleware.RequirePermission("AI_ANALYSIS", "ORGANIZATION", "ADMIN"))
	// AI configuration, skills, assessment data and embedding caches are
	// global tables today, not tenant-owned resources. An organization-scoped
	// administrator must therefore not be able to read or mutate them for all
	// other tenants.
	admin.Use(middleware.RequireSystemAdmin())
	{
		if embeddingWorker == nil {
			embeddingWorker = services.NewEmbeddingWorker(db)
		}
		embeddingController := controllers.NewEmbeddingController(db, embeddingWorker)

		// 获取 worker 状态
		admin.GET("/embedding/status", embeddingController.GetWorkerStatus)

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

		// ========== Skill Assessment Dashboard API ==========
		assessmentController := controllers.NewSkillAssessmentController(db)
		admin.GET("/skill-assessment/overview", assessmentController.GetOverview)
		admin.GET("/skill-assessment/detail", assessmentController.GetCapabilityDetail)
		admin.GET("/skill-assessment/compare", assessmentController.CompareVersions)
		admin.GET("/skill-assessment/top-violations", assessmentController.GetTopViolations)

		// ========== Summary Assessment Dashboard API ==========
		summaryAssessmentController := controllers.NewSummaryAssessmentController(db)
		admin.GET("/summary-assessment/overview", summaryAssessmentController.GetOverview)
		admin.GET("/summary-assessment/issue-resources", summaryAssessmentController.GetIssueResources)
		admin.POST("/summary-assessment/regenerate", summaryAssessmentController.RegenerateSummaries)

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
	// This group is registered separately from setupWorkspaceRoutes, so it must
	// install the same workspace-to-organization fence explicitly.
	workspaces.Use(middleware.EnforceWorkspaceOrgBinding(db))
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
