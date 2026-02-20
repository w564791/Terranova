package handlers

import (
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

// CreateApplication 创建应用
// @Summary 创建应用
// @Tags IAM-Application
// @Accept json
// @Produce json
// @Param request body service.CreateApplicationRequest true "创建应用请求"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/iam/applications [post]
func (h *ApplicationHandler) CreateApplication(c *gin.Context) {
	var req service.CreateApplicationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	app, secret, err := h.service.CreateApplication(c.Request.Context(), &req, userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"application": app,
		"app_secret":  secret, // 仅此一次返回明文密钥
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
func (h *ApplicationHandler) ListApplications(c *gin.Context) {
	orgIDStr := c.Query("org_id")
	if orgIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "org_id is required"})
		return
	}

	orgID, err := strconv.ParseUint(orgIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid org_id"})
		return
	}

	var isActive *bool
	if isActiveStr := c.Query("is_active"); isActiveStr != "" {
		val := isActiveStr == "true"
		isActive = &val
	}

	apps, err := h.service.ListApplications(c.Request.Context(), uint(orgID), isActive)
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
func (h *ApplicationHandler) GetApplication(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	app, err := h.service.GetApplication(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "application not found"})
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
func (h *ApplicationHandler) UpdateApplication(c *gin.Context) {
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

	if err := h.service.UpdateApplication(c.Request.Context(), uint(id), &req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
func (h *ApplicationHandler) DeleteApplication(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := h.service.DeleteApplication(c.Request.Context(), uint(id)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
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
func (h *ApplicationHandler) RegenerateSecret(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	newSecret, err := h.service.RegenerateSecret(c.Request.Context(), uint(id))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"app_secret": newSecret,
		"message":    "Secret regenerated successfully. Please save it, it will not be shown again.",
	})
}
