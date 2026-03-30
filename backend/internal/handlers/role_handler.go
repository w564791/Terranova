package handlers

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// RoleHandler IAM角色处理器
type RoleHandler struct {
	db *gorm.DB
}

// NewRoleHandler 创建角色处理器
func NewRoleHandler(db *gorm.DB) *RoleHandler {
	return &RoleHandler{db: db}
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

	query := h.db.Model(&entity.Role{})

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

	role := &entity.Role{
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

	// 系统角色不能修改名称
	if role.IsSystem && req.DisplayName != "" {
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

	// 系统角色不能删除
	if role.IsSystem {
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

	// 验证作用域类型
	_, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	assignedByInterface, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
		})
		return
	}

	assignedByStr, ok := assignedByInterface.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Invalid user ID type",
			"timestamp": time.Now(),
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

	// 解析过期时间
	if req.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
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

	if err := h.db.Create(userRole).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to assign role",
			"error":     err.Error(),
			"timestamp": time.Now(),
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

	// 过滤掉未激活的角色
	activeUserRoles := make([]*entity.UserRole, 0)
	for _, userRole := range userRoles {
		if userRole.RoleName != "" {
			activeUserRoles = append(activeUserRoles, userRole)
		}
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

	// 验证作用域类型
	_, err = valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
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

	// 验证作用域类型
	_, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid scope type",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	assignedByInterface, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
		})
		return
	}

	assignedByStr, ok := assignedByInterface.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Invalid user ID type",
			"timestamp": time.Now(),
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

	// 解析过期时间
	if req.ExpiresAt != "" {
		expiresAt, err := time.Parse(time.RFC3339, req.ExpiresAt)
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

	if err := h.db.Table("iam_team_roles").Create(&teamRole).Error; err != nil {
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

	var teamRoles []map[string]interface{}
	err := h.db.Table("iam_team_roles").
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

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"data":      teamRoles,
		"total":     len(teamRoles),
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

	// 验证角色分配存在且属于该团队
	var count int64
	err = h.db.Table("iam_team_roles").
		Where("id = ? AND team_id = ?", assignmentID, teamID).
		Count(&count).Error

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to check role assignment",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	if count == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "Role assignment not found",
			"timestamp": time.Now(),
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
