package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupGlobalRoutes sets up platform-global settings routes.
//
// None of the resources below carry an organization identifier. Authorizing
// them with an organization-scoped IAM grant lets an administrator in any
// tenant change settings for every tenant. Keep the whole group behind the
// platform-admin boundary instead of trying to infer a tenant from the request.
func setupGlobalRoutes(protected *gin.RouterGroup, db *gorm.DB, _ *middleware.IAMPermissionMiddleware) {
	globalSettings := protected.Group("/global/settings", middleware.RequireSystemAdmin())
	{
		// Terraform版本管理
		tfVersionController := controllers.NewTerraformVersionController(db)

		globalSettings.GET("/terraform-versions", tfVersionController.ListTerraformVersions)

		globalSettings.GET("/terraform-versions/default", tfVersionController.GetDefaultVersion)

		globalSettings.GET("/terraform-versions/:id", tfVersionController.GetTerraformVersion)

		globalSettings.POST("/terraform-versions", tfVersionController.CreateTerraformVersion)

		globalSettings.PUT("/terraform-versions/:id", tfVersionController.UpdateTerraformVersion)

		globalSettings.POST("/terraform-versions/:id/set-default", tfVersionController.SetDefaultVersion)

		globalSettings.DELETE("/terraform-versions/:id", tfVersionController.DeleteTerraformVersion)

		// Provider模板管理
		ptController := controllers.NewProviderTemplateController(db)

		globalSettings.GET("/provider-templates", ptController.ListProviderTemplates)

		globalSettings.GET("/provider-templates/:id", ptController.GetProviderTemplate)

		globalSettings.POST("/provider-templates", ptController.CreateProviderTemplate)

		globalSettings.PUT("/provider-templates/:id", ptController.UpdateProviderTemplate)

		globalSettings.POST("/provider-templates/:id/set-default", ptController.SetDefaultTemplate)

		globalSettings.DELETE("/provider-templates/:id", ptController.DeleteProviderTemplate)

		// AI配置管理
		aiController := controllers.NewAIController(db)

		globalSettings.GET("/ai-configs", aiController.ListConfigs)

		globalSettings.POST("/ai-configs", aiController.CreateConfig)

		globalSettings.GET("/ai-configs/:id", aiController.GetConfig)

		globalSettings.PUT("/ai-configs/:id", aiController.UpdateConfig)

		globalSettings.DELETE("/ai-configs/:id", aiController.DeleteConfig)

		globalSettings.PUT("/ai-configs/priorities", aiController.BatchUpdatePriorities)

		globalSettings.PUT("/ai-configs/:id/set-default", aiController.SetAsDefault)

		// AI 能力开关
		aiFeatureController := controllers.NewAIFeatureController(db)
		globalSettings.GET("/ai-features", aiFeatureController.GetFeatures)
		globalSettings.PUT("/ai-features", aiFeatureController.UpdateFeatures)

		globalSettings.GET("/ai-config/regions", aiController.GetAvailableRegions)

		globalSettings.GET("/ai-config/models", aiController.GetAvailableModels)

		globalSettings.GET("/ai-config/inference-profiles", aiController.GetAvailableInferenceProfiles)

		globalSettings.POST("/ai-config/openai-models", aiController.ListOpenAIModels)

		// 平台配置管理
		platformConfigHandler := handlers.NewPlatformConfigHandler(db)

		globalSettings.GET("/platform-config", platformConfigHandler.GetPlatformConfig)

		globalSettings.PUT("/platform-config", platformConfigHandler.UpdatePlatformConfig)

		// MFA全局配置管理
		mfaHandler := handlers.NewMFAHandler(db)

		globalSettings.GET("/mfa", mfaHandler.GetMFAConfig)

		globalSettings.PUT("/mfa", mfaHandler.UpdateMFAConfig)
	}

	// MFA records are global user-identity data, so use the same platform-admin
	// boundary as the global settings above.
	adminUsers := protected.Group("/admin/users", middleware.RequireSystemAdmin())
	{
		mfaHandler := handlers.NewMFAHandler(db)

		adminUsers.GET("/:user_id/mfa/status", mfaHandler.GetUserMFAStatus)

		adminUsers.POST("/:user_id/mfa/reset", mfaHandler.ResetUserMFA)
	}
}
