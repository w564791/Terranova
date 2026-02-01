package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupDashboardRoutes sets up dashboard routes
func setupDashboardRoutes(api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	dashboard := api.Group("/dashboard")
	dashboard.Use(middleware.JWTAuth())
	dashboard.Use(middleware.AuditLogger(db))
	{
		dashboardCtrl := controllers.NewDashboardController(db)
		// 需要组织的READ权限才能查看dashboard
		dashboard.GET("/overview",
			iamMiddleware.RequirePermission("ORGANIZATION", "ORGANIZATION", "READ"),
			dashboardCtrl.GetOverviewStats)
		dashboard.GET("/compliance",
			iamMiddleware.RequirePermission("ORGANIZATION", "ORGANIZATION", "READ"),
			dashboardCtrl.GetComplianceStats)
	}

}
