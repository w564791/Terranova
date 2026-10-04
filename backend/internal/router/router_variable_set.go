package router

import (
	"net/http"

	"iac-platform/controllers"
	"iac-platform/internal/middleware"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SetupVariableSetRoutes sets up variable set routes
func SetupVariableSetRoutes(protected *gin.RouterGroup, db *gorm.DB, iamMiddleware *middleware.IAMPermissionMiddleware) {
	vsController := controllers.NewVariableSetController(db)
	vvController := controllers.NewVarsetVariableController(db)
	workspaceService := services.NewWorkspaceService(db)

	// GET /variable-sets?workspace_id=: caller must be able to read the target
	// workspace (WORKSPACE_MANAGEMENT READ @ workspace or WORKSPACES READ @ org)
	// and it must belong to the authenticated org (404 otherwise, no enumeration).
	requireMountTargetReadable := func(c *gin.Context) {
		workspaceID := c.Query("workspace_id")
		if workspaceID == "" {
			c.Next()
			return
		}
		orgID, ok := middleware.AuthOrgID(c)
		if !ok || workspaceService.EnsureWorkspaceInOrg(workspaceID, orgID) != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "Workspace not found"})
			c.Abort()
			return
		}
		if !iamMiddleware.CheckWorkspaceOrOrgWorkspacesRead(c, workspaceID) {
			c.Abort() // 403 already written
			return
		}
		c.Next()
	}

	varsets := protected.Group("/variable-sets")
	{
		// Variable Set CRUD
		varsets.GET("",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			requireMountTargetReadable,
			vsController.List,
		)
		varsets.GET("/:varset_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vsController.Get,
		)
		varsets.POST("",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vsController.Create,
		)
		varsets.PUT("/:varset_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vsController.Update,
		)
		varsets.PUT("/:varset_id/scope",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.UpdateScope,
		)
		varsets.DELETE("/:varset_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.Delete,
		)

		// Variables
		varsets.GET("/:varset_id/variables",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vvController.List,
		)
		varsets.GET("/:varset_id/variables/:var_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vvController.Get,
		)
		varsets.POST("/:varset_id/variables",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vvController.Create,
		)
		varsets.PUT("/:varset_id/variables/:var_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vvController.Update,
		)
		varsets.DELETE("/:varset_id/variables/:var_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vvController.Delete,
		)

		// Assignments
		varsets.GET("/:varset_id/assignments",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vsController.ListAssignments,
		)
		varsets.POST("/:varset_id/assignments",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.CreateAssignment,
		)
		varsets.DELETE("/:varset_id/assignments/:assignment_id",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.DeleteAssignment,
		)
	}
}
