package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"iac-platform/internal/models"
	"iac-platform/internal/observability/metrics"
	"iac-platform/internal/services"
	"iac-platform/internal/services/sso"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// SSOHandler SSO 相关的 HTTP 处理器
type SSOHandler struct {
	db         *gorm.DB
	ssoService *sso.SSOService
	mfaService *services.MFAService
}

// NewSSOHandler 创建 SSO Handler
func NewSSOHandler(db *gorm.DB) *SSOHandler {
	return &SSOHandler{
		db:         db,
		ssoService: sso.NewSSOService(db),
		mfaService: services.NewMFAService(db),
	}
}

// ============================================
// 公开端点（无需认证）
// ============================================

// GetProviders returns the list of enabled SSO providers for the login page
// @Summary Get available SSO providers
// @Description Get the list of enabled SSO providers and global SSO config (e.g. whether local login is disabled)
// @Tags SSO
// @Produce json
// @Success 200 {object} gin.H "SSO providers list and config"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/sso/providers [get]
func (h *SSOHandler) GetProviders(c *gin.Context) {
	providers, err := h.ssoService.GetEnabledProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get SSO providers",
		})
		return
	}

	ssoConfig := h.ssoService.GetSSOConfig()

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"providers":           providers,
			"disable_local_login": ssoConfig.DisableLocalLogin,
		},
	})
}

// Login initiates SSO login by returning the provider's authorization URL
// @Summary Initiate SSO login
// @Description Generate the OAuth authorization URL for the specified SSO provider. The frontend should redirect the user to this URL.
// @Tags SSO
// @Produce json
// @Param provider path string true "SSO provider key"
// @Param redirect_url query string false "URL to redirect after login completes"
// @Success 200 {object} gin.H "Authorization URL"
// @Failure 400 {object} gin.H "Invalid provider or failed to generate auth URL"
// @Router /api/v1/auth/sso/{provider}/login [get]
func (h *SSOHandler) Login(c *gin.Context) {
	providerKey := c.Param("provider")
	if providerKey == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "provider is required",
		})
		return
	}

	// 前端回调后跳转的 URL
	redirectURL := c.Query("redirect_url")
	if redirectURL == "" {
		redirectURL = "/"
	}

	authURL, _, err := h.ssoService.GenerateAuthURL(providerKey, redirectURL, "login", "")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("Failed to generate auth URL: %v", err),
		})
		return
	}

	// 返回授权 URL，由前端进行重定向
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"auth_url": authURL,
		},
	})
}

// Callback handles the SSO callback (API mode, returns JSON)
// @Summary Handle SSO callback
// @Description Process the OAuth callback from the SSO provider. Returns JWT token on success, or MFA challenge if required.
// @Tags SSO
// @Produce json
// @Param provider path string true "SSO provider key"
// @Param code query string true "OAuth authorization code"
// @Param state query string true "OAuth state parameter"
// @Success 200 {object} gin.H "Login successful with JWT token, or MFA required"
// @Failure 400 {object} gin.H "Missing code/state or SSO error"
// @Failure 401 {object} gin.H "SSO authentication failed"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/sso/{provider}/callback [get]
func (h *SSOHandler) Callback(c *gin.Context) {
	providerKey := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		// 检查是否有错误
		errMsg := c.Query("error")
		errDesc := c.Query("error_description")
		if errMsg != "" {
			metrics.IncLoginTotal("sso", "failure")
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": fmt.Sprintf("SSO error: %s - %s", errMsg, errDesc),
			})
			return
		}
		metrics.IncLoginTotal("sso", "failure")
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "authorization code is required",
		})
		return
	}

	if state == "" {
		metrics.IncLoginTotal("sso", "failure")
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "state parameter is required",
		})
		return
	}

	// 处理回调
	result, err := h.ssoService.HandleCallback(
		c.Request.Context(),
		providerKey,
		code,
		state,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		metrics.IncLoginTotal("sso", "failure")
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": err.Error(),
		})
		return
	}

	// 新用户第一次登录：生成 mfa_token（非 JWT），用户必须完成 MFA 设置后才能获得 JWT
	if result.IsNewUser && !result.User.MFAEnabled {
		mfaConfig, _ := h.mfaService.GetMFAConfig()
		if mfaConfig != nil && mfaConfig.Enabled {
			mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"code":    500,
					"message": "Failed to create MFA token",
				})
				return
			}

			metrics.IncTokenIssued("mfa")
			c.JSON(http.StatusOK, gin.H{
				"code":    200,
				"message": "MFA setup required for new user",
				"data": gin.H{
					"mfa_setup_required": true,
					"mfa_token":          mfaToken.Token,
					"expires_in":         300,
					"is_new_user":        true,
					"user": gin.H{
						"username": result.User.Username,
					},
				},
			})
			return
		}
	}

	// 检查 MFA（已有用户）
	if result.User.MFAEnabled {
		mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "Failed to create MFA token",
			})
			return
		}

		mfaConfig, _ := h.mfaService.GetMFAConfig()
		requiredBackupCodes := 1
		if mfaConfig != nil {
			requiredBackupCodes = mfaConfig.RequiredBackupCodes
		}

		metrics.IncTokenIssued("mfa")
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "MFA verification required",
			"data": gin.H{
				"mfa_required":          true,
				"mfa_token":             mfaToken.Token,
				"expires_in":            300,
				"required_backup_codes": requiredBackupCodes,
				"is_new_user":           result.IsNewUser,
				"user": gin.H{
					"username": result.User.Username,
				},
			},
		})
		return
	}

	// 检查 MFA 强制策略
	mfaStatus, err := h.mfaService.GetUserMFAStatus(result.User)
	if err == nil && mfaStatus.IsRequired && !result.User.MFAEnabled {
		mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"code":    500,
				"message": "Failed to create MFA token",
			})
			return
		}

		metrics.IncTokenIssued("mfa")
		c.JSON(http.StatusOK, gin.H{
			"code":    200,
			"message": "MFA setup required",
			"data": gin.H{
				"mfa_setup_required": true,
				"mfa_token":          mfaToken.Token,
				"expires_in":         300,
				"is_new_user":        result.IsNewUser,
				"user": gin.H{
					"username": result.User.Username,
				},
			},
		})
		return
	}

	// 生成 session 和 JWT
	sessionID, err := generateSessionID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to generate session",
		})
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	session := models.LoginSession{
		SessionID: sessionID,
		UserID:    result.User.ID,
		Username:  result.User.Username,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		IsActive:  true,
	}

	if err := h.db.Create(&session).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to create session",
		})
		return
	}

	token, err := generateJWTWithSession(result.User.ID, result.User.Username, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to generate token",
		})
		return
	}

	metrics.IncLoginTotal("sso", "success")
	metrics.IncTokenIssued("access")
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SSO login successful",
		"data": gin.H{
			"token":       token,
			"expires_at":  expiresAt,
			"is_new_user": result.IsNewUser,
			"user": gin.H{
				"id":             result.User.ID,
				"username":       result.User.Username,
				"email":          result.User.Email,
				"is_system_admin": result.User.IsSystemAdmin,
			},
		},
	})
}

// CallbackRedirect handles the SSO callback in redirect mode
// @Summary Handle SSO callback (redirect mode)
// @Description Process the OAuth callback and redirect to the frontend page with token or error in URL parameters
// @Tags SSO
// @Param provider path string true "SSO provider key"
// @Param code query string true "OAuth authorization code"
// @Param state query string true "OAuth state parameter"
// @Success 302 "Redirect to frontend with token"
// @Failure 302 "Redirect to frontend with error"
// @Router /api/v1/auth/sso/{provider}/callback/redirect [get]
func (h *SSOHandler) CallbackRedirect(c *gin.Context) {
	providerKey := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	// 默认前端回调页面
	frontendCallbackURL := "/sso/callback"

	if code == "" {
		errMsg := c.Query("error")
		errDesc := c.Query("error_description")
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=%s&error_description=%s",
			frontendCallbackURL, url.QueryEscape(errMsg), url.QueryEscape(errDesc)))
		return
	}

	result, err := h.ssoService.HandleCallback(
		c.Request.Context(),
		providerKey,
		code,
		state,
		c.ClientIP(),
		c.Request.UserAgent(),
	)
	if err != nil {
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=sso_failed&error_description=%s",
			frontendCallbackURL, url.QueryEscape(err.Error())))
		return
	}

	// 检查 MFA（与 Callback 端点保持一致的安全策略）

	// 新用户第一次登录：生成 mfa_token（非 JWT），用户必须完成 MFA 设置后才能获得 JWT
	if result.IsNewUser && !result.User.MFAEnabled {
		mfaConfig, _ := h.mfaService.GetMFAConfig()
		if mfaConfig != nil && mfaConfig.Enabled {
			mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
			if err != nil {
				c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=mfa_error&error_description=%s",
					frontendCallbackURL, url.QueryEscape("Failed to create MFA token")))
				return
			}
			c.Redirect(http.StatusFound, fmt.Sprintf("%s?mfa_setup_required=true&mfa_token=%s&is_new_user=true",
				frontendCallbackURL, url.QueryEscape(mfaToken.Token)))
			return
		}
	}

	// 已有用户且已启用 MFA：需要 MFA 验证
	if result.User.MFAEnabled {
		mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
		if err != nil {
			c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=mfa_error&error_description=%s",
				frontendCallbackURL, url.QueryEscape("Failed to create MFA token")))
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?mfa_required=true&mfa_token=%s&is_new_user=%v",
			frontendCallbackURL, url.QueryEscape(mfaToken.Token), result.IsNewUser))
		return
	}

	// 检查 MFA 强制策略（已有用户但未启用 MFA）
	mfaStatus, err := h.mfaService.GetUserMFAStatus(result.User)
	if err == nil && mfaStatus.IsRequired && !result.User.MFAEnabled {
		mfaToken, err := h.mfaService.CreateMFAToken(result.User.ID, c.ClientIP())
		if err != nil {
			c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=mfa_error&error_description=%s",
				frontendCallbackURL, url.QueryEscape("Failed to create MFA token")))
			return
		}
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?mfa_setup_required=true&mfa_token=%s&is_new_user=%v",
			frontendCallbackURL, url.QueryEscape(mfaToken.Token), result.IsNewUser))
		return
	}

	// 无需 MFA，生成 JWT
	sessionID, err := generateSessionID()
	if err != nil {
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=session_error&error_description=%s",
			frontendCallbackURL, url.QueryEscape("Failed to generate session")))
		return
	}

	expiresAt := time.Now().Add(24 * time.Hour)
	session := models.LoginSession{
		SessionID: sessionID,
		UserID:    result.User.ID,
		Username:  result.User.Username,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
		IPAddress: c.ClientIP(),
		UserAgent: c.Request.UserAgent(),
		IsActive:  true,
	}
	if err := h.db.Create(&session).Error; err != nil {
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=session_error&error_description=%s",
			frontendCallbackURL, url.QueryEscape("Failed to create session")))
		return
	}

	token, err := generateJWTWithSession(result.User.ID, result.User.Username, sessionID)
	if err != nil {
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=token_error&error_description=%s",
			frontendCallbackURL, url.QueryEscape("Failed to generate token")))
		return
	}

	// 重定向到前端，携带 token
	c.Redirect(http.StatusFound, fmt.Sprintf("%s?token=%s&is_new_user=%v",
		frontendCallbackURL, url.QueryEscape(token), result.IsNewUser))
}

// ============================================
// 需要认证的端点
// ============================================

// GetIdentities returns the SSO identities linked to the current user
// @Summary Get linked SSO identities
// @Description Get the list of SSO identities linked to the currently authenticated user
// @Tags SSO
// @Produce json
// @Success 200 {object} gin.H "List of linked identities"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/auth/sso/identities [get]
// @Security BearerAuth
func (h *SSOHandler) GetIdentities(c *gin.Context) {
	userID, _ := c.Get("user_id")

	identities, err := h.ssoService.GetUserIdentities(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get identities",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    identities,
	})
}

// LinkIdentity initiates linking a new SSO identity to the current user
// @Summary Link SSO identity
// @Description Generate an OAuth authorization URL to link a new SSO identity to the current user
// @Tags SSO
// @Accept json
// @Produce json
// @Param request body object true "Provider key and optional redirect URL" example({"provider_key": "github", "redirect_url": "/settings"})
// @Success 200 {object} gin.H "Authorization URL for identity linking"
// @Failure 400 {object} gin.H "Invalid request or failed to generate auth URL"
// @Router /api/v1/auth/sso/identities/link [post]
// @Security BearerAuth
func (h *SSOHandler) LinkIdentity(c *gin.Context) {
	userID, _ := c.Get("user_id")

	var req struct {
		ProviderKey string `json:"provider_key" binding:"required"`
		RedirectURL string `json:"redirect_url"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	if req.RedirectURL == "" {
		req.RedirectURL = "/settings"
	}

	authURL, _, err := h.ssoService.GenerateAuthURL(req.ProviderKey, req.RedirectURL, "link", userID.(string))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": fmt.Sprintf("Failed to generate auth URL: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"auth_url": authURL,
		},
	})
}

// UnlinkIdentity removes a linked SSO identity from the current user
// @Summary Unlink SSO identity
// @Description Remove a linked SSO identity from the current user's account
// @Tags SSO
// @Produce json
// @Param id path int true "Identity ID"
// @Success 200 {object} gin.H "Identity unlinked"
// @Failure 400 {object} gin.H "Invalid identity ID or unlink error"
// @Router /api/v1/auth/sso/identities/{id} [delete]
// @Security BearerAuth
func (h *SSOHandler) UnlinkIdentity(c *gin.Context) {
	userID, _ := c.Get("user_id")
	identityIDStr := c.Param("id")

	identityID, err := strconv.ParseInt(identityIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid identity id",
		})
		return
	}

	if err := h.ssoService.UnlinkIdentity(userID.(string), identityID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Identity unlinked successfully",
	})
}

// SetPrimaryIdentity sets a specific SSO identity as the primary login method
// @Summary Set primary SSO identity
// @Description Set a specific linked SSO identity as the primary login method for the current user
// @Tags SSO
// @Produce json
// @Param id path int true "Identity ID"
// @Success 200 {object} gin.H "Primary identity updated"
// @Failure 400 {object} gin.H "Invalid identity ID or update error"
// @Router /api/v1/auth/sso/identities/{id}/primary [put]
// @Security BearerAuth
func (h *SSOHandler) SetPrimaryIdentity(c *gin.Context) {
	userID, _ := c.Get("user_id")
	identityIDStr := c.Param("id")

	identityID, err := strconv.ParseInt(identityIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid identity id",
		})
		return
	}

	if err := h.ssoService.SetPrimaryIdentity(userID.(string), identityID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Primary identity updated successfully",
	})
}

// ============================================
// 管理端点（需要管理员权限）
// ============================================

// AdminGetProviders returns all SSO providers with summary info (admin only)
// @Summary List all SSO providers (admin)
// @Description Get all SSO providers with summary information, excluding sensitive oauth_config details
// @Tags Admin SSO
// @Produce json
// @Success 200 {object} gin.H "List of provider summaries"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/sso/providers [get]
// @Security BearerAuth
func (h *SSOHandler) AdminGetProviders(c *gin.Context) {
	providers, err := h.ssoService.GetAllProviders()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get providers",
		})
		return
	}

	// 只返回摘要信息，不含 oauth_config
	type ProviderSummary struct {
		ID              int64  `json:"id"`
		ProviderKey     string `json:"provider_key"`
		ProviderType    string `json:"provider_type"`
		DisplayName     string `json:"display_name"`
		Icon            string `json:"icon"`
		IsEnabled       bool   `json:"is_enabled"`
		AutoCreateUser  bool   `json:"auto_create_user"`
		DisplayOrder    int    `json:"display_order"`
		ShowOnLoginPage bool   `json:"show_on_login_page"`
		CallbackURL     string `json:"callback_url"`
	}

	summaries := make([]ProviderSummary, len(providers))
	for i, p := range providers {
		summaries[i] = ProviderSummary{
			ID:              p.ID,
			ProviderKey:     p.ProviderKey,
			ProviderType:    p.ProviderType,
			DisplayName:     p.DisplayName,
			Icon:            p.Icon,
			IsEnabled:       p.IsEnabled,
			AutoCreateUser:  p.AutoCreateUser,
			DisplayOrder:    p.DisplayOrder,
			ShowOnLoginPage: p.ShowOnLoginPage,
			CallbackURL:     p.CallbackURL,
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    summaries,
	})
}

// AdminGetProvider returns a single SSO provider's details with redacted client_secret (admin only)
// @Summary Get SSO provider details (admin)
// @Description Get detailed information for a single SSO provider with client_secret redacted
// @Tags Admin SSO
// @Produce json
// @Param id path int true "Provider ID"
// @Success 200 {object} gin.H "Provider details"
// @Failure 400 {object} gin.H "Invalid provider ID"
// @Failure 404 {object} gin.H "Provider not found"
// @Router /api/v1/admin/sso/providers/{id} [get]
// @Security BearerAuth
func (h *SSOHandler) AdminGetProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid provider id",
		})
		return
	}

	var provider models.SSOProvider
	if err := h.db.First(&provider, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code":    404,
			"message": "Provider not found",
		})
		return
	}

	// 脱敏 oauth_config 中的 client_secret_encrypted
	pBytes, _ := json.Marshal(provider)
	var pMap map[string]interface{}
	json.Unmarshal(pBytes, &pMap)

	if oauthCfgRaw, ok := pMap["oauth_config"]; ok {
		var oauthCfg map[string]interface{}
		switch v := oauthCfgRaw.(type) {
		case string:
			json.Unmarshal([]byte(v), &oauthCfg)
		case map[string]interface{}:
			oauthCfg = v
		}
		if oauthCfg != nil {
			if _, exists := oauthCfg["client_secret_encrypted"]; exists {
				oauthCfg["client_secret_encrypted"] = "******"
			}
			pMap["oauth_config"] = oauthCfg
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    pMap,
	})
}

// AdminCreateProvider creates a new SSO provider configuration (admin only)
// @Summary Create SSO provider (admin)
// @Description Create a new SSO provider configuration with OAuth settings
// @Tags Admin SSO
// @Accept json
// @Produce json
// @Param request body object true "Provider configuration"
// @Success 201 {object} gin.H "Provider created"
// @Failure 400 {object} gin.H "Invalid provider data"
// @Router /api/v1/admin/sso/providers [post]
// @Security BearerAuth
func (h *SSOHandler) AdminCreateProvider(c *gin.Context) {
	// 使用 map 接收，因为 oauth_config 可能是字符串或对象
	var raw map[string]interface{}
	if err := c.ShouldBindJSON(&raw); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 处理 oauth_config：如果是字符串则转为 json.RawMessage
	if oauthCfg, ok := raw["oauth_config"]; ok {
		switch v := oauthCfg.(type) {
		case string:
			raw["oauth_config"] = json.RawMessage(v)
		}
	}

	// 序列化再反序列化到结构体
	jsonBytes, _ := json.Marshal(raw)
	var provider models.SSOProvider
	if err := json.Unmarshal(jsonBytes, &provider); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "Invalid provider data: " + err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	provider.CreatedBy = userID.(string)

	if err := h.ssoService.CreateProvider(&provider); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 返回脱敏的 Provider 信息（不含 oauth_config 敏感字段）
	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "Provider created successfully",
		"data": gin.H{
			"id":                 provider.ID,
			"provider_key":       provider.ProviderKey,
			"provider_type":      provider.ProviderType,
			"display_name":       provider.DisplayName,
			"icon":               provider.Icon,
			"is_enabled":         provider.IsEnabled,
			"auto_create_user":   provider.AutoCreateUser,
			"callback_url":       provider.CallbackURL,
			"display_order":      provider.DisplayOrder,
			"show_on_login_page": provider.ShowOnLoginPage,
		},
	})
}

// AdminUpdateProvider updates an existing SSO provider configuration (admin only)
// @Summary Update SSO provider (admin)
// @Description Update an existing SSO provider configuration. Only whitelisted fields are accepted.
// @Tags Admin SSO
// @Accept json
// @Produce json
// @Param id path int true "Provider ID"
// @Param request body object true "Fields to update"
// @Success 200 {object} gin.H "Provider updated"
// @Failure 400 {object} gin.H "Invalid request or no valid fields"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/sso/providers/{id} [put]
// @Security BearerAuth
func (h *SSOHandler) AdminUpdateProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid provider id",
		})
		return
	}

	var updates map[string]interface{}
	if err := c.ShouldBindJSON(&updates); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	// 使用白名单限制可更新字段
	allowedFields := map[string]bool{
		"provider_key":          true,
		"provider_type":         true,
		"display_name":          true,
		"description":           true,
		"icon":                  true,
		"oauth_config":          true,
		"authorize_endpoint":    true,
		"token_endpoint":        true,
		"userinfo_endpoint":     true,
		"callback_url":          true,
		"allowed_callback_urls": true,
		"auto_create_user":      true,
		"default_role":          true,
		"allowed_domains":       true,
		"attribute_mapping":     true,
		"is_enabled":            true,
		"display_order":         true,
		"show_on_login_page":    true,
	}

	sanitized := make(map[string]interface{})
	for k, v := range updates {
		if allowedFields[k] {
			sanitized[k] = v
		}
	}

	if len(sanitized) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "no valid fields to update",
		})
		return
	}

	if err := h.ssoService.UpdateProvider(id, sanitized); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Provider updated successfully",
	})
}

// AdminDeleteProvider deletes an SSO provider configuration (admin only)
// @Summary Delete SSO provider (admin)
// @Description Delete an SSO provider configuration by ID
// @Tags Admin SSO
// @Produce json
// @Param id path int true "Provider ID"
// @Success 200 {object} gin.H "Provider deleted"
// @Failure 400 {object} gin.H "Invalid provider ID"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/sso/providers/{id} [delete]
// @Security BearerAuth
func (h *SSOHandler) AdminDeleteProvider(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "invalid provider id",
		})
		return
	}

	if err := h.ssoService.DeleteProvider(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Provider deleted successfully",
	})
}

// AdminGetSSOConfig returns the global SSO configuration (admin only)
// @Summary Get SSO global config (admin)
// @Description Get the global SSO configuration including local login settings
// @Tags Admin SSO
// @Produce json
// @Success 200 {object} gin.H "SSO global configuration"
// @Router /api/v1/admin/sso/config [get]
// @Security BearerAuth
func (h *SSOHandler) AdminGetSSOConfig(c *gin.Context) {
	config := h.ssoService.GetSSOConfig()
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    config,
	})
}

// AdminUpdateSSOConfig updates the global SSO configuration (admin only)
// @Summary Update SSO global config (admin)
// @Description Update the global SSO configuration settings
// @Tags Admin SSO
// @Accept json
// @Produce json
// @Param request body sso.SSOConfig true "SSO configuration"
// @Success 200 {object} gin.H "SSO config updated"
// @Failure 400 {object} gin.H "Invalid request parameters"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/sso/config [put]
// @Security BearerAuth
func (h *SSOHandler) AdminUpdateSSOConfig(c *gin.Context) {
	var config sso.SSOConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": err.Error(),
		})
		return
	}

	userID, _ := c.Get("user_id")
	if err := h.ssoService.UpdateSSOConfig(&config, userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SSO config updated successfully",
		"data":    config,
	})
}

// AdminGetLoginLogs returns paginated SSO login logs (admin only)
// @Summary Get SSO login logs (admin)
// @Description Get paginated SSO login logs with optional provider filter
// @Tags Admin SSO
// @Produce json
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size (max 100)" default(20)
// @Param provider_key query string false "Filter by provider key"
// @Success 200 {object} gin.H "Paginated login logs"
// @Failure 500 {object} gin.H "Internal server error"
// @Router /api/v1/admin/sso/logs [get]
// @Security BearerAuth
func (h *SSOHandler) AdminGetLoginLogs(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	providerKey := c.Query("provider_key")

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	logs, total, err := h.ssoService.GetLoginLogs(page, pageSize, providerKey)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to get login logs",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data": gin.H{
			"items":     logs,
			"total":     total,
			"page":      page,
			"page_size": pageSize,
		},
	})
}
