package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupSSORoutes 注册 SSO 相关路由
func setupSSORoutes(api *gin.RouterGroup, db *gorm.DB) {
	ssoHandler := handlers.NewSSOHandler(db)

	// ============================================
	// 公开端点（无需认证）
	// ============================================
	ssoPublic := api.Group("/auth/sso")
	{
		// 获取可用的 SSO Provider 列表（登录页展示用）
		ssoPublic.GET("/providers", ssoHandler.GetProviders)

		// 发起 SSO 登录
		ssoPublic.GET("/:provider/login", ssoHandler.Login)

		// SSO 回调处理（API 模式，返回 JSON）
		ssoPublic.GET("/:provider/callback", ssoHandler.Callback)
		ssoPublic.POST("/:provider/callback", ssoHandler.Callback)

		// SSO 回调处理（重定向模式，重定向到前端页面）
		ssoPublic.GET("/:provider/callback/redirect", ssoHandler.CallbackRedirect)
	}

	// ============================================
	// 需要认证的端点
	// ============================================
	ssoAuth := api.Group("/auth/sso")
	ssoAuth.Use(middleware.JWTAuth())
	{
		// 获取当前用户绑定的身份列表
		ssoAuth.GET("/identities", ssoHandler.GetIdentities)

		// 绑定新的 SSO 身份
		ssoAuth.POST("/identities/link", ssoHandler.LinkIdentity)

		// 解绑 SSO 身份
		ssoAuth.DELETE("/identities/:id", ssoHandler.UnlinkIdentity)

		// 设置主要登录方式
		ssoAuth.PUT("/identities/:id/primary", ssoHandler.SetPrimaryIdentity)
	}

	// ============================================
	// 管理端点（需要管理员权限）
	// ============================================
	ssoAdmin := api.Group("/admin/sso")
	ssoAdmin.Use(middleware.JWTAuth())
	ssoAdmin.Use(middleware.RequireRole("admin"))
	{
		// Provider 配置管理
		ssoAdmin.GET("/providers", ssoHandler.AdminGetProviders)
		ssoAdmin.GET("/providers/:id", ssoHandler.AdminGetProvider)
		ssoAdmin.POST("/providers", ssoHandler.AdminCreateProvider)
		ssoAdmin.PUT("/providers/:id", ssoHandler.AdminUpdateProvider)
		ssoAdmin.DELETE("/providers/:id", ssoHandler.AdminDeleteProvider)

		// SSO 全局配置
		ssoAdmin.GET("/config", ssoHandler.AdminGetSSOConfig)
		ssoAdmin.PUT("/config", ssoHandler.AdminUpdateSSOConfig)

		// SSO 登录日志
		ssoAdmin.GET("/logs", ssoHandler.AdminGetLoginLogs)
	}
}
