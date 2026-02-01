package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"iac-platform/internal/domain/entity"
)

// CloneRoleRequest 克隆角色请求
type CloneRoleRequest struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
}

// CloneRole 克隆角色
// @Summary 克隆角色
// @Description 克隆一个现有角色，包括其所有权限策略
// @Tags IAM-Roles
// @Accept json
// @Produce json
// @Param id path int true "源角色ID"
// @Param request body CloneRoleRequest true "新角色信息"
// @Success 201 {object} entity.Role
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

	// 检查新角色名称是否已存在
	var existingRole entity.Role
	if err := h.db.Where("name = ?", req.Name).First(&existingRole).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   fmt.Sprintf("Role with name '%s' already exists", req.Name),
			"timestamp": time.Now(),
		})
		return
	}

	userIDInterface, _ := c.Get("user_id")
	userIDStr, ok := userIDInterface.(string)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to get user ID",
			"timestamp": time.Now(),
		})
		return
	}

	// 创建新角色
	newRole := &entity.Role{
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
