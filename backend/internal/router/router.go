package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/iam"
	"iac-platform/internal/middleware"
	"iac-platform/internal/websocket"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"gorm.io/gorm"

	_ "iac-platform/docs" // swagger docs
)

func Setup(db *gorm.DB, streamManager *services.OutputStreamManager, wsHub *websocket.Hub, agentMetricsHub *websocket.AgentMetricsHub, queueManager *services.TaskQueueManager, rawCCHandler *handlers.RawAgentCCHandler, runTaskExecutor *services.RunTaskExecutor) *gin.Engine {
	r := gin.Default()

	// 设置全局数据库连接（用于JWT中间件查询用户信息）
	middleware.SetGlobalDB(db)

	// 中间件
	r.Use(middleware.CORS())
	r.Use(middleware.Logger())
	r.Use(middleware.ErrorHandler())

	// 健康检查
	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	// Prometheus 指标端点（复用业务端口）
	r.GET("/metrics", gin.WrapF(services.MetricsHandler()))

	// 静态文件服务 - 提供自定义CSS
	r.Static("/static", "./static")

	// Swagger文档 - 配置以改善显示
	r.GET("/swagger/*any", ginSwagger.WrapHandler(
		swaggerFiles.Handler,
		ginSwagger.DeepLinking(true),
		ginSwagger.DocExpansion("list"),
		ginSwagger.PersistAuthorization(true),
		ginSwagger.DefaultModelsExpandDepth(-1),
	))

	// API路由组
	api := r.Group("/api/v1")

	// WebSocket路由（需要认证）
	ws := api.Group("/ws")
	ws.Use(middleware.JWTAuth())
	{
		wsHandler := handlers.NewWebSocketHandler(wsHub)
		ws.GET("/editing/:session_id", wsHandler.HandleConnection)
		ws.GET("/sessions", wsHandler.GetConnectedSessions)

		// Agent Metrics WebSocket
		agentMetricsWSHandler := handlers.NewAgentMetricsWSHandler(agentMetricsHub)
		ws.GET("/agent-pools/:pool_id/metrics", agentMetricsWSHandler.HandleAgentMetricsWS)
	}

	// 系统初始化路由（无需JWT，未初始化时可用）
	setup := api.Group("/setup")
	{
		setupHandler := handlers.NewSetupHandler(db)
		setup.GET("/status", setupHandler.GetStatus)
		setup.POST("/init", setupHandler.InitAdmin)
	}

	// 认证路由（无需JWT）
	auth := api.Group("/auth")
	{
		authHandler := handlers.NewAuthHandler(db)
		auth.POST("/login", authHandler.Login)
		// auth.POST("/register", authHandler.Register)

		// MFA路由（登录流程，无需JWT，使用mfa_token认证）
		mfaHandler := handlers.NewMFAHandler(db)
		auth.POST("/mfa/verify", mfaHandler.VerifyMFALogin)
		auth.POST("/mfa/setup", mfaHandler.SetupMFAWithToken)
		auth.POST("/mfa/enable", mfaHandler.VerifyAndEnableMFAWithToken)
	}

	// SSO 路由（包含公开端点和需要认证的端点）
	setupSSORoutes(api, db)

	// Token刷新、用户信息获取和登出需要JWT认证
	api.POST("/auth/refresh", middleware.JWTAuth(), handlers.NewAuthHandler(db).RefreshToken)
	api.GET("/auth/me", middleware.JWTAuth(), handlers.NewAuthHandler(db).GetMe)
	api.POST("/auth/logout", middleware.JWTAuth(), handlers.NewAuthHandler(db).Logout)

	// Agent API routes (使用 Pool Token 认证，不需要 JWT)
	setupAgentAPIRoutes(api, db, streamManager, agentMetricsHub, runTaskExecutor, queueManager)

	// Run Task Callback routes (公开路由，不需要认证，供外部 Run Task 服务回调)
	setupRunTaskCallbackRoutes(api, db, runTaskExecutor)

	// 需要认证的路由
	protected := api.Group("")
	protected.Use(middleware.JWTAuth())
	protected.Use(middleware.AuditLogger(db))

	// 初始化IAM权限中间件
	iamMiddleware := middleware.NewIAMPermissionMiddleware(db)

	// 通用密文管理路由（需要认证）
	setupSecretRoutes(protected, db, iamMiddleware)

	// 初始化IAM服务工厂（提前初始化，供权限检查使用）
	iamFactory := iam.NewServiceFactory(db)
	permissionHandler := handlers.NewPermissionHandler(
		iamFactory.GetPermissionService(),
		iamFactory.GetPermissionChecker(),
		iamFactory.GetTeamService(),
		db,
	)

	// 权限检查API - 所有认证用户都可以调用（用于检查自己的权限）
	protected.POST("/iam/permissions/check", permissionHandler.CheckPermission)

	// MFA 设置路由 - 所有已认证用户都可以访问（不需要 IAM 权限）
	{
		mfaHandler := handlers.NewMFAHandler(db)
		mfa := protected.Group("/user/mfa")
		{
			mfa.GET("/status", mfaHandler.GetMFAStatus)
			mfa.POST("/setup", mfaHandler.SetupMFA)
			mfa.POST("/verify", mfaHandler.VerifyAndEnableMFA)
			mfa.POST("/disable", mfaHandler.DisableMFA)
			mfa.POST("/backup-codes/regenerate", mfaHandler.RegenerateBackupCodes)
		}
	}

	// Dashboard统计 - 使用IAM权限控制
	setupDashboardRoutes(api, db, iamMiddleware)

	// Remote Data Token 访问路由（公开路由，使用临时token认证，不需要JWT）
	// 这个路由必须在 setupWorkspaceRoutes 之前注册，因为它不需要JWT中间件
	setupRemoteDataPublicRoutes(api, db)

	// 工作空间管理 - 使用IAM权限控制
	// 传入 permissionService 用于创建 workspace 时自动为创建者授权
	setupWorkspaceRoutes(api, db, streamManager, iamMiddleware, wsHub, queueManager, rawCCHandler, iamFactory.GetPermissionService())
	setupModuleRoutes(api, db, iamMiddleware)
	// Project 管理 - 使用 Organization 权限控制
	setupProjectRoutes(api, db, iamMiddleware)
	// AI分析路由
	setupAIRoutes(api, db, iamMiddleware)

	// 用户路由 - 使用IAM权限检查
	setupUserRoutes(protected, db, iamMiddleware)

	setupDemoRoutes(protected, api, db, iamMiddleware)

	// Schema管理 - 使用IAM权限检查
	setupSchemaRoutes(protected, db, iamMiddleware)

	// setupTaskRoutes sets up task log routes
	setupTaskRoutes(api, db, streamManager, iamMiddleware)

	// Agent Pool 管理 - 使用IAM权限检查（需要 JWT 认证）
	setupAgentPoolRoutes(protected, db, iamMiddleware)

	// Run Task 管理 - 使用IAM权限检查（需要 JWT 认证）
	setupRunTaskRoutes(protected, db, iamMiddleware)

	// IAM权限系统
	setupIAMRoutes(protected, db, iamMiddleware)

	// 全局设置管理
	setupGlobalRoutes(protected, db, iamMiddleware)

	// 通知管理
	SetupNotificationRoutes(protected, db, iamMiddleware)

	// Manifest 可视化编排器
	RegisterManifestRoutes(protected, db, queueManager, iamMiddleware)

	// CMDB资源索引（需要认证，只读功能对所有用户开放）
	SetupCMDBRoutes(protected, db)

	// 注意：Drift 检测路由已在 router_workspace.go 中注册

	return r
}
