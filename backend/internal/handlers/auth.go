package handlers

import (
	cryptoRand "crypto/rand"
	"fmt"
	"log"
	"net/http"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/models"
	"iac-platform/internal/observability/metrics"
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

// Login handles user login with username and password
// @Summary User login
// @Description Authenticate with username and password. Returns JWT token on success, or MFA challenge if MFA is enabled.
// @Tags Auth
// @Accept json
// @Produce json
// @Param request body LoginRequest true "Login credentials"
// @Success 200 {object} gin.H "Login successful or MFA required"
// @Failure 400 {object} gin.H "Invalid request parameters"
// @Failure 401 {object} gin.H "Invalid credentials"
// @Failure 403 {object} gin.H "Local login disabled"
// @Failure 500 {object} gin.H "Internal server error"
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
		metrics.IncLoginTotal("local", "failure")
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Invalid credentials",
			"timestamp": time.Now(),
		})
		return
	}

	// 检查本地登录是否被禁用（超管例外）
	if h.ssoService.IsLocalLoginDisabled() && !user.IsSystemAdmin {
		metrics.IncLoginTotal("local", "failure")
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
		metrics.IncLoginTotal("local", "failure")
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
		metrics.IncTokenIssued("mfa")
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
		metrics.IncTokenIssued("mfa")
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
	metrics.IncLoginTotal("local", "success")
	metrics.IncTokenIssued("access")

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

// Register handles new user registration.
// NOTE: Route is currently disabled (commented out in router). Not published in Swagger.
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

// ResetPassword handles password reset for the current user
// @Summary Reset password
// @Description Reset the current user's password by verifying the current password and setting a new one
// @Tags User
// @Accept json
// @Produce json
// @Param request body ResetPasswordRequest true "Password reset info"
// @Success 200 {object} gin.H "Password updated successfully"
// @Failure 400 {object} gin.H "Invalid request or incorrect current password"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 404 {object} gin.H "User not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/user/reset-password [post]
// @Security BearerAuth
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

	// 改密后吊销全部登录会话（强制重新登录）
	revokeAllLoginSessions(h.db, user.ID)

	c.JSON(http.StatusOK, gin.H{
		"code":      200,
		"message":   "Password updated successfully",
		"timestamp": time.Now(),
	})
}

// RefreshToken refreshes the current JWT token
// @Summary Refresh access token
// @Description Use a valid JWT token to obtain a new token with extended expiration
// @Tags Auth
// @Produce json
// @Success 200 {object} gin.H "Token refreshed successfully"
// @Failure 401 {object} gin.H "Unauthorized or user not found"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/refresh [post]
// @Security BearerAuth
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

	// 必须携带 login session（与 JWTAuth 中间件对齐；拒绝无 type/session 的旧 refresh 产物）
	sessionIDRaw, sessionOK := c.Get("session_id")
	sessionID, _ := sessionIDRaw.(string)
	if !sessionOK || sessionID == "" {
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":      401,
			"message":   "Invalid token format: missing session. Please login again.",
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

	// 延长 session 过期时间并签发带 type/session_id 的新 token
	newExpiry := time.Now().Add(24 * time.Hour)
	if err := h.db.Table("login_sessions").
		Where("session_id = ? AND is_active = ?", sessionID, true).
		Updates(map[string]interface{}{
			"expires_at":  newExpiry,
			"last_used_at": time.Now(),
		}).Error; err != nil {
		log.Printf("[Auth] Failed to extend session on refresh: %v", err)
	}

	newToken, err := generateJWTWithSession(user.ID, user.Username, sessionID)
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
			"expires_at": newExpiry,
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

// GetMe returns the current authenticated user's information
// @Summary Get current user info
// @Description Get the profile details of the currently authenticated user
// @Tags Auth
// @Produce json
// @Success 200 {object} gin.H "User info retrieved successfully"
// @Failure 401 {object} gin.H "Unauthorized"
// @Failure 404 {object} gin.H "User not found"
// @Router /api/v1/auth/me [get]
// @Security BearerAuth
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

// Logout handles user logout by revoking the current login session
// @Summary User logout
// @Description Log out the current user and revoke the active login session
// @Tags Auth
// @Produce json
// @Success 200 {object} gin.H "Logged out successfully"
// @Router /api/v1/auth/logout [post]
// @Security BearerAuth
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

// generateJWT 已弃用：缺 type/session_id，JWTAuth 会拒绝。保留仅供测试/兼容探测。
func generateJWT(userID string, username string) (string, error) {
	return generateJWTWithSession(userID, username, "legacy-no-session")
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

// revokeAllLoginSessions 吊销用户全部登录会话（改密/重置密码后强制重登）
func revokeAllLoginSessions(db *gorm.DB, userID string) {
	if db == nil || userID == "" {
		return
	}
	now := time.Now()
	if err := db.Table("login_sessions").
		Where("user_id = ? AND is_active = ?", userID, true).
		Updates(map[string]interface{}{
			"is_active":  false,
			"revoked_at": now,
		}).Error; err != nil {
		log.Printf("[Auth] Failed to revoke login sessions for %s: %v", userID, err)
	}
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
