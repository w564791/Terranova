package handlers

import (
	"net/http"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// UserTokenHandler 用户Token处理器
type UserTokenHandler struct {
	service *service.UserTokenService
	db      *gorm.DB
}

// NewUserTokenHandler 创建用户Token处理器实例
func NewUserTokenHandler(service *service.UserTokenService, db *gorm.DB) *UserTokenHandler {
	return &UserTokenHandler{
		service: service,
		db:      db,
	}
}

// CreateUserTokenRequest 创建用户Token请求
type CreateUserTokenRequest struct {
	TokenName     string `json:"token_name" binding:"required"`
	ExpiresInDays int    `json:"expires_in_days"` // 0表示永不过期
}

// CreateUserToken creates a user token
// @Summary Create user token
// @Description Create a new access token for the current user
// @Tags User-Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body CreateUserTokenRequest true "Create token request"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/user/tokens [post]
func (h *UserTokenHandler) CreateUserToken(c *gin.Context) {
	// 从上下文获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// 解析请求
	var req CreateUserTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证过期天数
	if req.ExpiresInDays < 0 || req.ExpiresInDays > 365 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "expires_in_days must be between 0 and 365"})
		return
	}

	// 生成token
	tokenResp, err := h.service.GenerateToken(c.Request.Context(), userID.(string), req.TokenName, req.ExpiresInDays)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"message": "Token created successfully",
		"data":    tokenResp,
	})
}

// ListUserTokens lists all tokens for the current user
// @Summary List user tokens
// @Description Get all tokens for the current user
// @Tags User-Settings
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/user/tokens [get]
func (h *UserTokenHandler) ListUserTokens(c *gin.Context) {
	// 从上下文获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// 获取token列表
	tokens, err := h.service.ListUserTokens(c.Request.Context(), userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"data": tokens,
	})
}

// RevokeUserToken revokes a user token
// @Summary Revoke user token
// @Description Revoke a specific token for the current user (using token_name as identifier)
// @Tags User-Settings
// @Produce json
// @Security BearerAuth
// @Param token_name path string true "Token Name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/user/tokens/{token_name} [delete]
func (h *UserTokenHandler) RevokeUserToken(c *gin.Context) {
	// 从上下文获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// 获取token name
	tokenName := c.Param("token_name")
	if tokenName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token_name is required"})
		return
	}

	// 吊销token
	if err := h.service.RevokeToken(c.Request.Context(), userID.(string), tokenName); err != nil {
		if err.Error() == "token not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Token revoked successfully",
	})
}

// ChangePasswordRequest 修改密码请求
type ChangePasswordRequest struct {
	OldPassword string `json:"old_password" binding:"required"`
	NewPassword string `json:"new_password" binding:"required,min=6"`
}

// ChangePassword changes the current user's password
// @Summary Change password
// @Description Change the current user's password
// @Tags User-Settings
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param request body ChangePasswordRequest true "Change password request"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/user/change-password [post]
func (h *UserTokenHandler) ChangePassword(c *gin.Context) {
	// 从上下文获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "User not authenticated"})
		return
	}

	// 解析请求
	var req ChangePasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 验证新密码长度
	if len(req.NewPassword) < 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "New password must be at least 6 characters"})
		return
	}

	// 查询用户
	var user models.User
	if err := h.db.Where("user_id = ? AND is_active = ?", userID.(string), true).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	// 验证旧密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.OldPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Old password is incorrect"})
		return
	}

	// 生成新密码哈希
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to hash new password"})
		return
	}

	// 更新密码
	if err := h.db.Model(&user).Update("password_hash", string(newHashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update password"})
		return
	}

	// 改密后吊销全部登录会话，强制重新登录
	now := time.Now()
	_ = h.db.Table("login_sessions").
		Where("user_id = ? AND is_active = ?", user.ID, true).
		Updates(map[string]interface{}{
			"is_active":  false,
			"revoked_at": now,
		})

	c.JSON(http.StatusOK, gin.H{
		"message": "Password changed successfully; please login again",
	})
}
