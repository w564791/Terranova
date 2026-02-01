package handlers

import (
	"net/http"
	"strconv"

	"iac-platform/internal/application/service"

	"github.com/gin-gonic/gin"
)

// UserHandler 用户处理器
type UserHandler struct {
	service *service.UserService
}

// NewUserHandler 创建用户处理器实例
func NewUserHandler(service *service.UserService) *UserHandler {
	return &UserHandler{
		service: service,
	}
}

// ListUsers 列出用户
// @Summary 列出用户
// @Tags IAM-User
// @Produce json
// @Param role query string false "角色筛选"
// @Param is_active query bool false "状态筛选"
// @Param search query string false "搜索关键词"
// @Param limit query int false "限制数量"
// @Param offset query int false "偏移量"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	req := &service.ListUsersRequest{
		Role:   c.Query("role"),
		Search: c.Query("search"),
	}

	// 解析is_active
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		val := isActiveStr == "true"
		req.IsActive = &val
	}

	// 解析limit和offset
	if limitStr := c.Query("limit"); limitStr != "" {
		if limit, err := strconv.Atoi(limitStr); err == nil {
			req.Limit = limit
		}
	}
	if offsetStr := c.Query("offset"); offsetStr != "" {
		if offset, err := strconv.Atoi(offsetStr); err == nil {
			req.Offset = offset
		}
	}

	users, total, err := h.service.ListUsers(c.Request.Context(), req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"users": users,
		"total": total,
	})
}

// GetUser 获取用户详情
// @Summary 获取用户详情
// @Tags IAM-User
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id} [get]
func (h *UserHandler) GetUser(c *gin.Context) {
	userID := c.Param("id")

	user, err := h.service.GetUser(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, user)
}

// UpdateUser 更新用户
// @Summary 更新用户
// @Tags IAM-User
// @Accept json
// @Produce json
// @Param id path int true "用户ID"
// @Param request body service.UpdateUserRequest true "更新用户请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id} [put]
func (h *UserHandler) UpdateUser(c *gin.Context) {
	userID := c.Param("id")

	var req service.UpdateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateUser(c.Request.Context(), userID, &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User updated successfully"})
}

// DeactivateUser 停用用户
// @Summary 停用用户
// @Tags IAM-User
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/deactivate [post]
func (h *UserHandler) DeactivateUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.DeactivateUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deactivated successfully"})
}

// ActivateUser 激活用户
// @Summary 激活用户
// @Tags IAM-User
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/activate [post]
func (h *UserHandler) ActivateUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.ActivateUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User activated successfully"})
}

// GetUserStats 获取用户统计
// @Summary 获取用户统计
// @Tags IAM-User
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/stats [get]
func (h *UserHandler) GetUserStats(c *gin.Context) {
	stats, err := h.service.GetUserStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CreateUser 创建用户
// @Summary 创建用户
// @Tags IAM-User
// @Accept json
// @Produce json
// @Param request body service.CreateUserRequest true "创建用户请求"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/iam/users [post]
func (h *UserHandler) CreateUser(c *gin.Context) {
	var req service.CreateUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := h.service.CreateUser(c.Request.Context(), &req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "User created successfully",
		"user":    user,
	})
}

// DeleteUser 删除用户
// @Summary 删除用户
// @Tags IAM-User
// @Produce json
// @Param id path int true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.DeleteUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
