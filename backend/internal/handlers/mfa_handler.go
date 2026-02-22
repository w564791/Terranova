package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"iac-platform/internal/models"
	"iac-platform/internal/services"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// MFAHandler MFA处理器
type MFAHandler struct {
	db         *gorm.DB
	mfaService *services.MFAService
}

// NewMFAHandler 创建MFA处理器实例
func NewMFAHandler(db *gorm.DB) *MFAHandler {
	return &MFAHandler{
		db:         db,
		mfaService: services.NewMFAService(db),
	}
}

// GetMFAStatus 获取当前用户MFA状态
// @Summary 获取MFA状态
// @Description 获取当前登录用户的MFA设置状态
// @Tags MFA
// @Produce json
// @Success 200 {object} models.MFAStatus
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/user/mfa/status [get]
// @Security Bearer
func (h *MFAHandler) GetMFAStatus(c *gin.Context) {
	user, err := h.getCurrentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Unauthorized"})
		return
	}

	status, err := h.mfaService.GetUserMFAStatus(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": status})
}

// SetupMFA 初始化MFA设置
// @Summary 初始化MFA设置
// @Description 生成TOTP密钥和二维码，开始MFA设置流程
// @Tags MFA
// @Produce json
// @Success 200 {object} models.MFASetupResponse
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/user/mfa/setup [post]
// @Security Bearer
func (h *MFAHandler) SetupMFA(c *gin.Context) {
	user, err := h.getCurrentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Unauthorized"})
		return
	}

	if user.MFAEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "MFA is already enabled"})
		return
	}

	response, err := h.mfaService.SetupMFA(user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": response})
}

// VerifyMFARequest 验证MFA请求
type VerifyMFARequest struct {
	Code string `json:"code" binding:"required"`
}

// VerifyAndEnableMFA 验证并启用MFA
// @Summary 验证并启用MFA
// @Description 验证TOTP码并启用MFA
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body VerifyMFARequest true "验证码"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/user/mfa/verify [post]
// @Security Bearer
func (h *MFAHandler) VerifyAndEnableMFA(c *gin.Context) {
	var req VerifyMFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user, err := h.getCurrentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Unauthorized"})
		return
	}

	if err := h.mfaService.VerifyAndEnableMFA(user, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "MFA已成功启用",
		"data": gin.H{
			"mfa_enabled":     true,
			"mfa_verified_at": user.MFAVerifiedAt,
		},
	})
}

// DisableMFARequest 禁用MFA请求
type DisableMFARequest struct {
	Code     string `json:"code" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// DisableMFA 禁用MFA
// @Summary 禁用MFA
// @Description 禁用MFA（需要验证密码和TOTP码）
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body DisableMFARequest true "验证信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Failure 403 {object} map[string]interface{}
// @Router /api/v1/user/mfa/disable [post]
// @Security Bearer
func (h *MFAHandler) DisableMFA(c *gin.Context) {
	var req DisableMFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user, err := h.getCurrentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Unauthorized"})
		return
	}

	// 验证密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.Password)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "密码错误"})
		return
	}

	if err := h.mfaService.DisableMFA(user, req.Code, req.Password); err != nil {
		if err.Error() == "MFA cannot be disabled due to security policy" {
			c.JSON(http.StatusForbidden, gin.H{"code": 403, "message": "根据安全策略，无法禁用MFA"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "MFA已禁用"})
}

// RegenerateBackupCodes 重新生成备用恢复码
// @Summary 重新生成备用恢复码
// @Description 重新生成备用恢复码（需要验证TOTP码）
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body VerifyMFARequest true "验证码"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/user/mfa/backup-codes/regenerate [post]
// @Security Bearer
func (h *MFAHandler) RegenerateBackupCodes(c *gin.Context) {
	var req VerifyMFARequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	user, err := h.getCurrentUser(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Unauthorized"})
		return
	}

	codes, err := h.mfaService.RegenerateBackupCodes(user, req.Code)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"backup_codes": codes,
		},
	})
}

// MFAVerifyRequest 登录MFA验证请求
type MFAVerifyRequest struct {
	MFAToken string `json:"mfa_token" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

// VerifyMFALogin 登录时验证MFA
// @Summary 登录MFA验证
// @Description 登录时验证TOTP码或备用恢复码
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body MFAVerifyRequest true "MFA验证信息"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/auth/mfa/verify [post]
func (h *MFAHandler) VerifyMFALogin(c *gin.Context) {
	var req MFAVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 验证MFA临时令牌
	mfaToken, err := h.mfaService.ValidateMFAToken(req.MFAToken, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": err.Error()})
		return
	}

	log.Printf("[MFA] Looking for user with ID: %s", mfaToken.UserID)

	// 获取用户
	var user models.User
	if err := h.db.Where("user_id = ?", mfaToken.UserID).First(&user).Error; err != nil {
		log.Printf("[MFA] User not found: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "User not found"})
		return
	}

	log.Printf("[MFA] User found: %s", user.Username)

	// 尝试验证TOTP码
	err = h.mfaService.VerifyMFACode(&user, req.Code)
	if err != nil {
		log.Printf("[MFA] TOTP verification failed: %v", err)
		// 如果TOTP验证失败，尝试验证备用恢复码
		err = h.mfaService.VerifyBackupCode(&user, req.Code)
		if err != nil {
			log.Printf("[MFA] Backup code verification failed: %v", err)
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "验证码无效"})
			return
		}
	}

	// 标记MFA令牌为已使用
	if err := h.mfaService.MarkMFATokenUsed(mfaToken); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to mark token as used"})
		return
	}

	// 生成session ID
	sessionID, err := generateSessionID()
	if err != nil {
		log.Printf("[MFA] Failed to generate session ID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to generate session"})
		return
	}

	// 创建session记录
	expiresAt := time.Now().Add(24 * time.Hour)
	session := models.LoginSession{
		SessionID: sessionID,
		UserID:    user.ID,
		Username:  user.Username,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		IsActive:  true,
	}

	if err := h.db.Create(&session).Error; err != nil {
		log.Printf("[MFA] Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to create session"})
		return
	}

	log.Printf("[MFA] Session created for user %s", user.Username)

	// 生成JWT token（包含session_id）
	token, err := generateJWTWithSession(user.ID, user.Username, sessionID)
	if err != nil {
		log.Printf("[MFA] Failed to generate JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to generate token"})
		return
	}

	log.Printf("[MFA] MFA verification successful for user %s", user.Username)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "MFA验证成功",
		"data": gin.H{
			"token":      token,
			"expires_at": expiresAt,
			"user": gin.H{
				"id":             user.ID,
				"username":       user.Username,
				"email":          user.Email,
				"is_system_admin": user.IsSystemAdmin,
			},
		},
	})
}

// 管理员API

// GetMFAConfig 获取MFA全局配置
// @Summary 获取MFA全局配置
// @Description 获取MFA全局配置和统计信息
// @Tags Admin MFA
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/admin/settings/mfa [get]
// @Security Bearer
func (h *MFAHandler) GetMFAConfig(c *gin.Context) {
	config, err := h.mfaService.GetMFAConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	stats, err := h.mfaService.GetMFAStatistics()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"config":     config,
			"statistics": stats,
		},
	})
}

// UpdateMFAConfigRequest 更新MFA配置请求
type UpdateMFAConfigRequest struct {
	Enabled                *bool   `json:"enabled"`
	Enforcement            *string `json:"enforcement"`
	Issuer                 *string `json:"issuer"`
	GracePeriodDays        *int    `json:"grace_period_days"`
	MaxFailedAttempts      *int    `json:"max_failed_attempts"`
	LockoutDurationMinutes *int    `json:"lockout_duration_minutes"`
	RequiredBackupCodes    *int    `json:"required_backup_codes"`
}

// UpdateMFAConfig 更新MFA全局配置
// @Summary 更新MFA全局配置
// @Description 更新MFA全局配置
// @Tags Admin MFA
// @Accept json
// @Produce json
// @Param request body UpdateMFAConfigRequest true "MFA配置"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/admin/settings/mfa [put]
// @Security Bearer
func (h *MFAHandler) UpdateMFAConfig(c *gin.Context) {
	var req UpdateMFAConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 获取当前配置
	config, err := h.mfaService.GetMFAConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	// 更新配置
	if req.Enabled != nil {
		config.Enabled = *req.Enabled
	}
	if req.Enforcement != nil {
		config.Enforcement = *req.Enforcement
	}
	if req.Issuer != nil {
		config.Issuer = *req.Issuer
	}
	if req.GracePeriodDays != nil {
		config.GracePeriodDays = *req.GracePeriodDays
	}
	if req.MaxFailedAttempts != nil {
		config.MaxFailedAttempts = *req.MaxFailedAttempts
	}
	if req.LockoutDurationMinutes != nil {
		config.LockoutDurationMinutes = *req.LockoutDurationMinutes
	}
	if req.RequiredBackupCodes != nil {
		config.RequiredBackupCodes = *req.RequiredBackupCodes
	}

	if err := h.mfaService.UpdateMFAConfig(config); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "MFA配置已更新"})
}

// ResetUserMFA 重置用户MFA
// @Summary 重置用户MFA
// @Description 管理员重置指定用户的MFA设置
// @Tags Admin MFA
// @Produce json
// @Param user_id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/admin/users/{user_id}/mfa/reset [post]
// @Security Bearer
func (h *MFAHandler) ResetUserMFA(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "user_id is required"})
		return
	}

	if err := h.mfaService.ResetUserMFA(userID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "message": "用户MFA已重置，用户下次登录需要重新设置MFA"})
}

// GetUserMFAStatus 获取指定用户MFA状态
// @Summary 获取用户MFA状态
// @Description 管理员获取指定用户的MFA状态
// @Tags Admin MFA
// @Produce json
// @Param user_id path string true "用户ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/admin/users/{user_id}/mfa/status [get]
// @Security Bearer
func (h *MFAHandler) GetUserMFAStatus(c *gin.Context) {
	userID := c.Param("user_id")
	if userID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "user_id is required"})
		return
	}

	var user models.User
	if err := h.db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "message": "User not found"})
		return
	}

	status, err := h.mfaService.GetUserMFAStatus(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": status})
}

// SetupMFAWithToken 使用 mfa_token 认证的 MFA 初始设置（用于首次登录强制设置 MFA）
// @Summary 初始化MFA设置（mfa_token认证）
// @Description 用户首次登录时通过 mfa_token 认证，初始化 MFA 设置
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body SetupMFAWithTokenRequest true "MFA Token"
// @Success 200 {object} models.MFASetupResponse
// @Failure 401 {object} map[string]interface{}
// @Router /api/v1/auth/mfa/setup [post]
func (h *MFAHandler) SetupMFAWithToken(c *gin.Context) {
	var req SetupMFAWithTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 验证 mfa_token
	mfaToken, err := h.mfaService.ValidateMFAToken(req.MFAToken, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Authorization required"})
		return
	}

	// 获取用户
	var user models.User
	if err := h.db.Where("user_id = ?", mfaToken.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "User not found"})
		return
	}

	if user.MFAEnabled {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": "MFA is already enabled"})
		return
	}

	response, err := h.mfaService.SetupMFA(&user)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "data": response})
}

// SetupMFAWithTokenRequest 使用 mfa_token 的 MFA 设置请求
type SetupMFAWithTokenRequest struct {
	MFAToken string `json:"mfa_token" binding:"required"`
}

// VerifyAndEnableMFAWithToken 使用 mfa_token 认证验证并启用 MFA
// @Summary 验证并启用MFA（mfa_token认证）
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body VerifyMFAWithTokenRequest true "MFA Token + 验证码"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/auth/mfa/enable [post]
func (h *MFAHandler) VerifyAndEnableMFAWithToken(c *gin.Context) {
	var req VerifyMFAWithTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 验证 mfa_token
	mfaToken, err := h.mfaService.ValidateMFAToken(req.MFAToken, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Authorization required"})
		return
	}

	// 获取用户
	var user models.User
	if err := h.db.Where("user_id = ?", mfaToken.UserID).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "User not found"})
		return
	}

	if err := h.mfaService.VerifyAndEnableMFA(&user, req.Code); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// MFA 启用成功，标记 token 已使用
	_ = h.mfaService.MarkMFATokenUsed(mfaToken)

	// 生成 session 和 JWT，直接登录
	sessionID, err := generateSessionID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to generate session"})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	session := models.LoginSession{
		SessionID: sessionID,
		UserID:    user.ID,
		Username:  user.Username,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		IsActive:  true,
	}

	if err := h.db.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to create session"})
		return
	}

	token, err := generateJWTWithSession(user.ID, user.Username, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "message": "Failed to generate token"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "MFA已成功启用",
		"data": gin.H{
			"mfa_enabled": true,
			"token":       token,
			"expires_at":  expiresAt,
			"user": gin.H{
				"id":             user.ID,
				"username":       user.Username,
				"email":          user.Email,
				"is_system_admin": user.IsSystemAdmin,
			},
		},
	})
}

// VerifyMFAWithTokenRequest 使用 mfa_token 的 MFA 验证请求
type VerifyMFAWithTokenRequest struct {
	MFAToken string `json:"mfa_token" binding:"required"`
	Code     string `json:"code" binding:"required"`
}

// 辅助方法

func (h *MFAHandler) getCurrentUser(c *gin.Context) (*models.User, error) {
	userID, exists := c.Get("user_id")
	if !exists {
		return nil, fmt.Errorf("user_id not found in context")
	}

	var user models.User
	if err := h.db.Where("user_id = ?", userID).First(&user).Error; err != nil {
		return nil, err
	}

	return &user, nil
}
