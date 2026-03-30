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

// ListUsers lists users
// @Summary List users
// @Description List users with optional filters for status and search
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param is_active query bool false "Filter by active status"
// @Param search query string false "Search keyword"
// @Param limit query int false "Limit"
// @Param offset query int false "Offset"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users [get]
func (h *UserHandler) ListUsers(c *gin.Context) {
	req := &service.ListUsersRequest{
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

// GetUser gets user details
// @Summary Get user details
// @Description Get detailed information for a specific user
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
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

// UpdateUser updates a user
// @Summary Update user
// @Description Update user information
// @Tags IAM-User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Param request body service.UpdateUserRequest true "Update user request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
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

// DeactivateUser deactivates a user
// @Summary Deactivate user
// @Description Deactivate a user account
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/deactivate [post]
func (h *UserHandler) DeactivateUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.DeactivateUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deactivated successfully"})
}

// ActivateUser activates a user
// @Summary Activate user
// @Description Activate a user account
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id}/activate [post]
func (h *UserHandler) ActivateUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.ActivateUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User activated successfully"})
}

// GetUserStats gets user statistics
// @Summary Get user statistics
// @Description Get aggregated user statistics
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/iam/users/stats [get]
func (h *UserHandler) GetUserStats(c *gin.Context) {
	stats, err := h.service.GetUserStats(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// CreateUser creates a new user
// @Summary Create user
// @Description Create a new user account
// @Tags IAM-User
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body service.CreateUserRequest true "Create user request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
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

// DeleteUser deletes a user
// @Summary Delete user
// @Description Delete a user account
// @Tags IAM-User
// @Produce json
// @Security BearerAuth
// @Param id path string true "User ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/iam/users/{id} [delete]
func (h *UserHandler) DeleteUser(c *gin.Context) {
	userID := c.Param("id")

	if err := h.service.DeleteUser(c.Request.Context(), userID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "User deleted successfully"})
}
