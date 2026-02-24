package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupGlobalRoutes sets up global settings routes
func setupGlobalRoutes(protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	globalSettings := protected.Group("/global/settings")
	{
		// Terraform版本管理
		tfVersionController := controllers.NewTerraformVersionController(db)

		globalSettings.GET("/terraform-versions",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ"),
			tfVersionController.ListTerraformVersions,
		)

		globalSettings.GET("/terraform-versions/default",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ"),
			tfVersionController.GetDefaultVersion,
		)

		globalSettings.GET("/terraform-versions/:id",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "READ"),
			tfVersionController.GetTerraformVersion,
		)

		globalSettings.POST("/terraform-versions",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "WRITE"),
			tfVersionController.CreateTerraformVersion,
		)

		globalSettings.PUT("/terraform-versions/:id",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "WRITE"),
			tfVersionController.UpdateTerraformVersion,
		)

		globalSettings.POST("/terraform-versions/:id/set-default",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "ADMIN"),
			tfVersionController.SetDefaultVersion,
		)

		globalSettings.DELETE("/terraform-versions/:id",
			iamMiddleware.RequirePermission("TERRAFORM_VERSIONS", "ORGANIZATION", "ADMIN"),
			tfVersionController.DeleteTerraformVersion,
		)

		// Provider模板管理
		ptController := controllers.NewProviderTemplateController(db)

		globalSettings.GET("/provider-templates",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "READ"),
			ptController.ListProviderTemplates,
		)

		globalSettings.GET("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "READ"),
			ptController.GetProviderTemplate,
		)

		globalSettings.POST("/provider-templates",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "WRITE"),
			ptController.CreateProviderTemplate,
		)

		globalSettings.PUT("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "WRITE"),
			ptController.UpdateProviderTemplate,
		)

		globalSettings.POST("/provider-templates/:id/set-default",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "ADMIN"),
			ptController.SetDefaultTemplate,
		)

		globalSettings.DELETE("/provider-templates/:id",
			iamMiddleware.RequirePermission("PROVIDER_TEMPLATES", "ORGANIZATION", "ADMIN"),
			ptController.DeleteProviderTemplate,
		)

		// AI配置管理
		aiController := controllers.NewAIController(db)

		globalSettings.GET("/ai-configs",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ"),
			aiController.ListConfigs,
		)

		globalSettings.POST("/ai-configs",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE"),
			aiController.CreateConfig,
		)

		globalSettings.GET("/ai-configs/:id",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ"),
			aiController.GetConfig,
		)

		globalSettings.PUT("/ai-configs/:id",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE"),
			aiController.UpdateConfig,
		)

		globalSettings.DELETE("/ai-configs/:id",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "ADMIN"),
			aiController.DeleteConfig,
		)

		globalSettings.PUT("/ai-configs/priorities",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "WRITE"),
			aiController.BatchUpdatePriorities,
		)

		globalSettings.PUT("/ai-configs/:id/set-default",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "ADMIN"),
			aiController.SetAsDefault,
		)

		globalSettings.GET("/ai-config/regions",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ"),
			aiController.GetAvailableRegions,
		)

		globalSettings.GET("/ai-config/models",
			iamMiddleware.RequirePermission("AI_CONFIGS", "ORGANIZATION", "READ"),
			aiController.GetAvailableModels,
		)

		// 平台配置管理
		platformConfigHandler := handlers.NewPlatformConfigHandler(db)

		globalSettings.GET("/platform-config",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			platformConfigHandler.GetPlatformConfig,
		)

		globalSettings.PUT("/platform-config",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			platformConfigHandler.UpdatePlatformConfig,
		)

		// MFA全局配置管理
		mfaHandler := handlers.NewMFAHandler(db)

		globalSettings.GET("/mfa",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "READ"),
			mfaHandler.GetMFAConfig,
		)

		globalSettings.PUT("/mfa",
			iamMiddleware.RequirePermission("SYSTEM_SETTINGS", "ORGANIZATION", "ADMIN"),
			mfaHandler.UpdateMFAConfig,
		)
	}

	// 管理员用户MFA管理路由
	adminUsers := protected.Group("/admin/users")
	{
		mfaHandler := handlers.NewMFAHandler(db)

		adminUsers.GET("/:user_id/mfa/status",
			iamMiddleware.RequirePermission("USER_MANAGEMENT", "ORGANIZATION", "READ"),
			mfaHandler.GetUserMFAStatus,
		)

		adminUsers.POST("/:user_id/mfa/reset",
			iamMiddleware.RequirePermission("USER_MANAGEMENT", "ORGANIZATION", "ADMIN"),
			mfaHandler.ResetUserMFA,
		)
	}
}
