package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// CloneRoleRequest 克隆角色请求
type CloneRoleRequest struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
}

// CloneRole clones a role
// @Summary Clone role
// @Description Clone an existing role including all its permission policies
// @Tags IAM-Role
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path int true "Source role ID"
// @Param request body CloneRoleRequest true "New role information"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 409 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/roles/{id}/clone [post]
func (h *RoleHandler) CloneRole(c *gin.Context) {
	sourceIDStr := c.Param("id")
	sourceID, err := strconv.ParseUint(sourceIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid role ID",
			"timestamp": time.Now(),
		})
		return
	}

	var req CloneRoleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Invalid request",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 获取源角色
	var sourceRole entity.Role
	if err := h.db.First(&sourceRole, uint(sourceID)).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"code":      404,
				"message":   "Source role not found",
				"timestamp": time.Now(),
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get source role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	userIDStr, isSystemAdmin, ok := actorFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User ID not found in context",
			"timestamp": time.Now(),
		})
		return
	}

	// 防提权：fail-closed + 使用鉴权 org（C1）
	if h.guard == nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation not configured", "timestamp": time.Now(),
		})
		return
	}
	orgID := authOrgFromContext(c)
	if orgID == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400, "message": "auth org not resolved; pass org_id", "timestamp": time.Now(),
		})
		return
	}
	// 角色是租户资源。先验证源角色对当前鉴权组织可见，避免通过 role id
	// 克隆其他组织的自定义角色及其权限策略。
	if !h.ensureRoleVisible(c, &sourceRole) {
		return
	}
	// org_id=0 表示平台 Role；历史遗留的非 is_system 平台行也不能由租户
	// 管理员复制到其组织中。
	if (sourceRole.IsSystem || sourceRole.OrgID == 0) && !isSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403, "message": "Platform roles can only be cloned by system admin", "timestamp": time.Now(),
		})
		return
	}

	// role 名称只在同一租户内唯一；系统 Role 与其他租户的同名 Role 不应
	// 阻塞当前组织创建自己的自定义 Role。
	var existingRole entity.Role
	if err := h.db.Where("name = ? AND org_id = ?", req.Name, orgID).First(&existingRole).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   fmt.Sprintf("Role with name '%s' already exists", req.Name),
			"timestamp": time.Now(),
		})
		return
	} else if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Failed to check role name", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}
	if err := h.guard.EnsureCanCloneRole(c.Request.Context(), userIDStr, isSystemAdmin, uint(sourceID),
		valueobject.ScopeTypeOrganization, orgID); err != nil {
		if respondAntiEscalation(c, err) {
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500, "message": "Anti-escalation check failed", "error": err.Error(), "timestamp": time.Now(),
		})
		return
	}

	// 创建新角色
	newRole := &entity.Role{
		OrgID:       orgID,
		Name:        req.Name,
		DisplayName: req.DisplayName,
		Description: req.Description,
		IsSystem:    false, // 克隆的角色始终是自定义角色
		IsActive:    true,
		CreatedBy:   &userIDStr,
	}

	// 开始事务
	tx := h.db.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 创建新角色
	if err := tx.Create(newRole).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to create cloned role",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 获取源角色的所有策略
	var sourcePolicies []*entity.RolePolicy
	if err := tx.Where("role_id = ?", uint(sourceID)).Find(&sourcePolicies).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get source role policies",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 复制所有策略到新角色
	for _, sourcePolicy := range sourcePolicies {
		newPolicy := &entity.RolePolicy{
			RoleID:          newRole.ID,
			PermissionID:    sourcePolicy.PermissionID,
			PermissionLevel: sourcePolicy.PermissionLevel,
			ScopeType:       sourcePolicy.ScopeType,
		}
		if err := tx.Create(newPolicy).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Failed to clone role policies",
				"error":     err.Error(),
				"timestamp": time.Now(),
			})
			return
		}
	}

	// 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to commit transaction",
			"error":     err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":      201,
		"message":   fmt.Sprintf("Role cloned successfully from '%s'", sourceRole.DisplayName),
		"data":      newRole,
		"timestamp": time.Now(),
	})
}
