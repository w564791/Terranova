package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupDemoRoutes sets up demo routes
func setupDemoRoutes(protected *gin.RouterGroup, api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现demo路由
	// 参考原router.go中的 demos := protected.Group("/demos") 和 api.GET("/demo-versions/:versionId") 部分
	// Demo管理（独立路由组）- 添加IAM权限检查
	demos := protected.Group("/demos")
	{
		demoController := controllers.NewModuleDemoController(db)

		demos.GET("/:id",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ"),
			demoController.GetDemoByID,
		)

		demos.PUT("/:id",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "WRITE"),
			demoController.UpdateDemo,
		)

		demos.DELETE("/:id",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "ADMIN"),
			demoController.DeleteDemo,
		)

		// 版本管理
		demos.GET("/:id/versions",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ"),
			demoController.GetVersionsByDemoID,
		)

		demos.GET("/:id/compare",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ"),
			demoController.CompareVersions,
		)

		demos.POST("/:id/rollback",
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "WRITE"),
			demoController.RollbackToVersion,
		)
	}

	// Demo版本详情 - 添加IAM权限检查
	demoController := controllers.NewModuleDemoController(db)
	api.GET("/demo-versions/:versionId",
		middleware.JWTAuth(),
		iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ"),
		demoController.GetVersionByID,
	)
}
