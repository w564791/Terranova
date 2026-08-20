package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/iam"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupApplicationPrincipalRoutes 选项 A：Application 密钥作为 IAM 主体的 API 面
//
// 路径前缀：/api/v1/app/*
// 认证：X-App-Key + X-App-Secret（ApplicationAuthOnly）
// 授权：业务接口叠加 RequirePermission / handler 内双检
//
// 与 Agent Pool Token 路由分离：任务执行/锁/state 仍走 /agents/* + Pool Token。
func setupApplicationPrincipalRoutes(api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	factory := iam.NewServiceFactory(db)
	appPrincipal := handlers.NewApplicationPrincipalHandler(factory.GetPermissionChecker())
	appWS := handlers.NewApplicationWorkspaceHandler(db, iamMiddleware)

	appGroup := api.Group("/app")
	appGroup.Use(middleware.ApplicationAuthOnly(db), middleware.AuditLogger(db))
	{
		// 身份与权限探查（无需额外 IAM grant，有效 App 即可）
		appGroup.GET("/whoami", appPrincipal.WhoAmI)
		appGroup.POST("/permissions/check", appPrincipal.CheckPermission)

		// 工作区只读：须 org 级 WORKSPACES READ；列表按 auth_org 过滤
		appGroup.GET("/workspaces",
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ"),
			appWS.ListWorkspaces,
		)
		// 详情：handler 内 org WORKSPACES READ 或 workspace MANAGEMENT READ；并校验 org 归属
		appGroup.GET("/workspaces/:id",
			appWS.GetWorkspace,
		)
	}
}
