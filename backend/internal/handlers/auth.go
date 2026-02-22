package handlers

import (
	cryptoRand "crypto/rand"
	"fmt"
	"log"
	"net/http"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/models"
	"iac-platform/internal/services"
	"iac-platform/internal/services/sso"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// dummyHash 用于用户不存在时执行 bcrypt 比较，防止时序攻击
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dummy-timing-defense"), bcrypt.DefaultCost)

type AuthHandler struct {
	db         *gorm.DB
	mfaService *services.MFAService
	ssoService *sso.SSOService
}

func NewAuthHandler(db *gorm.DB) *AuthHandler {
	return &AuthHandler{
		db:         db,
		mfaService: services.NewMFAService(db),
		ssoService: sso.NewSSOService(db),
	}
}

type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type RegisterRequest struct {
	Username string `json:"username" binding:"required"`
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=6"`
}

// Login 用户登录
// @Summary 用户登录
// @Description 使用用户名和密码登录系统
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "登录信息"
// @Success 200 {object} map[string]interface{} "登录成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 401 {object} map[string]interface{} "用户名或密码错误"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/auth/login [post]
func (h *AuthHandler) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	var user models.User
	userNotFound := false
	if err := h.db.Where("username = ? AND is_active = ?", req.Username, true).First(&user).Error; err != nil {
		userNotFound = true
	}

	// 始终执行 bcrypt 比较，防止时序攻击枚举用户名
	hashToCompare := dummyHash
	if !userNotFound {
		hashToCompare = []byte(user.PasswordHash)
	}
	passwordErr := bcrypt.CompareHashAndPassword(hashToCompare, []byte(req.Password))

	if userNotFound || passwordErr != nil {
		log.Printf("[Auth] Login failed for username: %s", req.Username)
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Invalid credentials",
			"timestamp": time.Now(),
		})
		return
	}

	// 检查本地登录是否被禁用（超管例外）
	if h.ssoService.IsLocalLoginDisabled() && !user.IsSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{
			"code":      403,
			"message":   "Local login is disabled. Please use SSO to login.",
			"timestamp": time.Now(),
		})
		return
	}

	// Check if user ID is empty
	if user.ID == "" {
		log.Printf("[Auth] WARNING: User ID is empty for %s", user.Username)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "User ID is empty, please contact administrator",
			"timestamp": time.Now(),
		})
		return
	}

	// 检查是否需要MFA验证
	if user.MFAEnabled {
		// 用户已启用MFA，需要进行两步验证
		mfaToken, err := h.mfaService.CreateMFAToken(user.ID, c.ClientIP())
		if err != nil {
			log.Printf("[Auth] Failed to create MFA token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Failed to create MFA token",
				"timestamp": time.Now(),
			})
			return
		}

		// 获取MFA配置中的备用码数量要求
		mfaConfig, _ := h.mfaService.GetMFAConfig()
		requiredBackupCodes := 1
		if mfaConfig != nil {
			requiredBackupCodes = mfaConfig.RequiredBackupCodes
		}

		log.Printf("[Auth] MFA required for user: %s", user.Username)
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "需要MFA验证",
			"data": gin.H{
				"mfa_required":          true,
				"mfa_token":             mfaToken.Token,
				"expires_in":            300, // 5分钟
				"required_backup_codes": requiredBackupCodes,
				"user": gin.H{
					"username": user.Username,
				},
			},
			"timestamp": time.Now(),
		})
		return
	}

	// 检查是否需要强制设置MFA（新用户）
	mfaStatus, err := h.mfaService.GetUserMFAStatus(&user)
	if err == nil && mfaStatus.IsRequired && !user.MFAEnabled {
		// 需要设置MFA但尚未设置，返回需要设置MFA的提示
		// 先生成临时token让用户可以设置MFA
		mfaToken, err := h.mfaService.CreateMFAToken(user.ID, c.ClientIP())
		if err != nil {
			log.Printf("[Auth] Failed to create MFA token: %v", err)
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":      500,
				"message":   "Failed to create MFA token",
				"timestamp": time.Now(),
			})
			return
		}

		log.Printf("[Auth] MFA setup required for user: %s", user.Username)
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "需要设置MFA",
			"data": gin.H{
				"mfa_setup_required": true,
				"mfa_token":          mfaToken.Token,
				"expires_in":         300, // 5分钟
				"user": gin.H{
					"username": user.Username,
				},
			},
			"timestamp": time.Now(),
		})
		return
	}

	// 生成session ID
	sessionID, err := generateSessionID()
	if err != nil {
		log.Printf("[Auth] Failed to generate session ID: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to generate session",
			"timestamp": time.Now(),
		})
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
		log.Printf("[Auth] Failed to create session: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to create session",
			"timestamp": time.Now(),
		})
		return
	}

	// 生成JWT token（包含session_id）
	token, err := generateJWTWithSession(user.ID, user.Username, sessionID)
	if err != nil {
		log.Printf("[Auth] Failed to generate JWT: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to generate token",
			"timestamp": time.Now(),
		})
		return
	}

	log.Printf("[Auth] Login successful: %s", user.Username)

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "登录成功",
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
		"timestamp": time.Now(),
	})
}

// Register 用户注册
// @Summary 用户注册
// @Description 注册新用户账号
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body RegisterRequest true "注册信息"
// @Success 201 {object} map[string]interface{} "注册成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 409 {object} map[string]interface{} "用户名或邮箱已存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/auth/register [post]
func (h *AuthHandler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to hash password",
			"timestamp": time.Now(),
		})
		return
	}

	user := models.User{
		Username:     req.Username,
		Email:        req.Email,
		PasswordHash: string(hashedPassword),
		IsActive:     true,
	}

	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusConflict, gin.H{
			"code":      409,
			"message":   "Username or email already exists",
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "User created successfully",
		"data": gin.H{
			"id":             user.ID,
			"username":       user.Username,
			"email":          user.Email,
			"is_system_admin": user.IsSystemAdmin,
		},
		"timestamp": time.Now(),
	})
}

type ResetPasswordRequest struct {
	CurrentPassword string `json:"current_password" binding:"required"`
	NewPassword     string `json:"new_password" binding:"required,min=6"`
}

// ResetPassword 重置密码
// @Summary 重置密码
// @Description 用户重置自己的密码
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "密码重置信息"
// @Success 200 {object} map[string]interface{} "密码重置成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效或当前密码错误"
// @Failure 401 {object} map[string]interface{} "未授权"
// @Failure 404 {object} map[string]interface{} "用户不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/user/reset-password [post]
// @Security Bearer
func (h *AuthHandler) ResetPassword(c *gin.Context) {
	var req ResetPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   err.Error(),
			"timestamp": time.Now(),
		})
		return
	}

	// 从JWT中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Unauthorized",
			"timestamp": time.Now(),
		})
		return
	}

	var user models.User
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "User not found",
			"timestamp": time.Now(),
		})
		return
	}

	// 验证当前密码
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":      400,
			"message":   "Current password is incorrect",
			"timestamp": time.Now(),
		})
		return
	}

	// 生成新密码哈希
	newHashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to hash new password",
			"timestamp": time.Now(),
		})
		return
	}

	// 更新密码
	if err := h.db.Model(&user).Update("password_hash", string(newHashedPassword)).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to update password",
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Password updated successfully",
		"timestamp": time.Now(),
	})
}

// RefreshToken 刷新Token
// @Summary 刷新访问令牌
// @Description 使用当前有效的token获取新的token
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "Token刷新成功"
// @Failure 401 {object} map[string]interface{} "未授权或用户不存在"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/auth/refresh [post]
// @Security Bearer
func (h *AuthHandler) RefreshToken(c *gin.Context) {
	// 从JWT中获取用户信息
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Unauthorized",
			"timestamp": time.Now(),
		})
		return
	}

	// 验证用户仍然有效
	var user models.User
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "User not found or inactive",
			"timestamp": time.Now(),
		})
		return
	}

	// 生成新token
	newToken, err := generateJWT(user.ID, user.Username)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":      500,
			"message":   "Failed to generate new token",
			"timestamp": time.Now(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Token refreshed successfully",
		"data": gin.H{
			"token":      newToken,
			"expires_at": time.Now().Add(24 * time.Hour),
			"user": gin.H{
				"id":             user.ID,
				"username":       user.Username,
				"email":          user.Email,
				"is_system_admin": user.IsSystemAdmin,
			},
		},
		"timestamp": time.Now(),
	})
}

// GetMe 获取当前用户信息
// @Summary 获取当前用户信息
// @Description 获取当前登录用户的详细信息
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "获取成功"
// @Failure 401 {object} map[string]interface{} "未授权"
// @Failure 404 {object} map[string]interface{} "用户不存在"
// @Router /api/v1/auth/me [get]
// @Security Bearer
func (h *AuthHandler) GetMe(c *gin.Context) {
	// 从JWT中获取用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Unauthorized",
			"timestamp": time.Now(),
		})
		return
	}

	var user models.User
	if err := h.db.Where("user_id = ? AND is_active = ?", userID, true).First(&user).Error; err != nil {
		log.Printf("[Auth] GetMe: user not found: %s", userID)
		c.JSON(http.StatusNotFound, gin.H{
			"code":      404,
			"message":   "User not found",
			"timestamp": time.Now(),
		})
		return
	}


	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"id":             user.ID,
			"username":       user.Username,
			"email":          user.Email,
			"is_system_admin": user.IsSystemAdmin,
		},
		"timestamp": time.Now(),
	})
}

// Logout 用户登出
// @Summary 用户登出
// @Description 用户登出系统，吊销当前session和所有user token
// @Tags Auth
// @Accept json
// @Produce json
// @Success 200 {object} map[string]interface{} "登出成功"
// @Router /api/v1/auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	now := time.Now()

	// 只吊销当前login session（不吊销user token）
	// User token是用户手动创建的长期令牌，应该由用户自己管理
	sessionID, sessionExists := c.Get("session_id")
	if sessionExists && sessionID != "" {
		h.db.Table("login_sessions").
			Where("session_id = ?", sessionID).
			Updates(map[string]interface{}{
				"is_active":  false,
				"revoked_at": now,
			})
		log.Printf("[Auth] Logout: session revoked for user %s", c.GetString("user_id"))
	} else {
		log.Printf("[Auth] Logout called with user token, no session to revoke")
	}

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Logged out successfully",
		"timestamp": time.Now(),
	})
}

func generateJWT(userID string, username string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":  userID,
		"username": username,
		"exp":      time.Now().Add(24 * time.Hour).Unix(),
		"iat":      time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetJWTSecret()))
}

func generateJWTWithSession(userID string, username, sessionID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"session_id": sessionID,
		"type":       "login_token",
		"exp":        time.Now().Add(24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetJWTSecret()))
}

func generateSessionID() (string, error) {
	randStr, err := secureRandomString(16)
	if err != nil {
		return "", fmt.Errorf("failed to generate random string: %w", err)
	}
	return "session-" + time.Now().Format("20060102150405") + "-" + randStr, nil
}

func secureRandomString(length int) (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	randomBytes := make([]byte, length)
	if _, err := cryptoRand.Read(randomBytes); err != nil {
		return "", err
	}
	for i := range b {
		b[i] = charset[int(randomBytes[i])%len(charset)]
	}
	return string(b), nil
}
