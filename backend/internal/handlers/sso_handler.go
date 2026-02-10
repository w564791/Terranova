package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/config"
	"iac-platform/internal/models"
	"iac-platform/internal/services/sso"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
)

// SSOHandler SSO 相关的 HTTP 处理器
type SSOHandler struct {
	db         *gorm.DB
	ssoService *sso.SSOService
}

// NewSSOHandler 创建 SSO Handler
func NewSSOHandler(db *gorm.DB) *SSOHandler {
	return &SSOHandler{
		db:         db,
		ssoService: sso.NewSSOService(db),
	}
}

// ============================================
// 公开端点（无需认证）
// ============================================

// GetProviders 获取可用的 SSO Provider 列表（登录页展示用）
// 同时返回 SSO 全局配置（如是否禁用本地登录）
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

// Login 发起 SSO 登录（重定向到 Provider 授权页面）
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

// Callback 处理 SSO 回调
func (h *SSOHandler) Callback(c *gin.Context) {
	providerKey := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	if code == "" {
		// 检查是否有错误
		errMsg := c.Query("error")
		errDesc := c.Query("error_description")
		if errMsg != "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    400,
				"message": fmt.Sprintf("SSO error: %s - %s", errMsg, errDesc),
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"code":    400,
			"message": "authorization code is required",
		})
		return
	}

	if state == "" {
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
		c.JSON(http.StatusUnauthorized, gin.H{
			"code":    401,
			"message": err.Error(),
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

	token, err := generateJWTWithSession(result.User.ID, result.User.Username, result.User.Role, sessionID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code":    500,
			"message": "Failed to generate token",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "SSO login successful",
		"data": gin.H{
			"token":       token,
			"expires_at":  expiresAt,
			"is_new_user": result.IsNewUser,
			"user": gin.H{
				"id":       result.User.ID,
				"username": result.User.Username,
				"email":    result.User.Email,
				"role":     result.User.Role,
			},
		},
	})
}

// CallbackRedirect 处理 SSO 回调（重定向模式，Provider 直接重定向到此端点）
// 处理完成后重定向到前端页面，通过 URL 参数传递 token
func (h *SSOHandler) CallbackRedirect(c *gin.Context) {
	providerKey := c.Param("provider")
	code := c.Query("code")
	state := c.Query("state")

	// 默认前端回调页面
	frontendCallbackURL := "/sso/callback"

	if code == "" {
		errMsg := c.Query("error")
		errDesc := c.Query("error_description")
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=%s&error_description=%s", frontendCallbackURL, errMsg, errDesc))
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
		c.Redirect(http.StatusFound, fmt.Sprintf("%s?error=sso_failed&error_description=%s", frontendCallbackURL, err.Error()))
		return
	}

	// 生成 JWT
	sessionID, _ := generateSessionID()
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
	h.db.Create(&session)

	token, _ := generateJWTWithSession(result.User.ID, result.User.Username, result.User.Role, sessionID)

	// 重定向到前端，携带 token
	c.Redirect(http.StatusFound, fmt.Sprintf("%s?token=%s&is_new_user=%v", frontendCallbackURL, token, result.IsNewUser))
}

// ============================================
// 需要认证的端点
// ============================================

// GetIdentities 获取当前用户绑定的身份列表
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

// LinkIdentity 发起绑定新的 SSO 身份
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

// UnlinkIdentity 解绑 SSO 身份
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

// SetPrimaryIdentity 设置主要登录方式
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

// AdminGetProviders 获取所有 Provider 列表（仅摘要信息，不含 oauth_config 等详情）
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

// AdminGetProvider 获取单个 Provider 详情（脱敏 client_secret）
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

// AdminCreateProvider 创建 Provider 配置
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

	c.JSON(http.StatusCreated, gin.H{
		"code":    201,
		"message": "Provider created successfully",
		"data":    provider,
	})
}

// AdminUpdateProvider 更新 Provider 配置
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

	// 不允许更新的字段
	delete(updates, "id")
	delete(updates, "created_at")
	delete(updates, "created_by")

	if err := h.ssoService.UpdateProvider(id, updates); err != nil {
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

// AdminDeleteProvider 删除 Provider 配置
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

// AdminGetSSOConfig 获取 SSO 全局配置
func (h *SSOHandler) AdminGetSSOConfig(c *gin.Context) {
	config := h.ssoService.GetSSOConfig()
	c.JSON(http.StatusOK, gin.H{
		"code":    200,
		"message": "Success",
		"data":    config,
	})
}

// AdminUpdateSSOConfig 更新 SSO 全局配置
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

// AdminGetLoginLogs 获取 SSO 登录日志
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

// ============================================
// 辅助函数（复用 auth.go 中的函数）
// ============================================

// generateSSOJWT 生成 SSO 登录的 JWT（复用 auth.go 中的逻辑）
func generateSSOJWT(userID string, username, role, sessionID string) (string, error) {
	claims := jwt.MapClaims{
		"user_id":    userID,
		"username":   username,
		"role":       role,
		"session_id": sessionID,
		"type":       "login_token",
		"exp":        time.Now().Add(24 * time.Hour).Unix(),
		"iat":        time.Now().Unix(),
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.GetJWTSecret()))
}
