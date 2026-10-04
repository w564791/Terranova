package router

import (
	"net/http"

	"iac-platform/controllers"
	"iac-platform/internal/handlers"
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

	// Assignment target (create / delete assignment): the workspace or project
	// must be in the auth org (404 otherwise, same response as a missing one)
	// and the caller must hold WORKSPACE_VARIABLES WRITE on it - on the
	// workspace, or at project scope (project/org grants) for a project target,
	// since a project assignment reaches every workspace of the project.
	vsController.AuthorizeAssignmentTarget = func(c *gin.Context, scopeType string, projectID *int, workspaceID *string) bool {
		orgID, ok := middleware.AuthOrgID(c)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "assignment target not found"})
			return false
		}
		switch {
		case scopeType == "workspace" && workspaceID != nil:
			if workspaceService.EnsureWorkspaceInOrg(*workspaceID, orgID) != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
				return false
			}
			return iamMiddleware.RequireWorkspaceResourcePermission(c, *workspaceID, "WORKSPACE_VARIABLES", "WRITE")
		case scopeType == "project" && projectID != nil:
			if *projectID <= 0 || handlers.EnsureProjectInAuthOrg(c.Request.Context(), db, uint(*projectID), orgID) != nil {
				c.JSON(http.StatusNotFound, gin.H{"error": "project not found"})
				return false
			}
			return iamMiddleware.RequireProjectResourcePermission(c, uint(*projectID), "WORKSPACE_VARIABLES", "WRITE")
		default:
			return true // malformed target: the service rejects it with 400
		}
	}

	// Every /:varset_id route: RequirePermission, then the org binding
	// (404 not visible in the auth org; 403 on write routes when read-only).
	read := manifestRouteChain(vsController.VarsetInAuthOrg(false))
	write := manifestRouteChain(vsController.VarsetInAuthOrg(true))

	varsets := protected.Group("/variable-sets")
	{
		// Variable Set CRUD
		varsets.GET("",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			requireMountTargetReadable,
			vsController.List,
		)
		varsets.GET("/:varset_id", read(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vsController.Get,
		)...)
		varsets.POST("",
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vsController.Create,
		)
		varsets.PUT("/:varset_id", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vsController.Update,
		)...)
		varsets.PUT("/:varset_id/scope", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.UpdateScope,
		)...)
		varsets.DELETE("/:varset_id", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.Delete,
		)...)

		// Variables
		varsets.GET("/:varset_id/variables", read(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vvController.List,
		)...)
		varsets.GET("/:varset_id/variables/:var_id", read(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vvController.Get,
		)...)
		varsets.POST("/:varset_id/variables", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vvController.Create,
		)...)
		varsets.PUT("/:varset_id/variables/:var_id", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "WRITE"),
			vvController.Update,
		)...)
		varsets.DELETE("/:varset_id/variables/:var_id", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vvController.Delete,
		)...)

		// Assignments
		varsets.GET("/:varset_id/assignments", read(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "READ"),
			vsController.ListAssignments,
		)...)
		varsets.POST("/:varset_id/assignments", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.CreateAssignment,
		)...)
		varsets.DELETE("/:varset_id/assignments/:assignment_id", write(
			iamMiddleware.RequirePermission("VARIABLE_SETS", "ORGANIZATION", "ADMIN"),
			vsController.DeleteAssignment,
		)...)
	}
}
