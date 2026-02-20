package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// setupProjectRoutes sets up project routes
func setupProjectRoutes(api *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	projects := api.Group("/projects")
	projects.Use(middleware.JWTAuth())
	projects.Use(middleware.AuditLogger(db))
	{
		wpHandler := handlers.NewWorkspaceProjectHandler(db)

		// List projects with workspace count - READ level
		projects.GET("",
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ"),
			wpHandler.ListProjectsWithWorkspaceCount,
		)

		// List workspaces by project - READ level
		projects.GET("/:id/workspaces",
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ"),
			wpHandler.ListProjectWorkspaces,
		)
	}
}
