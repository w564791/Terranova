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
	schemas := protected.Group("/schemas")
	{
		schemaController := controllers.NewSchemaController(services.NewSchemaService(db))

		schemas.GET("/:id",
			iamMiddleware.RequirePermission("SCHEMAS", "ORGANIZATION", "READ"),
			schemaController.GetSchema,
		)

		schemas.PUT("/:id",
			iamMiddleware.RequirePermission("SCHEMAS", "ORGANIZATION", "WRITE"),
			schemaController.UpdateSchema,
		)
	}
}
