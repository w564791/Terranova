package handlers

import (
	"net/http"

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

// CreateUserToken 创建用户Token
// @Summary 创建用户Token
// @Description 为当前用户创建一个新的访问Token
// @Tags User Settings
// @Accept json
// @Produce json
// @Param request body CreateUserTokenRequest true "创建Token请求"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
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

// ListUserTokens 列出当前用户的所有Token
// @Summary 列出用户Token
// @Description 获取当前用户的所有Token列表
// @Tags User Settings
// @Produce json
// @Success 200 {object} map[string]interface{}
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

// RevokeUserToken 吊销用户Token
// @Summary 吊销用户Token
// @Description 吊销当前用户的指定Token（使用token_name作为标识）
// @Tags User Settings
// @Param token_name path string true "Token Name"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
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

// ChangePassword 修改密码
// @Summary 修改密码
// @Description 修改当前用户的密码
// @Tags User Settings
// @Accept json
// @Produce json
// @Param request body ChangePasswordRequest true "修改密码请求"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
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

	c.JSON(http.StatusOK, gin.H{
		"message": "Password changed successfully",
	})
}
