package router

import (
	"iac-platform/controllers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupSchemaRoutes sets up schema routes
func setupSchemaRoutes(protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	// TODO: 实现schema路由
	// 参考原router.go中的 schemas := protected.Group("/schemas") 部分
	schemas := protected.Group("/schemas")
	{
		schemaController := controllers.NewSchemaController(services.NewSchemaService(db))

		schemas.GET("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaController.GetSchema(c)
				return
			}
			iamMiddleware.RequirePermission("SCHEMAS", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				schemaController.GetSchema(c)
			}
		})

		schemas.PUT("/:id", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				schemaController.UpdateSchema(c)
				return
			}
			iamMiddleware.RequirePermission("SCHEMAS", "ORGANIZATION", "WRITE")(c)
			if !c.IsAborted() {
				schemaController.UpdateSchema(c)
			}
		})
	}
}
