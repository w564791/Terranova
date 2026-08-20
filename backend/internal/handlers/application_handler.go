package handlers

import (
	"errors"
	"net/http"
	"strconv"

	"iac-platform/internal/application/service"

	"github.com/gin-gonic/gin"
)

// ApplicationHandler 应用处理器
type ApplicationHandler struct {
	service *service.ApplicationService
}

// NewApplicationHandler 创建应用处理器实例
func NewApplicationHandler(service *service.ApplicationService) *ApplicationHandler {
	return &ApplicationHandler{
		service: service,
	}
}

// authOrgID 从 IAM 中间件写入的 auth_org_id 读取鉴权组织；缺失则 400
func authOrgID(c *gin.Context) (uint, bool) {
	raw, ok := c.Get("auth_org_id")
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "auth org not resolved; pass org_id query or use org-scoped path"})
		return 0, false
	}
	switch v := raw.(type) {
	case uint:
		if v == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth org"})
			return 0, false
		}
		return v, true
	case int:
		if v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth org"})
			return 0, false
		}
		return uint(v), true
	case float64:
		if v <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth org"})
			return 0, false
		}
		return uint(v), true
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid auth org type"})
		return 0, false
	}
}

func mapAppErr(c *gin.Context, err error) {
	if errors.Is(err, service.ErrApplicationNotFound) || errors.Is(err, service.ErrApplicationOrgMismatch) {
		// 统一 404 防跨 org 探测
		c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
		return
	}
	if errors.Is(err, service.ErrApplicationOrgForbidden) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
}

// CreateApplication 创建应用
// @Summary 创建应用
// @Tags IAM-Application
// @Accept json
// @Produce json
// @Param request body service.CreateApplicationRequest true "创建应用请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications [post]
// @Security BearerAuth
func (h *ApplicationHandler) CreateApplication(c *gin.Context) {
	var req service.CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}
	// body.org_id 必须与鉴权 org 一致（A-1）
	if req.OrgID != 0 && req.OrgID != authOrg {
		c.JSON(http.StatusForbidden, gin.H{"error": "org_id does not match authorized organization"})
		return
	}
	req.OrgID = authOrg

	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	app, secret, err := h.service.CreateApplication(c.Request.Context(), &req, userID.(string))
	if err != nil {
		mapAppErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"application": app,
		"app_secret":  secret,
		"message":     "Application created successfully. Please save the app_secret, it will not be shown again.",
	})
}

// ListApplications 获取应用列表
// @Summary 获取应用列表
// @Tags IAM-Application
// @Produce json
// @Param org_id query int true "组织ID"
// @Param is_active query bool false "是否启用"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications [get]
// @Security BearerAuth
func (h *ApplicationHandler) ListApplications(c *gin.Context) {
	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}

	// 若传了 org_id query，必须等于鉴权 org
	if orgIDStr := c.Query("org_id"); orgIDStr != "" {
		orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
			return
		}
		if uint(orgID) != authOrg {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot list applications for another organization"})
			return
		}
	}

	var isActive *bool
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		val := isActiveStr == "true"
		isActive = &val
	}

	apps, err := h.service.ListApplications(c.Request.Context(), authOrg, isActive)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"applications": apps,
		"total":        len(apps),
	})
}

// GetApplication 获取应用详情
// @Summary 获取应用详情
// @Tags IAM-Application
// @Produce json
// @Param id path int true "应用ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id} [get]
// @Security BearerAuth
func (h *ApplicationHandler) GetApplication(c *gin.Context) {
	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	app, err := h.service.GetApplicationInOrg(c.Request.Context(), uint(id), authOrg)
	if err != nil {
		mapAppErr(c, err)
		return
	}

	c.JSON(http.StatusOK, app)
}

// UpdateApplication 更新应用
// @Summary 更新应用
// @Tags IAM-Application
// @Accept json
// @Produce json
// @Param id path int true "应用ID"
// @Param request body service.UpdateApplicationRequest true "更新应用请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id} [put]
// @Security BearerAuth
func (h *ApplicationHandler) UpdateApplication(c *gin.Context) {
	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var req service.UpdateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.service.UpdateApplicationInOrg(c.Request.Context(), uint(id), authOrg, &req); err != nil {
		mapAppErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Application updated successfully"})
}

// DeleteApplication 删除应用
// @Summary 删除应用
// @Tags IAM-Application
// @Produce json
// @Param id path int true "应用ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id} [delete]
// @Security BearerAuth
func (h *ApplicationHandler) DeleteApplication(c *gin.Context) {
	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteApplicationInOrg(c.Request.Context(), uint(id), authOrg); err != nil {
		mapAppErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Application deleted successfully"})
}

// RegenerateSecret 重新生成密钥
// @Summary 重新生成应用密钥
// @Tags IAM-Application
// @Produce json
// @Param id path int true "应用ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications/{id}/regenerate-secret [post]
// @Security BearerAuth
func (h *ApplicationHandler) RegenerateSecret(c *gin.Context) {
	authOrg, ok := authOrgID(c)
	if !ok {
		return
	}
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	newSecret, err := h.service.RegenerateSecretInOrg(c.Request.Context(), uint(id), authOrg)
	if err != nil {
		mapAppErr(c, err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"app_secret": newSecret,
		"message":    "Secret regenerated successfully. Please save it, it will not be shown again.",
	})
}
