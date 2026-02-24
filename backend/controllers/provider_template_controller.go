package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ProviderTemplateController Provider模板控制器
type ProviderTemplateController struct {
	service *services.ProviderTemplateService
}

// NewProviderTemplateController 创建Provider模板控制器
func NewProviderTemplateController(db *gorm.DB) *ProviderTemplateController {
	return &ProviderTemplateController{
		service: services.NewProviderTemplateService(db),
	}
}

// ListProviderTemplates 获取所有Provider模板
// @Summary 获取Provider模板列表
// @Description 获取所有已配置的Provider模板，支持按enabled和type过滤
// @Tags Admin
// @Accept json
// @Produce json
// @Param enabled query boolean false "是否启用"
// @Param type query string false "Provider类型"
// @Success 200 {object} models.ProviderTemplateListResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates [get]
func (c *ProviderTemplateController) ListProviderTemplates(ctx *gin.Context) {
	var enabled *bool

	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}

	providerType := ctx.Query("type")

	templates, err := c.service.List(enabled, providerType)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"items": templates, "total": len(templates)})
}

// GetProviderTemplate 获取单个Provider模板
// @Summary 获取Provider模板详情
// @Description 根据ID获取Provider模板详情，敏感字段会被过滤
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "模板ID"
// @Success 200 {object} models.ProviderTemplate
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates/{id} [get]
func (c *ProviderTemplateController) GetProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	template, err := c.service.GetByID(uint(id))
	if err != nil {
		if err.Error() == "provider template not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	// 过滤敏感信息
	template.Config = models.JSONB(services.FilterTemplateSensitiveInfo(template.Config))

	ctx.JSON(http.StatusOK, template)
}

// CreateProviderTemplate 创建Provider模板
// @Summary 创建Provider模板
// @Description 创建新的Provider模板配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param request body models.CreateProviderTemplateRequest true "创建请求"
// @Success 201 {object} models.ProviderTemplate
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates [post]
func (c *ProviderTemplateController) CreateProviderTemplate(ctx *gin.Context) {
	var req models.CreateProviderTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template, err := c.service.Create(&req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, template)
}

// UpdateProviderTemplate 更新Provider模板
// @Summary 更新Provider模板
// @Description 更新Provider模板配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "模板ID"
// @Param request body models.UpdateProviderTemplateRequest true "更新请求"
// @Success 200 {object} models.ProviderTemplate
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates/{id} [put]
func (c *ProviderTemplateController) UpdateProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req models.UpdateProviderTemplateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	template, err := c.service.Update(uint(id), &req)
	if err != nil {
		if err.Error() == "provider template not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, template)
}

// SetDefaultTemplate 设置默认Provider模板
// @Summary 设置默认Provider模板
// @Description 设置指定模板为默认模板（同类型唯一）
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "模板ID"
// @Success 200 {object} models.ProviderTemplate
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates/{id}/set-default [post]
func (c *ProviderTemplateController) SetDefaultTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	template, err := c.service.SetDefault(uint(id))
	if err != nil {
		if err.Error() == "provider template not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "cannot set disabled template as default" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, template)
}

// DeleteProviderTemplate 删除Provider模板
// @Summary 删除Provider模板
// @Description 删除Provider模板配置（如果有workspace在使用或是默认模板则无法删除）
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "模板ID"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/provider-templates/{id} [delete]
func (c *ProviderTemplateController) DeleteProviderTemplate(ctx *gin.Context) {
	id, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	err = c.service.Delete(uint(id))
	if err != nil {
		if err.Error() == "provider template not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "template is in use by workspaces" ||
			err.Error() == "cannot delete default template, please set another template as default first" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}
