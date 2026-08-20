package controllers

import (
	"fmt"
	"net/http"
	"strconv"

	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// loadAndAuthorizeTaskWorkspace is the mandatory guard for legacy global
// /tasks/:task_id/* endpoints.  A task ID is globally enumerable, so neither
// an organization-level permission nor a task lookup by itself is sufficient:
// resolve task -> workspace -> project -> organization first, then check IAM
// on that exact workspace.
//
// If the request has selected an organization (query org_id) or an upstream
// auth_org_id context value, it must match the task's owning organization.
// Requests without an organization keep backward compatibility, but still use
// the task-derived workspace for a fail-closed IAM check.
func loadAndAuthorizeTaskWorkspace(
	ctx *gin.Context,
	db *gorm.DB,
	iam *middleware.IAMPermissionMiddleware,
) (*models.WorkspaceTask, bool) {
	if db == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "task access is not configured"})
		return nil, false
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil || taskID == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return nil, false
	}

	var task models.WorkspaceTask
	if err := db.WithContext(ctx.Request.Context()).First(&task, uint(taskID)).Error; err != nil {
		// Do not reveal whether an arbitrary global task ID exists.
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, false
	}
	if task.WorkspaceID == "" {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, false
	}

	orgID, err := taskWorkspaceOrgID(ctx, db, task.WorkspaceID)
	if err != nil {
		// A task without one unambiguous workspace/project/org ownership must
		// never be exposed through a global ID route.
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, false
	}

	if requestedOrgID, present, err := requestedTaskOrgID(ctx); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
		return nil, false
	} else if present && requestedOrgID != orgID {
		// Match workspace routes: hide cross-organization resources rather than
		// confirming that a guessed task ID exists.
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return nil, false
	}

	if iam == nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "IAM not configured"})
		return nil, false
	}
	if !iam.RequireWorkspacePermission(ctx, task.WorkspaceID, "READ") {
		return nil, false
	}

	// Available to handlers/audit code without making an absent selected org
	// look like a caller-supplied auth_org_id.
	ctx.Set("task_org_id", orgID)
	ctx.Set("task_workspace_id", task.WorkspaceID)
	return &task, true
}

// taskWorkspaceOrgID follows the same ownership chain used by workspace
// routes. More than one relation is corrupt even when the projects happen to
// share an org, so it fails closed instead of choosing an arbitrary project.
func taskWorkspaceOrgID(ctx *gin.Context, db *gorm.DB, workspaceID string) (uint, error) {
	if db == nil || workspaceID == "" {
		return 0, fmt.Errorf("task workspace binding unavailable")
	}

	type workspaceBinding struct {
		ProjectID uint `gorm:"column:project_id"`
		OrgID     uint `gorm:"column:org_id"`
	}
	var bindings []workspaceBinding
	err := db.WithContext(ctx.Request.Context()).
		Table("workspaces AS w").
		Select("wpr.project_id, p.org_id").
		Joins("JOIN workspace_project_relations AS wpr ON wpr.workspace_id = w.workspace_id").
		Joins("JOIN projects AS p ON p.id = wpr.project_id").
		Where("w.workspace_id = ?", workspaceID).
		Scan(&bindings).Error
	if err != nil || len(bindings) != 1 || bindings[0].ProjectID == 0 || bindings[0].OrgID == 0 {
		return 0, fmt.Errorf("workspace %q does not have one organization binding", workspaceID)
	}
	return bindings[0].OrgID, nil
}

// requestedTaskOrgID returns the two optional organization selectors used by
// global routes. Both are checked when present, so an inconsistent context and
// query cannot be used to switch tenants.
func requestedTaskOrgID(ctx *gin.Context) (uint, bool, error) {
	var selected uint
	present := false

	if raw, ok := ctx.Get("auth_org_id"); ok {
		orgID, err := parseTaskOrgID(raw)
		if err != nil {
			return 0, false, err
		}
		selected = orgID
		present = true
	}

	if raw := ctx.Query("org_id"); raw != "" {
		orgID64, err := strconv.ParseUint(raw, 10, 32)
		if err != nil || orgID64 == 0 {
			return 0, false, fmt.Errorf("invalid org_id")
		}
		orgID := uint(orgID64)
		if present && selected != orgID {
			return 0, false, fmt.Errorf("conflicting organization selectors")
		}
		selected = orgID
		present = true
	}

	return selected, present, nil
}

func parseTaskOrgID(raw interface{}) (uint, error) {
	var orgID uint64
	switch v := raw.(type) {
	case uint:
		orgID = uint64(v)
	case uint64:
		orgID = v
	case uint32:
		orgID = uint64(v)
	case int:
		if v > 0 {
			orgID = uint64(v)
		}
	case int64:
		if v > 0 {
			orgID = uint64(v)
		}
	case int32:
		if v > 0 {
			orgID = uint64(v)
		}
	case float64:
		if v > 0 && v == float64(uint64(v)) {
			orgID = uint64(v)
		}
	case string:
		parsed, err := strconv.ParseUint(v, 10, 32)
		if err != nil {
			return 0, err
		}
		orgID = parsed
	default:
		return 0, fmt.Errorf("invalid auth_org_id type %T", raw)
	}
	if orgID == 0 || orgID > uint64(^uint(0)) {
		return 0, fmt.Errorf("invalid auth_org_id")
	}
	return uint(orgID), nil
}
