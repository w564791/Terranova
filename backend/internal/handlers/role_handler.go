package handlers

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// RoleHandler IAM角色处理器
type RoleHandler struct {
	db    *gorm.DB
	guard *service.RoleAntiEscalationService
}

// NewRoleHandler 创建角色处理器
// checker 用于防提权；可为 nil（仅测试旁路，生产必须注入）
func NewRoleHandler(db *gorm.DB, checker service.PermissionChecker) *RoleHandler {
	var guard *service.RoleAntiEscalationService
	if checker != nil {
		guard = service.NewRoleAntiEscalationService(db, checker)
	}
	return &RoleHandler{db: db, guard: guard}
}

// actorFromContext 读取当前操作者与平台超管标志
func actorFromContext(c *gin.Context) (userID string, isSystemAdmin bool, ok bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		return "", false, false
	}
	userID, _ = raw.(string)
	if userID == "" {
		return "", false, false
	}
	if v, exists := c.Get("is_system_admin"); exists {
		isSystemAdmin, _ = v.(bool)
	}
	return userID, isSystemAdmin, true
}

// respondAntiEscalation 将防提权错误映射为 HTTP
func respondAntiEscalation(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, service.ErrRoleNotFound) || errors.Is(err, service.ErrPermissionDefNotFound) {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": err.Error(), "timestamp": time.Now(),
		})
		return true
	}
	if errors.Is(err, service.ErrAntiEscalationMisconfigured) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "error": err.Error(), "timestamp": time.Now(),
		})
		return true
	}
	if service.IsPrivilegeEscalationError(err) {
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Privilege escalation denied",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return true
	}
	return false
}

func authOrgFromContext(c *gin.Context) uint {
	if raw, ok := c.Get("auth_org_id"); ok {
		switch v := raw.(type) {
		case uint:
			if v > 0 {
				return v
			}
		case int:
			if v > 0 {
				return uint(v)
			}
		case float64:
			if v > 0 {
				return uint(v)
			}
		}
	}
	if raw, ok := c.Get("org_id"); ok {
		switch v := raw.(type) {
		case uint:
			if v > 0 {
				return v
			}
		case int:
			if v > 0 {
				return uint(v)
			}
		case float64:
			if v > 0 {
				return uint(v)
			}
		}
	}
	// 仅单租户默认 org=1；多租户缺省 0，由 guard 侧按 scope 拒绝
	if middlewareIsSingleTenant() {
		return 1
	}
	return 0
}

// middlewareIsSingleTenant 与 iam_permission.isSingleTenantIAM 语义对齐（handlers 包内避免循环依赖）
func middlewareIsSingleTenant() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("IAM_SINGLE_TENANT")))
	return v == "1" || v == "true" || v == "yes"
}

// ListRolesResponse 角色列表响应
type ListRolesResponse struct {
	Roles []*RoleWithPolicyCount `json:"roles"`
	Total int64                  `json:"total"`
}

// RoleWithPolicyCount 带策略数量的角色
type RoleWithPolicyCount struct {
	entity.Role
	PolicyCount int64 `json:"policy_count"`
}

// RoleDetailResponse 角色详情响应
type RoleDetailResponse struct {
	entity.Role
	Policies []*entity.RolePolicy `json:"policies"`
}

// CreateRoleRequest 创建角色请求
type CreateRoleRequest struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
}

// UpdateRoleRequest 更新角色请求
type UpdateRoleRequest struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	IsActive    *bool  `json:"is_active"`
}

// AssignRoleRequest 分配角色请求
type AssignRoleRequest struct {
	RoleID    uint   `json:"role_id" binding:"required"`
	ScopeType string `json:"scope_type" binding:"required"`
	ScopeID   uint   `json:"scope_id" binding:"required"`
	ExpiresAt string `json:"expires_at"`
	Reason    string `json:"reason"`
}

// AddRolePolicyRequest 添加角色策略请求
type AddRolePolicyRequest struct {
	PermissionID    string `json:"permission_id" binding:"required"` // 业务语义ID
	PermissionLevel string `json:"permission_level" binding:"required"`
	ScopeType       string `json:"scope_type" binding:"required"`
}

// ListRoles lists all roles
// @Summary List all roles
// @Description Get all IAM roles including system predefined and custom roles
// @Tags IAM-Role
// @Produce json
// @Security BearerAuth
// @Param is_active query bool false "Filter by active status"
// @Success 200 {object} ListRolesResponse
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles [get]
func (h *RoleHandler) ListRoles(c *gin.Context) {
	isActiveStr := c.Query("is_active")
	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}

	// 只有明确标记为系统的 Role 才可跨租户可见。历史遗留的 custom
	// org_id=0 行会被迁移隔离，不能因 org_id 的哨兵值而成为平台 Role。
	query := h.db.Model(&entity.Role{}).
		Where("is_system = ? OR (is_system = ? AND org_id = ?)", true, false, authOrg)

	if isActiveStr != "" {
		isActive, _ := strconv.ParseBool(isActiveStr)
		query = query.Where("is_active = ?", isActive)
	}

	var roles []*entity.Role
	if err := query.Order("is_system DESC, id ASC").Find(&roles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to list roles",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 获取每个角色的策略数量
	rolesWithCount := make([]*RoleWithPolicyCount, len(roles))
	for i, role := range roles {
		var count int64
		h.db.Model(&entity.RolePolicy{}).Where("role_id = ?", role.ID).Count(&count)
		rolesWithCount[i] = &RoleWithPolicyCount{
			Role:        *role,
			PolicyCount: count,
		}
	}

	c.JSON(http.StatusOK, ListRolesResponse{
		Roles: rolesWithCount,
		Total: int64(len(roles)),
	})
}

// GetRole gets role details
// @Summary Get role details
// @Description Get detailed information for a specific role including all permission policies
// @Tags IAM-Role
// @Produce json
// @Security BearerAuth
// @Param id path int true "Role ID"
// @Success 200 {object} RoleDetailResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id} [get]
// ensureRoleVisible 系统 Role 全局可见；自定义 Role 仅所属 org 可见
func (h *RoleHandler) ensureRoleVisible(c *gin.Context, role *entity.Role) bool {
	if role.IsSystem {
		return true
	}
	authOrg := authOrgFromContext(c)
	if authOrg == 0 || role.OrgID != authOrg {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role not found", "timestamp": time.Now(),
		})
		return false
	}
	return true
}

func (h *RoleHandler) GetRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	var role entity.Role
	if err := h.db.First(&role, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 获取角色的所有策略
	var policies []*entity.RolePolicy
	if err := h.db.Where("role_id = ?", uint(id)).Find(&policies).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to load role policies",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 手动加载权限定义信息
	for _, policy := range policies {
		var permDef entity.PermissionDefinition
		if err := h.db.Where("id = ?", policy.PermissionID).First(&permDef).Error; err == nil {
			policy.PermissionName = permDef.Name
			policy.PermissionDisplayName = permDef.DisplayName
			policy.ResourceType = string(permDef.ResourceType)
		}
	}

	c.JSON(http.StatusOK, RoleDetailResponse{
		Role:     role,
		Policies: policies,
	})
}

// CreateRole creates a custom role
// @Summary Create custom role
// @Description Create a new custom role (system roles cannot be created via API)
// @Tags IAM-Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateRoleRequest true "Role information"
// @Success 201 {object} entity.Role
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles [post]
func (h *RoleHandler) CreateRole(c *gin.Context) {
	var req CreateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	userIDInterface, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
		})
		return
	}

	userID, ok := userIDInterface.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Invalid user ID type",
			"timestamp": time.Now(),
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

	role := &entity.Role{
		OrgID:       authOrg, // 自定义 Role 租户化
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsSystem:    false, // 通过API创建的都是自定义角色
		IsActive:    true,
		CreatedBy:   &userID,
	}

	if err := h.db.Create(role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to create role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusCreated, role)
}

// UpdateRole updates a role
// @Summary Update role
// @Description Update role information (some fields of system roles cannot be modified)
// @Tags IAM-Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Role ID"
// @Param request body UpdateRoleRequest true "Role information"
// @Success 200 {object} entity.Role
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id} [put]
func (h *RoleHandler) UpdateRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	var req UpdateRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	var role entity.Role
	if err := h.db.First(&role, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 系统/平台 Role：仅平台超管可改元数据
	if role.IsSystem || role.OrgID == 0 {
		if _, isSys, ok := actorFromContext(c); !ok || !isSys {
			c.JSON(http.StatusForbidden, gin.H{
				"code": 403, "message": "System roles can only be modified by system admin", "timestamp": time.Now(),
			})
			return
		}
	}

	// 系统角色不能修改名称
	if role.IsSystem && req.DisplayName != "" {
		role.DisplayName = req.DisplayName
	} else if !role.IsSystem && req.DisplayName != "" {
		role.DisplayName = req.DisplayName
	}

	if req.Description != "" {
		role.Description = req.Description
	}

	if req.IsActive != nil {
		role.IsActive = *req.IsActive
	}

	role.UpdatedAt = time.Now()

	if err := h.db.Save(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to update role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusOK, role)
}

// DeleteRole deletes a role
// @Summary Delete role
// @Description Delete a custom role (system roles cannot be deleted)
// @Tags IAM-Role
// @Produce json
// @Security BearerAuth
// @Param id path int true "Role ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id} [delete]
func (h *RoleHandler) DeleteRole(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	var role entity.Role
	if err := h.db.First(&role, uint(id)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 系统角色不能删除；平台级自定义（org_id=0 且非 system）亦禁止租户删除
	if role.IsSystem || role.OrgID == 0 {
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Cannot delete system role",
			"timestamp": time.Now(),
		})
		return
	}

	if err := h.db.Delete(&role).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to delete role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// AssignRole assigns a role to a user
// @Summary Assign role to user
// @Description Assign a role to a user within a specific scope
// @Tags IAM-User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID (semantic ID)"
// @Param request body AssignRoleRequest true "Assignment information"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/roles [post]
func (h *RoleHandler) AssignRole(c *gin.Context) {
	userID := c.Param("id")

	var req AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 验证角色存在
	var role entity.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 验证作用域类型
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	assignedByStr, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
		})
		return
	}

	// 防提权：scope ∈ 鉴权 org + policy 闭包 ⊆ actor
	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
		return
	}
	authOrg := authOrgFromContext(c)
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

	// 检查是否已存在相同的角色分配
	var existingRole entity.UserRole
	err = h.db.Where("user_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
		userID, req.RoleID, req.ScopeType, req.ScopeID).First(&existingRole).Error

	if err == nil {
		// 已存在相同的角色分配
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   "Role already assigned to user in this scope",
			"data":      existingRole,
			"timestamp": time.Now(),
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		// 查询出错
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check existing role assignment",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	userRole := &entity.UserRole{
		UserID:     userID,
		RoleID:     req.RoleID,
		ScopeType:  req.ScopeType, // 直接使用字符串
		ScopeID:    req.ScopeID,
		AssignedBy: &assignedByStr,
		AssignedAt: time.Now(),
		Reason:     req.Reason,
	}

	// 解析过期时间（兼容 RFC3339 与 datetime-local）
	if req.ExpiresAt != "" {
		expiresAt, err := parseFlexibleTime(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Invalid expires_at format",
				"error":     err.Error(),
				"timestamp": time.Now(),
			})
			return
		}
		userRole.ExpiresAt = &expiresAt
	}

	// Role 赋予本身即表示目标用户进入当前鉴权组织。成员关系和角色分配必须
	// 同事务写入，否则用户可能已经拥有权限却无法从 accessible-org 进入该组织。
	tx := h.db.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to start role assignment transaction", "error": tx.Error.Error(), "timestamp": time.Now(),
		})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	var membership entity.UserOrganization
	membershipErr := tx.Where("user_id = ? AND org_id = ?", userID, authOrg).First(&membership).Error
	if membershipErr != nil && membershipErr != gorm.ErrRecordNotFound {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to check organization membership", "error": membershipErr.Error(), "timestamp": time.Now(),
		})
		return
	}
	if membershipErr == gorm.ErrRecordNotFound {
		membership = entity.UserOrganization{UserID: userID, OrgID: authOrg, JoinedAt: time.Now()}
		// PostgreSQL/SQLite 均支持无 target 的 ON CONFLICT DO NOTHING；生产库的
		// (user_id, org_id) 唯一约束保证并发赋权时不会生成重复 membership。
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&membership).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500, "message": "Failed to ensure organization membership", "error": err.Error(), "timestamp": time.Now(),
			})
			return
		}
	}

	if err := tx.Create(userRole).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to assign role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to commit role assignment", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	// 加载角色名称
	userRole.RoleName = role.Name
	userRole.RoleDisplayName = role.DisplayName

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Role assigned successfully",
		"data":      userRole,
		"timestamp": time.Now(),
	})
}

// RevokeRole revokes a user's role
// @Summary Revoke user role
// @Description Revoke a user's role assignment in a specific scope
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID (semantic ID)"
// @Param assignment_id path int true "Role assignment ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/roles/{assignment_id} [delete]
func (h *RoleHandler) RevokeRole(c *gin.Context) {
	userID := c.Param("id")

	assignmentIDStr := c.Param("assignment_id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid assignment ID",
			"timestamp": time.Now(),
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

	// 验证角色分配存在且属于该用户
	var userRole entity.UserRole
	if err := h.db.Where("id = ? AND user_id = ?", assignmentID, userID).First(&userRole).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role assignment not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role assignment",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// assignment scope 必须落在鉴权 org
	st, err := valueobject.ParseScopeType(userRole.ScopeType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "invalid assignment scope", "timestamp": time.Now(),
		})
		return
	}
	if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, st, userRole.ScopeID, authOrg); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}

	if err := h.db.Delete(&userRole).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to revoke role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// ListUserRoles lists all roles for a user
// @Summary List user roles
// @Description Get all role assignments for a user across all scopes
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID (semantic ID)"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/roles [get]
func (h *RoleHandler) ListUserRoles(c *gin.Context) {
	userID := c.Param("id")

	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}

	// 先查询用户角色分配
	var userRoles []*entity.UserRole
	if err := h.db.Where("user_id = ?", userID).Order("assigned_at DESC").Find(&userRoles).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to list user roles",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 手动加载角色信息
	for _, userRole := range userRoles {
		var role entity.Role
		if err := h.db.First(&role, userRole.RoleID).Error; err == nil {
			if role.IsActive {
				userRole.RoleName = role.Name
				userRole.RoleDisplayName = role.DisplayName
			}
		}
	}

	// 过滤：激活角色 + assignment scope 落在鉴权 org
	activeUserRoles := make([]*entity.UserRole, 0)
	for _, userRole := range userRoles {
		if userRole.RoleName == "" {
			continue
		}
		st, err := valueobject.ParseScopeType(userRole.ScopeType)
		if err != nil {
			continue
		}
		if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, st, userRole.ScopeID, authOrg); err != nil {
			continue
		}
		activeUserRoles = append(activeUserRoles, userRole)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      activeUserRoles,
		"total":     len(activeUserRoles),
		"timestamp": time.Now(),
	})
}

// AddRolePolicy adds a permission policy to a role
// @Summary Add role policy
// @Description Add a permission policy to a specific role
// @Tags IAM-Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Role ID"
// @Param request body AddRolePolicyRequest true "Policy information"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id}/policies [post]
func (h *RoleHandler) AddRolePolicy(c *gin.Context) {
	roleIDStr := c.Param("id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	var req AddRolePolicyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 验证角色存在
	var role entity.Role
	if err := h.db.First(&role, roleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 验证权限定义存在
	var permDef entity.PermissionDefinition
	if err := h.db.Where("id = ?", req.PermissionID).First(&permDef).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Permission definition not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get permission definition",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 验证权限级别
	_, err = valueobject.ParsePermissionLevel(req.PermissionLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid permission level",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 策略 scope 表示 Role 可在哪个 assignment 层生效。它可位于资源固有
	// scope 或其父层（例如组织级 Role 具备 workspace execution），但绝不能
	// 反向把组织资源挂到 project/workspace Role 上。
	policyScope, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !permDef.ScopeLevel.IsValid() || permDef.ResourceType.GetScopeLevel() != permDef.ScopeLevel {
		c.JSON(http.StatusConflict, gin.H{
			"code": 409, "message": "Permission definition has invalid scope level", "timestamp": time.Now(),
		})
		return
	}
	if !policyScope.CanHostPolicyFor(permDef.ScopeLevel) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "Policy scope type must be the permission scope level or an ancestor", "timestamp": time.Now(),
		})
		return
	}

	// 防提权：系统 Role 只读 + 不可写入超出自身有效权限的策略
	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
		return
	}
	actorID, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401, "message": "User ID not found in context", "timestamp": time.Now(),
		})
		return
	}
	orgID := authOrgFromContext(c)
	if err := h.guard.EnsureCanAddRolePolicy(c.Request.Context(), actorID, isSystemAdmin, uint(roleID),
		req.PermissionID, req.PermissionLevel, policyScope, valueobject.ScopeTypeOrganization, orgID); err != nil {
		if respondAntiEscalation(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation check failed", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	policy := &entity.RolePolicy{
		RoleID:          uint(roleID),
		PermissionID:    req.PermissionID,
		PermissionLevel: req.PermissionLevel,
		ScopeType:       req.ScopeType,
	}

	if err := h.db.Create(policy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to add role policy",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 加载权限定义信息
	policy.PermissionName = permDef.Name
	policy.PermissionDisplayName = permDef.DisplayName
	policy.ResourceType = string(permDef.ResourceType)

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Policy added successfully",
		"data":      policy,
		"timestamp": time.Now(),
	})
}

// RemoveRolePolicy removes a permission policy from a role
// @Summary Remove role policy
// @Description Remove a permission policy from a specific role
// @Tags IAM-Role
// @Produce json
// @Security BearerAuth
// @Param id path int true "Role ID"
// @Param policy_id path int true "Policy ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id}/policies/{policy_id} [delete]
func (h *RoleHandler) RemoveRolePolicy(c *gin.Context) {
	roleIDStr := c.Param("id")
	roleID, err := strconv.ParseUint(roleIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	policyIDStr := c.Param("policy_id")
	policyID, err := strconv.ParseUint(policyIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid policy ID",
			"timestamp": time.Now(),
		})
		return
	}

	// 系统 Role 策略删除：fail-closed（C1）
	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
		return
	}
	_, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code": 401, "message": "User ID not found in context", "timestamp": time.Now(),
		})
		return
	}

	// 先按 role 所属租户绑定目标，再处理 policy。不能仅用 policy_id/role_id
	// 删除，否则拥有任一组织 IAM_ROLES 权限的调用方可删除其他组织的自定义 Role 策略。
	var role entity.Role
	if err := h.db.First(&role, uint(roleID)).Error; err != nil {
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
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// org_id=0 是平台 Role（包括历史遗留的非 is_system 行）；其策略只能由
	// 平台超管维护。is_system 单独判断可防御异常数据中的非零 org_id 系统 Role。
	if (role.IsSystem || role.OrgID == 0) && !isSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403, "message": "Platform role policies can only be modified by system admin", "timestamp": time.Now(),
		})
		return
	}

	if err := h.guard.EnsureCanMutateSystemRolePolicies(c.Request.Context(), isSystemAdmin, uint(roleID)); err != nil {
		if respondAntiEscalation(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation check failed", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	// 验证策略存在且属于该角色
	var policy entity.RolePolicy
	if err := h.db.Where("id = ? AND role_id = ?", policyID, roleID).First(&policy).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Policy not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get policy",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	if err := h.db.Delete(&policy).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to remove policy",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}

// AssignTeamRole assigns a role to a team
// @Summary Assign role to team
// @Description Assign a role to a team within a specific scope
// @Tags IAM-Team
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Param request body AssignRoleRequest true "Assignment information"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/roles [post]
func (h *RoleHandler) AssignTeamRole(c *gin.Context) {
	teamID := c.Param("id")

	var req AssignRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 验证角色存在
	var role entity.Role
	if err := h.db.First(&role, req.RoleID).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}
	if !h.ensureRoleVisible(c, &role) {
		return
	}

	// 验证作用域类型
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	assignedByStr, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
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
	// Team 本身须属于鉴权 org（与 List/Revoke 对齐，防跨租户赋权）——先于 guard，避免漏检
	teamOrg, err := loadTeamOrgID(c.Request.Context(), h.db, teamID)
	if err != nil || ensureTeamBelongsToAuthOrg(teamOrg, authOrg) != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Team not found", "timestamp": time.Now(),
		})
		return
	}
	// 防提权：与用户赋 Role 相同规则
	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
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

	// 检查是否已存在相同的角色分配
	var count int64
	err = h.db.Table("iam_team_roles").
		Where("team_id = ? AND role_id = ? AND scope_type = ? AND scope_id = ?",
			teamID, req.RoleID, req.ScopeType, req.ScopeID).
		Count(&count).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check existing role assignment",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	if count > 0 {
		// 已存在相同的角色分配
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   "Role already assigned to team in this scope",
			"timestamp": time.Now(),
		})
		return
	}

	teamRole := map[string]interface{}{
		"team_id":     teamID,
		"role_id":     req.RoleID,
		"scope_type":  req.ScopeType,
		"scope_id":    req.ScopeID,
		"assigned_by": assignedByStr,
		"assigned_at": time.Now(),
		"reason":      req.Reason,
	}

	// 解析过期时间（兼容 RFC3339 与 datetime-local）
	if req.ExpiresAt != "" {
		expiresAt, err := parseFlexibleTime(req.ExpiresAt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":      400,
				"message":   "Invalid expires_at format",
				"error":     err.Error(),
				"timestamp": time.Now(),
			})
			return
		}
		teamRole["expires_at"] = expiresAt
	}

	// Existing team members may predate the active-organization bootstrap.
	// Write the role and any missing user_organizations rows atomically so a
	// team role never authorizes users who cannot select this organization.
	if err := h.db.WithContext(c.Request.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Table("iam_team_roles").Create(&teamRole).Error; err != nil {
			return err
		}
		return ensureTeamMembersOrganizationMemberships(c.Request.Context(), tx, teamID, authOrg)
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to assign role to team",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Role assigned to team successfully",
		"data":      teamRole,
		"timestamp": time.Now(),
	})
}

// ListTeamRoles lists all roles for a team
// @Summary List team roles
// @Description Get all role assignments for a team across all scopes
// @Tags IAM-Team
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/roles [get]
func (h *RoleHandler) ListTeamRoles(c *gin.Context) {
	teamID := c.Param("id")

	authOrg := authOrgFromContext(c)
	if authOrg == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}
	// team 本身须属于鉴权 org
	teamOrg, err := loadTeamOrgID(c.Request.Context(), h.db, teamID)
	if err != nil || ensureTeamBelongsToAuthOrg(teamOrg, authOrg) != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Team not found", "timestamp": time.Now(),
		})
		return
	}

	var teamRoles []map[string]interface{}
	err = h.db.Table("iam_team_roles").
		Select("iam_team_roles.*, iam_roles.name as role_name, iam_roles.display_name as role_display_name").
		Joins("JOIN iam_roles ON iam_roles.id = iam_team_roles.role_id").
		Where("iam_team_roles.team_id = ?", teamID).
		Where("iam_roles.is_active = ?", true).
		Where("iam_team_roles.expires_at IS NULL OR iam_team_roles.expires_at > ?", time.Now()).
		Order("iam_team_roles.assigned_at DESC").
		Find(&teamRoles).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to list team roles",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 过滤 assignment scope 落在鉴权 org
	filtered := make([]map[string]interface{}, 0, len(teamRoles))
	for _, tr := range teamRoles {
		stStr, _ := tr["scope_type"].(string)
		var scopeID uint
		switch v := tr["scope_id"].(type) {
		case int64:
			scopeID = uint(v)
		case int:
			scopeID = uint(v)
		case uint:
			scopeID = v
		case float64:
			scopeID = uint(v)
		}
		st, err := valueobject.ParseScopeType(stStr)
		if err != nil {
			continue
		}
		if err := ensureScopeInAuthOrg(c.Request.Context(), h.db, st, scopeID, authOrg); err != nil {
			continue
		}
		filtered = append(filtered, tr)
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      filtered,
		"total":     len(filtered),
		"timestamp": time.Now(),
	})
}

// RevokeTeamRole revokes a team's role
// @Summary Revoke team role
// @Description Revoke a team's role assignment in a specific scope
// @Tags IAM-Team
// @Produce json
// @Security BearerAuth
// @Param id path string true "Team ID"
// @Param assignment_id path int true "Role assignment ID"
// @Success 204
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/teams/{id}/roles/{assignment_id} [delete]
func (h *RoleHandler) RevokeTeamRole(c *gin.Context) {
	teamID := c.Param("id")

	assignmentIDStr := c.Param("assignment_id")
	assignmentID, err := strconv.ParseUint(assignmentIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid assignment ID",
			"timestamp": time.Now(),
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
	teamOrg, err := loadTeamOrgID(c.Request.Context(), h.db, teamID)
	if err != nil || ensureTeamBelongsToAuthOrg(teamOrg, authOrg) != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}

	// 加载 assignment 并校验 scope
	var row struct {
		ScopeType string
		ScopeID   uint
	}
	err = h.db.Table("iam_team_roles").
		Select("scope_type, scope_id").
		Where("id = ? AND team_id = ?", assignmentID, teamID).
		Take(&row).Error
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "Role assignment not found",
			"timestamp": time.Now(),
		})
		return
	}
	st, err := valueobject.ParseScopeType(row.ScopeType)
	if err != nil || ensureScopeInAuthOrg(c.Request.Context(), h.db, st, row.ScopeID, authOrg) != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404, "message": "Role assignment not found", "timestamp": time.Now(),
		})
		return
	}

	if err := h.db.Table("iam_team_roles").Where("id = ?", assignmentID).Delete(nil).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to revoke team role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.Status(http.StatusNoContent)
}
