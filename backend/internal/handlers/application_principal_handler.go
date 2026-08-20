package handlers

import (
	"net/http"
	"time"

	"iac-platform/internal/application/service"
	"iac-platform/internal/domain/valueobject"

	"github.com/gin-gonic/gin"
)

// ApplicationPrincipalHandler Application 作为 IAM 主体时的自服务 API（选项 A）
type ApplicationPrincipalHandler struct {
	checker service.PermissionChecker
}

// NewApplicationPrincipalHandler 创建
func NewApplicationPrincipalHandler(checker service.PermissionChecker) *ApplicationPrincipalHandler {
	return &ApplicationPrincipalHandler{checker: checker}
}

// WhoAmI 返回当前 Application principal 上下文
// @Summary Application whoami
// @Tags Application-Principal
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Security AppKeyAuth
// @Router /api/v1/app/whoami [get]
func (h *ApplicationPrincipalHandler) WhoAmI(c *gin.Context) {
	pt, _ := c.Get("principal_type")
	pid, _ := c.Get("principal_id")
	uid, _ := c.Get("user_id")
	org, _ := c.Get("auth_org_id")
	appID, _ := c.Get("application_id")
	name, _ := c.Get("username")

	c.JSON(http.StatusOK, gin.H{
		"principal_type":  pt,
		"principal_id":    pid,
		"user_id":         uid,
		"auth_org_id":     org,
		"application_id":  appID,
		"username":        name,
		"auth_mode":       "application",
		"timestamp":       time.Now(),
	})
}

// CheckPermission 以 Application principal 检查权限
// @Summary Check permission as Application
// @Tags Application-Principal
// @Accept json
// @Produce json
// @Param request body object true "resource_type, scope_type, scope_id, required_level"
// @Success 200 {object} service.CheckPermissionResult
// @Security AppKeyAuth
// @Router /api/v1/app/permissions/check [post]
func (h *ApplicationPrincipalHandler) CheckPermission(c *gin.Context) {
	if h.checker == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "permission checker not configured"})
		return
	}

	var req struct {
		ResourceType  string `json:"resource_type" binding:"required"`
		ScopeType     string `json:"scope_type" binding:"required"`
		ScopeID       string `json:"scope_id" binding:"required"`
		RequiredLevel string `json:"required_level" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, pt, pid, ok := principalFromAppContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "application principal missing"})
		return
	}

	resourceType, err := valueobject.ParseResourceType(req.ResourceType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	scopeType, err := valueobject.ParseScopeType(req.ScopeType)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	level, err := valueobject.ParsePermissionLevel(req.RequiredLevel)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	result, err := h.checker.CheckPermission(c.Request.Context(), &service.CheckPermissionRequest{
		UserID:        userID,
		PrincipalType: pt,
		PrincipalID:   pid,
		ResourceType:  resourceType,
		ScopeType:     scopeType,
		ScopeIDStr:    req.ScopeID,
		RequiredLevel: level,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

func principalFromAppContext(c *gin.Context) (userID string, pt valueobject.PrincipalType, pid string, ok bool) {
	raw, exists := c.Get("user_id")
	if !exists {
		return "", "", "", false
	}
	userID, _ = raw.(string)
	if userID == "" {
		return "", "", "", false
	}
	pt = valueobject.PrincipalTypeApplication
	if v, exists := c.Get("principal_type"); exists {
		if s, ok2 := v.(string); ok2 && s != "" {
			if parsed, err := valueobject.ParsePrincipalType(s); err == nil {
				pt = parsed
			}
		}
	}
	if v, exists := c.Get("principal_id"); exists {
		if s, ok2 := v.(string); ok2 {
			pid = s
		}
	}
	if pid == "" {
		pid = userID
	}
	return userID, pt, pid, true
}
