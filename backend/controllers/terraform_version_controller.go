package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TerraformVersionController Terraform版本控制器
type TerraformVersionController struct {
	service *services.TerraformVersionService
}

// NewTerraformVersionController 创建Terraform版本控制器
func NewTerraformVersionController(db *gorm.DB) *TerraformVersionController {
	return &TerraformVersionController{
		service: services.NewTerraformVersionService(db),
	}
}

// ListTerraformVersions 获取所有Terraform版本
// @Summary 获取Terraform版本列表
// @Description 获取所有已配置的Terraform版本，支持按enabled和deprecated过滤
// @Tags Admin
// @Accept json
// @Produce json
// @Param enabled query boolean false "是否启用"
// @Param deprecated query boolean false "是否弃用"
// @Success 200 {object} models.TerraformVersionListResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions [get]
func (c *TerraformVersionController) ListTerraformVersions(ctx *gin.Context) {
	var enabled *bool
	var deprecated *bool

	if enabledStr := ctx.Query("enabled"); enabledStr != "" {
		val := enabledStr == "true"
		enabled = &val
	}

	if deprecatedStr := ctx.Query("deprecated"); deprecatedStr != "" {
		val := deprecatedStr == "true"
		deprecated = &val
	}

	versions, err := c.service.List(enabled, deprecated)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, models.TerraformVersionListResponse{
		Items: versions,
		Total: len(versions),
	})
}

// GetTerraformVersion 获取单个Terraform版本
// @Summary 获取Terraform版本详情
// @Description 根据ID获取Terraform版本详情
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "版本ID"
// @Success 200 {object} models.TerraformVersion
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions/{id} [get]
func (c *TerraformVersionController) GetTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	version, err := c.service.GetByID(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// CreateTerraformVersion 创建Terraform版本
// @Summary 创建Terraform版本
// @Description 创建新的Terraform版本配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param request body models.CreateTerraformVersionRequest true "创建请求"
// @Success 201 {object} models.TerraformVersion
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions [post]
func (c *TerraformVersionController) CreateTerraformVersion(ctx *gin.Context) {
	var req models.CreateTerraformVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version, err := c.service.Create(&req)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, version)
}

// UpdateTerraformVersion 更新Terraform版本
// @Summary 更新Terraform版本
// @Description 更新Terraform版本配置
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "版本ID"
// @Param request body models.UpdateTerraformVersionRequest true "更新请求"
// @Success 200 {object} models.TerraformVersion
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions/{id} [put]
func (c *TerraformVersionController) UpdateTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	var req models.UpdateTerraformVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version, err := c.service.Update(id, &req)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// GetDefaultVersion 获取默认版本
// @Summary 获取默认Terraform版本
// @Description 获取当前设置的默认Terraform版本
// @Tags Admin
// @Accept json
// @Produce json
// @Success 200 {object} models.TerraformVersion
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions/default [get]
func (c *TerraformVersionController) GetDefaultVersion(ctx *gin.Context) {
	version, err := c.service.GetDefault()
	if err != nil {
		if err.Error() == "no default version configured" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// SetDefaultVersion 设置默认版本
// @Summary 设置默认Terraform版本
// @Description 设置指定版本为默认版本（全局唯一）
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "版本ID"
// @Success 200 {object} models.TerraformVersion
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions/{id}/set-default [post]
func (c *TerraformVersionController) SetDefaultVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	version, err := c.service.SetDefault(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "cannot set disabled version as default" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// DeleteTerraformVersion 删除Terraform版本
// @Summary 删除Terraform版本
// @Description 删除Terraform版本配置（如果有workspace在使用或是默认版本则无法删除）
// @Tags Admin
// @Accept json
// @Produce json
// @Param id path int true "版本ID"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/admin/terraform-versions/{id} [delete]
func (c *TerraformVersionController) DeleteTerraformVersion(ctx *gin.Context) {
	id, err := strconv.Atoi(ctx.Param("id"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid ID"})
		return
	}

	err = c.service.Delete(id)
	if err != nil {
		if err.Error() == "terraform version not found" {
			ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		} else if err.Error() == "version is in use by workspaces" ||
			err.Error() == "cannot delete default version, please set another version as default first" {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else {
			ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		}
		return
	}

	ctx.Status(http.StatusNoContent)
}
