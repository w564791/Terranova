package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// loadApplicationOrgID 查 applications.org_id（path 可为数字 id 或 app_key）
func loadApplicationOrgID(ctx context.Context, db *gorm.DB, rawID string) (orgID uint, appKey string, err error) {
	appKey, err = resolveApplicationPrincipalID(ctx, db, rawID)
	if err != nil {
		return 0, "", err
	}
	err = db.WithContext(ctx).Table("applications").
		Select("org_id").Where("app_key = ?", appKey).Scan(&orgID).Error
	if err != nil || orgID == 0 {
		return 0, "", gorm.ErrRecordNotFound
	}
	return orgID, appKey, nil
}

// AssignApplicationRole assigns a role to an Application (app_key principal)
// @Summary Assign role to application
// @Description Assign a role to an application within a specific scope (principal stored as app_key)
// @Tags IAM-Application
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID or app_key"
// @Param request body AssignRoleRequest true "Assignment information"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id}/roles [post]
func (h *RoleHandler) AssignApplicationRole(c *gin.Context) {
	rawID := c.Param("id")

	var req AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Invalid request", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	appOrg, appKey, err := loadApplicationOrgID(c.Request.Context(), h.db, rawID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Application not found", "timestamp": time.Now(),
		})
		return
	}

	var role entity.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404, "message": "Role not found", "timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to get role", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Invalid scope type", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}
	// Application principal 只能在组织级被赋予 Role。这个约束必须在写入
	// iam_application_roles 前执行，避免历史/手工数据在求值时扩大到项目或工作空间。
	if !valueobject.PrincipalTypeApplication.CanBeGrantedAt(scopeType) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Application roles can only be assigned at organization scope", "timestamp": time.Now(),
		})
		return
	}

	assignedByStr, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401, "message": "User ID not found in context", "timestamp": time.Now(),
		})
		return
	}

	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
		return
	}
	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}
	if appOrg != authOrg {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Application not found", "timestamp": time.Now(),
		})
		return
	}
	// 自定义 Role 必须属于当前应用所在组织；系统 Role 保持全局可见。否则
	// 已知 role id 可被用来把其他租户的策略挂到当前应用。
	if !h.ensureRoleVisible(c, &role) {
		return
	}
	if err := h.guard.EnsureAssignmentScopeInAuthOrg(c.Request.Context(), scopeType, req.ScopeID, authOrg); err != nil {
		if respondAntiEscalation(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Scope check failed", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}
	if err := h.guard.EnsureCanAssignRole(c.Request.Context(), assignedByStr, isSystemAdmin, req.RoleID, scopeType, req.ScopeID); err != nil {
		if respondAntiEscalation(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation check failed", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	var existing entity.ApplicationRole
	err = h.db.Where(
		"application_principal_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
		appKey, req.RoleID, req.ScopeType, req.ScopeID,
	).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"code": 409, "message": "Role already assigned to application in this scope", "data": existing, "timestamp": time.Now(),
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to check existing role assignment", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	ar := &entity.ApplicationRole{
		ApplicationPrincipalID: appKey,
		RoleID:                 req.RoleID,
		ScopeType:              req.ScopeType,
		ScopeID:                req.ScopeID,
		AssignedBy:             &assignedByStr,
		AssignedAt:             time.Now(),
		Reason:                 req.Reason,
	}
	if req.ExpiresAt != "" {
		expiresAt, err := parseFlexibleTime(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400, "message": "Invalid expires_at format", "error": err.Error(), "timestamp": time.Now(),
			})
			return
		}
		ar.ExpiresAt = &expiresAt
	}

	if err := h.db.Create(ar).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to assign role to application", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}
	ar.RoleName = role.Name
	ar.RoleDisplayName = role.DisplayName

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "message": "Role assigned to application successfully", "data": ar, "timestamp": time.Now(),
	})
}

// ListApplicationRoles lists roles for an Application
// @Summary List application roles
// @Description Get all role assignments for an application (by id or app_key)
// @Tags IAM-Application
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID or app_key"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id}/roles [get]
func (h *RoleHandler) ListApplicationRoles(c *gin.Context) {
	rawID := c.Param("id")

	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}
	appOrg, appKey, err := loadApplicationOrgID(c.Request.Context(), h.db, rawID)
	if err != nil || appOrg != authOrg {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Application not found", "timestamp": time.Now(),
		})
		return
	}

	var roles []*entity.ApplicationRole
	if err := h.db.Where("application_principal_id = ?", appKey).
		Order("assigned_at DESC").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to list application roles", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	filtered := make([]*entity.ApplicationRole, 0, len(roles))
	for _, ar := range roles {
		var role entity.Role
		if err := h.db.First(&role, ar.RoleID).Error; err != nil || !role.IsActive {
			continue
		}
		ar.RoleName = role.Name
		ar.RoleDisplayName = role.DisplayName
		st, err := valueobject.ParseScopeType(ar.ScopeType)
		if err != nil {
			continue
		}
		// 忽略迁移前或手工写入的非法 Application 非组织级角色。
		if !valueobject.PrincipalTypeApplication.CanBeGrantedAt(st) {
			continue
		}
		if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, st, ar.ScopeID, authOrg); err != nil {
			continue
		}
		if !ar.IsValid() {
			continue
		}
		filtered = append(filtered, ar)
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200, "data": filtered, "total": len(filtered), "timestamp": time.Now(),
	})
}

// RevokeApplicationRole revokes an Application role assignment
// @Summary Revoke application role
// @Description Revoke a role assignment from an application
// @Tags IAM-Application
// @Produce json
// @Security BearerAuth
// @Param id path string true "Application ID or app_key"
// @Param assignment_id path int true "Role assignment ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id}/roles/{assignment_id} [delete]
func (h *RoleHandler) RevokeApplicationRole(c *gin.Context) {
	rawID := c.Param("id")

	assignmentIDStr := c.Param("assignment_id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Invalid assignment ID", "timestamp": time.Now(),
		})
		return
	}

	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}
	appOrg, appKey, err := loadApplicationOrgID(c.Request.Context(), h.db, rawID)
	if err != nil || appOrg != authOrg {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}

	var ar entity.ApplicationRole
	if err := h.db.Where("id = ? AND application_principal_id = ?", assignmentID, appKey).
		First(&ar).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}
	st, err := valueobject.ParseScopeType(ar.ScopeType)
	if err != nil || ensureScopeInAuthOrg(c.Request.Context(), h.db, st, ar.ScopeID, authOrg) != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}

	if err := h.db.Delete(&ar).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to revoke application role", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}
	c.Status(http.StatusNoContent)
}
