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

		demos.GET("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.GetDemoByID(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				demoController.GetDemoByID(c)
			}
		})

		demos.PUT("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.UpdateDemo(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				demoController.UpdateDemo(c)
			}
		})

		demos.DELETE("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.DeleteDemo(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "ADMIN")(c)
			if !c.IsAborted() {
				demoController.DeleteDemo(c)
			}
		})

		// 版本管理
		demos.GET("/:id/versions", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.GetVersionsByDemoID(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				demoController.GetVersionsByDemoID(c)
			}
		})

		demos.GET("/:id/compare", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.CompareVersions(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				demoController.CompareVersions(c)
			}
		})

		demos.POST("/:id/rollback", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				demoController.RollbackToVersion(c)
				return
			}
			iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				demoController.RollbackToVersion(c)
			}
		})
	}

	// Demo版本详情 - 添加IAM权限检查
	api.GET("/demo-versions/:versionId", middleware.JWTAuth(), func(c *gin.Context) {
		demoController := controllers.NewModuleDemoController(db)
		role, _ := c.Get("role")
		if role == "admin" {
			demoController.GetVersionByID(c)
			return
		}
		iamMiddleware.RequirePermission("MODULE_DEMOS", "ORGANIZATION", "READ")(c)
		if !c.IsAborted() {
			demoController.GetVersionByID(c)
		}
	})
}
