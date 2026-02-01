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

		// List projects with workspace count - READ level (使用 Organization READ 权限)
		projects.GET("", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				wpHandler.ListProjectsWithWorkspaceCount(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				wpHandler.ListProjectsWithWorkspaceCount(c)
			}
		})

		// List workspaces by project - READ level (使用 Organization READ 权限)
		projects.GET("/:id/workspaces", func(c *gin.Context) {
			role, _ := c.Get("role")
			if role == "admin" {
				wpHandler.ListProjectWorkspaces(c)
				return
			}
			iamMiddleware.RequirePermission("WORKSPACES", "ORGANIZATION", "READ")(c)
			if !c.IsAborted() {
				wpHandler.ListProjectWorkspaces(c)
			}
		})
	}
}
