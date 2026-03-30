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

// GetMFAStatus returns the MFA status for the current user
// @Summary Get current user MFA status
// @Description Get the MFA configuration status for the currently authenticated user
// @Tags MFA
// @Produce json
// @Success 200 {object} gin.H "MFA status retrieved"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/user/mfa/status [get]
// @Security BearerAuth
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

// SetupMFA initializes MFA setup for the current user
// @Summary Initialize MFA setup
// @Description Generate TOTP secret key and QR code to begin MFA setup
// @Tags MFA
// @Produce json
// @Success 200 {object} gin.H "MFA setup data with QR code"
// @Failure 400 {object} gin.H "MFA already enabled"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/user/mfa/setup [post]
// @Security BearerAuth
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

// VerifyAndEnableMFA verifies the TOTP code and enables MFA
// @Summary Verify and enable MFA
// @Description Verify the TOTP code from authenticator app and enable MFA for the current user
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body VerifyMFARequest true "TOTP verification code"
// @Success 200 {object} gin.H "MFA enabled successfully"
// @Failure 400 {object} gin.H "Invalid verification code"
// @Failure 401 {object} gin.H "Unauthorized"
// @Router /api/v1/user/mfa/verify [post]
// @Security BearerAuth
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

// DisableMFA disables MFA for the current user
// @Summary Disable MFA
// @Description Disable MFA after verifying password and TOTP code
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body DisableMFARequest true "Password and TOTP code for verification"
// @Success 200 {object} gin.H "MFA disabled"
// @Failure 400 {object} gin.H "Invalid password or verification code"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 403 {object} gin.H "Cannot disable MFA due to security policy"
// @Router /api/v1/user/mfa/disable [post]
// @Security BearerAuth
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

// RegenerateBackupCodes regenerates MFA backup recovery codes
// @Summary Regenerate backup codes
// @Description Regenerate MFA backup recovery codes after verifying TOTP code
// @Tags MFA
// @Accept json
// @Produce json
// @Param request body VerifyMFARequest true "TOTP verification code"
// @Success 200 {object} gin.H "New backup codes generated"
// @Failure 400 {object} gin.H "Invalid verification code"
// @Failure 401 {object} gin.H "Unauthorized"
// @Router /api/v1/user/mfa/backup-codes/regenerate [post]
// @Security BearerAuth
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

// VerifyMFALogin verifies MFA code during the login flow
// @Summary Verify MFA during login
// @Description Verify TOTP code or backup recovery code to complete MFA-protected login. Uses mfa_token from login response, not JWT.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body MFAVerifyRequest true "MFA token and verification code"
// @Success 200 {object} gin.H "MFA verified, JWT token returned"
// @Failure 400 {object} gin.H "Invalid request parameters"
// @Failure 401 {object} gin.H "Invalid MFA token or verification code"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/mfa/verify [post]
func (h *MFAHandler) VerifyMFALogin(c *gin.Context) {
	var req MFAVerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 验证并消费MFA临时令牌（原子操作，防止竞争条件）
	mfaToken, err := h.mfaService.ValidateAndConsumeMFAToken(req.MFAToken, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Invalid or expired MFA token"})
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

// GetMFAConfig returns the global MFA configuration and statistics
// @Summary Get global MFA configuration
// @Description Get the global MFA configuration and usage statistics (admin only)
// @Tags Admin MFA
// @Produce json
// @Success 200 {object} gin.H "MFA config and statistics"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/global/settings/mfa [get]
// @Security BearerAuth
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

// UpdateMFAConfig updates the global MFA configuration
// @Summary Update global MFA configuration
// @Description Update the global MFA configuration settings (admin only)
// @Tags Admin MFA
// @Accept json
// @Produce json
// @Param request body UpdateMFAConfigRequest true "MFA configuration"
// @Success 200 {object} gin.H "MFA config updated"
// @Failure 400 {object} gin.H "Invalid request parameters"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/global/settings/mfa [put]
// @Security BearerAuth
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

// ResetUserMFA resets MFA for a specific user (admin only)
// @Summary Reset user MFA
// @Description Admin resets MFA settings for a specific user, requiring them to set up MFA again on next login
// @Tags Admin MFA
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} gin.H "User MFA reset"
// @Failure 400 {object} gin.H "Missing user_id"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/users/{user_id}/mfa/reset [post]
// @Security BearerAuth
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

// GetUserMFAStatus returns MFA status for a specific user (admin only)
// @Summary Get user MFA status
// @Description Admin retrieves MFA status for a specific user
// @Tags Admin MFA
// @Produce json
// @Param user_id path string true "User ID"
// @Success 200 {object} gin.H "User MFA status"
// @Failure 400 {object} gin.H "Missing user_id"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 404 {object} gin.H "User not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/users/{user_id}/mfa/status [get]
// @Security BearerAuth
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

// SetupMFAWithToken initializes MFA setup using mfa_token authentication (for forced MFA setup on first login)
// @Summary Initialize MFA setup (mfa_token auth)
// @Description Initialize MFA setup during login flow using mfa_token instead of JWT. Used when MFA is required for first-time login.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body SetupMFAWithTokenRequest true "MFA token"
// @Success 200 {object} gin.H "MFA setup data with QR code"
// @Failure 400 {object} gin.H "MFA already enabled"
// @Failure 401 {object} gin.H "Invalid or expired MFA token"
// @Failure 500 {object} gin.H "Internal server error"
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

// VerifyAndEnableMFAWithToken verifies TOTP code and enables MFA using mfa_token authentication
// @Summary Verify and enable MFA (mfa_token auth)
// @Description Verify TOTP code and enable MFA during login flow. On success, returns JWT token to complete login.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body VerifyMFAWithTokenRequest true "MFA token and TOTP verification code"
// @Success 200 {object} gin.H "MFA enabled and JWT token returned"
// @Failure 400 {object} gin.H "Invalid verification code"
// @Failure 401 {object} gin.H "Invalid or expired MFA token"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/mfa/enable [post]
func (h *MFAHandler) VerifyAndEnableMFAWithToken(c *gin.Context) {
	var req VerifyMFAWithTokenRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "message": err.Error()})
		return
	}

	// 验证并消费 mfa_token（原子操作）
	mfaToken, err := h.mfaService.ValidateAndConsumeMFAToken(req.MFAToken, c.ClientIP())
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "message": "Invalid or expired MFA token"})
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
