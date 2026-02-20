package controllers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModuleVersionController 模块版本控制器
type ModuleVersionController struct {
	service *services.ModuleVersionService
}

// NewModuleVersionController 创建模块版本控制器
func NewModuleVersionController(db *gorm.DB) *ModuleVersionController {
	return &ModuleVersionController{
		service: services.NewModuleVersionService(db),
	}
}

// ListVersions 获取模块的所有版本
// @Summary 获取模块版本列表
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(20)
// @Success 200 {object} services.ModuleVersionListResponse
// @Router /api/v1/modules/{id}/versions [get]
func (c *ModuleVersionController) ListVersions(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "20"))

	result, err := c.service.ListVersions(uint(moduleID), page, pageSize)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// GetVersion 获取版本详情
// @Summary 获取模块版本详情
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Version ID"
// @Success 200 {object} models.ModuleVersion
// @Router /api/v1/modules/{id}/versions/{version_id} [get]
func (c *ModuleVersionController) GetVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	version, err := c.service.GetVersion(uint(moduleID), versionID)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// GetDefaultVersion 获取默认版本
// @Summary 获取模块的默认版本
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Success 200 {object} models.ModuleVersion
// @Router /api/v1/modules/{id}/default-version [get]
func (c *ModuleVersionController) GetDefaultVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	version, err := c.service.GetDefaultVersion(uint(moduleID))
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "No default version found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// CreateVersion 创建新版本
// @Summary 创建模块新版本
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param body body services.CreateModuleVersionRequest true "创建请求"
// @Success 201 {object} models.ModuleVersion
// @Router /api/v1/modules/{id}/versions [post]
func (c *ModuleVersionController) CreateVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	var req services.CreateModuleVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID := ctx.GetString("user_id")
	version, err := c.service.CreateVersion(uint(moduleID), &req, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, version)
}

// UpdateVersion 更新版本信息
// @Summary 更新模块版本信息
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Version ID"
// @Param body body services.UpdateModuleVersionRequest true "更新请求"
// @Success 200 {object} models.ModuleVersion
// @Router /api/v1/modules/{id}/versions/{version_id} [put]
func (c *ModuleVersionController) UpdateVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	var req services.UpdateModuleVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	version, err := c.service.UpdateVersion(uint(moduleID), versionID, &req)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// DeleteVersion 删除版本
// @Summary 删除模块版本
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Version ID"
// @Success 204
// @Router /api/v1/modules/{id}/versions/{version_id} [delete]
func (c *ModuleVersionController) DeleteVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	if err := c.service.DeleteVersion(uint(moduleID), versionID); err != nil {
		if err == gorm.ErrRecordNotFound {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
			return
		}
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.Status(http.StatusNoContent)
}

// SetDefaultVersion 设置默认版本
// @Summary 设置模块的默认版本
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param body body services.SetDefaultVersionRequest true "设置请求"
// @Success 200 {object} gin.H
// @Router /api/v1/modules/{id}/default-version [put]
func (c *ModuleVersionController) SetDefaultVersion(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	var req services.SetDefaultVersionRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	if err := c.service.SetDefaultVersion(uint(moduleID), req.VersionID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Default version updated successfully"})
}

// InheritDemos 继承 Demos
// @Summary 从其他版本继承 Demos
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Target Version ID"
// @Param body body services.InheritDemosRequest true "继承请求"
// @Success 200 {object} gin.H
// @Router /api/v1/modules/{id}/versions/{version_id}/inherit-demos [post]
func (c *ModuleVersionController) InheritDemos(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	var req services.InheritDemosRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}

	userID := ctx.GetString("user_id")
	count, err := c.service.InheritDemos(uint(moduleID), versionID, &req, userID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":         "Demos inherited successfully",
		"inherited_count": count,
	})
}

// GetVersionSchemas 获取版本的所有 Schema
// @Summary 获取版本的所有 Schema
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Version ID"
// @Success 200 {array} models.Schema
// @Router /api/v1/modules/{id}/versions/{version_id}/schemas [get]
func (c *ModuleVersionController) GetVersionSchemas(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	schemas, err := c.service.GetVersionSchemas(uint(moduleID), versionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, schemas)
}

// GetVersionDemos 获取版本的所有 Demo
// @Summary 获取版本的所有 Demo
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param version_id path string true "Version ID"
// @Success 200 {array} models.ModuleDemo
// @Router /api/v1/modules/{id}/versions/{version_id}/demos [get]
func (c *ModuleVersionController) GetVersionDemos(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	versionID := ctx.Param("version_id")
	if versionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Version ID is required"})
		return
	}

	demos, err := c.service.GetVersionDemos(uint(moduleID), versionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, demos)
}

// CompareVersions 比较两个版本的 Schema 差异
// @Summary 比较两个版本的 Schema 差异
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Param id path int true "Module ID"
// @Param from query string true "源版本 ID"
// @Param to query string true "目标版本 ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/modules/{id}/versions/compare [get]
func (c *ModuleVersionController) CompareVersions(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	fromVersionID := ctx.Query("from")
	toVersionID := ctx.Query("to")

	if fromVersionID == "" || toVersionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Both 'from' and 'to' version IDs are required"})
		return
	}

	result, err := c.service.CompareVersions(uint(moduleID), fromVersionID, toVersionID)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// MigrateExistingModules 迁移现有模块数据
// @Summary 迁移现有模块数据到多版本结构（管理员操作）
// @Tags ModuleVersion
// @Accept json
// @Produce json
// @Success 200 {object} gin.H
// @Router /api/v1/modules/migrate-versions [post]
func (c *ModuleVersionController) MigrateExistingModules(ctx *gin.Context) {
	if err := c.service.MigrateExistingModules(); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Migration completed successfully"})
}
