package router

import (
	"iac-platform/internal/handlers"
	"iac-platform/internal/middleware"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RegisterSandboxRoutes registers org-scoped sandbox registration APIs (§10.2).
// Paths follow existing /organizations/:org_id/… (spec shorthand "orgs").
// Permission: WORKSPACE_RESOURCES WRITE on the target workspace (checked in
// handler); org gate uses WORKSPACE_MANAGEMENT READ so auth_org_id is set.
func RegisterSandboxRoutes(protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	h := handlers.NewSandboxHandler(db, iamMiddleware)
	orgRead := iamMiddleware.RequirePermission("WORKSPACE_MANAGEMENT", "ORGANIZATION", "READ")

	g := protected.Group("/organizations/:org_id/sandboxes")
	{
		g.POST("", orgRead, h.CreateSandbox)
		g.GET("", orgRead, h.ListSandboxes)
		// Static segment before /:id so Gin does not capture "shell-sessions".
		g.DELETE("/shell-sessions/:session_id", orgRead, h.DeleteShellSession)
		g.POST("/shell-sessions/:session_id/plans", orgRead, h.CreateShellPlan)
		g.GET("/shell-sessions/:session_id/plans", orgRead, h.ListShellPlans)
		g.POST("/:id/shell-sessions", orgRead, h.CreateShellSession)
		g.GET("/:id", orgRead, h.GetSandbox)
		g.DELETE("/:id", orgRead, h.DeleteSandbox)
	}
}
