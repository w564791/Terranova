package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupGlobalRoutes sets up global settings routes
func setupGlobalRoutes(adminProtected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现global settings路由
	// 参考原router.go中的:
	// - globalSettings := adminProtected.Group("/global/settings")
	// - Terraform版本管理
	// - AI配置管理
	globalSettings := adminProtected.Group("/global/settings")
	{
		// Terraform版本管理 - 添加IAM权限检查
		tfVersionController := controllers.NewTerraformVersionController(db)

		globalSettings.GET("/terraform-versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.ListTerraformVersions(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				tfVersionController.ListTerraformVersions(c)
			}
		})

		globalSettings.GET("/terraform-versions/default", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.GetDefaultVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				tfVersionController.GetDefaultVersion(c)
			}
		})

		globalSettings.GET("/terraform-versions/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.GetTerraformVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				tfVersionController.GetTerraformVersion(c)
			}
		})

		globalSettings.POST("/terraform-versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.CreateTerraformVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				tfVersionController.CreateTerraformVersion(c)
			}
		})

		globalSettings.PUT("/terraform-versions/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.UpdateTerraformVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				tfVersionController.UpdateTerraformVersion(c)
			}
		})

		globalSettings.POST("/terraform-versions/:id/set-default", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.SetDefaultVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				tfVersionController.SetDefaultVersion(c)
			}
		})

		globalSettings.DELETE("/terraform-versions/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				tfVersionController.DeleteTerraformVersion(c)
				return
			}
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				tfVersionController.DeleteTerraformVersion(c)
			}
		})

		// AI配置管理 - 添加IAM权限检查
		aiController := controllers.NewAIController(db)

		globalSettings.GET("/ai-configs", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.ListConfigs(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				aiController.ListConfigs(c)
			}
		})

		globalSettings.POST("/ai-configs", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.CreateConfig(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				aiController.CreateConfig(c)
			}
		})

		globalSettings.GET("/ai-configs/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.GetConfig(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				aiController.GetConfig(c)
			}
		})

		globalSettings.PUT("/ai-configs/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.UpdateConfig(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				aiController.UpdateConfig(c)
			}
		})

		globalSettings.DELETE("/ai-configs/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.DeleteConfig(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				aiController.DeleteConfig(c)
			}
		})

		globalSettings.PUT("/ai-configs/priorities", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.BatchUpdatePriorities(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				aiController.BatchUpdatePriorities(c)
			}
		})

		globalSettings.PUT("/ai-configs/:id/set-default", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.SetAsDefault(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				aiController.SetAsDefault(c)
			}
		})

		globalSettings.GET("/ai-config/regions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.GetAvailableRegions(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				aiController.GetAvailableRegions(c)
			}
		})

		globalSettings.GET("/ai-config/models", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				aiController.GetAvailableModels(c)
				return
			}
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				aiController.GetAvailableModels(c)
			}
		})

		// 平台配置管理 - 仅限 admin
		platformConfigHandler := handlers.NewPlatformConfigHandler(db)

		globalSettings.GET("/platform-config", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				platformConfigHandler.GetPlatformConfig(c)
				return
			}
			// 非 admin 用户也可以读取平台配置（用于显示）
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				platformConfigHandler.GetPlatformConfig(c)
			}
		})

		globalSettings.PUT("/platform-config", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				platformConfigHandler.UpdatePlatformConfig(c)
				return
			}
			// 只有 admin 可以修改平台配置
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				platformConfigHandler.UpdatePlatformConfig(c)
			}
		})

		// MFA全局配置管理 - 仅限 admin
		mfaHandler := handlers.NewMFAHandler(db)

		globalSettings.GET("/mfa", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				mfaHandler.GetMFAConfig(c)
				return
			}
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				mfaHandler.GetMFAConfig(c)
			}
		})

		globalSettings.PUT("/mfa", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				mfaHandler.UpdateMFAConfig(c)
				return
			}
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				mfaHandler.UpdateMFAConfig(c)
			}
		})
	}

	// 管理员用户MFA管理路由
	adminUsers := adminProtected.Group("/admin/users")
	{
		mfaHandler := handlers.NewMFAHandler(db)

		adminUsers.GET("/:user_id/mfa/status", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				mfaHandler.GetUserMFAStatus(c)
				return
			}
			iamMiddleware.RequirePermission("USER_MANAGEMENT", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				mfaHandler.GetUserMFAStatus(c)
			}
		})

		adminUsers.POST("/:user_id/mfa/reset", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				mfaHandler.ResetUserMFA(c)
				return
			}
			iamMiddleware.RequirePermission("USER_MANAGEMENT", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				mfaHandler.ResetUserMFA(c)
			}
		})
	}
}
