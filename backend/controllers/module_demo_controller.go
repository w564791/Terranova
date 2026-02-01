package controllers

import (
	"net/http"
	"strconv"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ModuleDemoController 模块演示控制器
type ModuleDemoController struct {
	service *services.ModuleDemoService
}

// NewModuleDemoController 创建模块演示控制器实例
func NewModuleDemoController(db *gorm.DB) *ModuleDemoController {
	return &ModuleDemoController{
		service: services.NewModuleDemoService(db),
	}
}

// CreateDemo 创建演示配置
// @Summary 创建模块Demo
// @Description 为指定模块创建新的Demo配置
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param demo body object true "Demo配置信息"
// @Success 200 {object} models.ModuleDemo "创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/modules/{id}/demos [post]
// @Security Bearer
func (c *ModuleDemoController) CreateDemo(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	var req struct {
		Name        string                 `json:"name" binding:"required"`
		Description string                 `json:"description"`
		UsageNotes  string                 `json:"usage_notes"`
		ConfigData  map[string]interface{} `json:"config_data" binding:"required"`
		VersionID   string                 `json:"version_id"` // 关联的模块版本 ID
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取用户ID（从认证中间件）
	var userID *string
	if uid, exists := ctx.Get("user_id"); exists {
		if id, ok := uid.(string); ok {
			userID = &id
		}
	}

	demo := &models.ModuleDemo{
		ModuleID:    uint(moduleID),
		Name:        req.Name,
		Description: req.Description,
		UsageNotes:  req.UsageNotes,
	}

	// 设置版本 ID（如果提供）
	if req.VersionID != "" {
		demo.ModuleVersionID = &req.VersionID
	}

	if err := c.service.CreateDemo(demo, req.ConfigData, userID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 重新加载完整数据
	result, err := c.service.GetDemoByID(demo.ID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// GetDemosByModuleID 获取模块的所有演示配置
// @Summary 获取模块Demo列表
// @Description 获取指定模块的Demo配置。如果不传 version_id，则返回默认版本的 Demo
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param version_id query string false "Module Version ID (modv-xxx)，不传则获取默认版本的 Demo"
// @Success 200 {array} models.ModuleDemo "成功返回Demo列表"
// @Failure 400 {object} map[string]interface{} "无效的模块ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/modules/{id}/demos [get]
// @Security Bearer
func (c *ModuleDemoController) GetDemosByModuleID(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	// 支持按 version_id 过滤
	versionID := ctx.Query("version_id")

	demos, err := c.service.GetDemosByModuleIDAndVersion(uint(moduleID), versionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, demos)
}

// ImportDemos 从指定版本导入 Demo
// @Summary 导入Demo
// @Description 从指定版本导入 Demo 到目标版本
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "模块ID"
// @Param version_id path string true "目标版本ID (modv-xxx)"
// @Param body body object true "导入请求"
// @Success 200 {object} map[string]interface{} "导入成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "导入失败"
// @Router /api/v1/modules/{id}/versions/{version_id}/import-demos [post]
// @Security Bearer
func (c *ModuleDemoController) ImportDemos(ctx *gin.Context) {
	moduleID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid module ID"})
		return
	}

	targetVersionID := ctx.Param("version_id")
	if targetVersionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Target version ID is required"})
		return
	}

	var req struct {
		FromVersionID string `json:"from_version_id" binding:"required"`
		DemoIDs       []uint `json:"demo_ids,omitempty"` // 可选，不传则导入全部
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取用户ID
	var userID *string
	if uid, exists := ctx.Get("user_id"); exists {
		if id, ok := uid.(string); ok {
			userID = &id
		}
	}

	importedCount, err := c.service.ImportDemosFromVersion(uint(moduleID), targetVersionID, req.FromVersionID, req.DemoIDs, userID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message":        "Demos imported successfully",
		"imported_count": importedCount,
	})
}

// GetDemoByID 获取演示配置详情
// @Summary 获取Demo详情
// @Description 根据ID获取Demo配置详情
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Success 200 {object} models.ModuleDemo "成功返回Demo详情"
// @Failure 400 {object} map[string]interface{} "无效的Demo ID"
// @Failure 404 {object} map[string]interface{} "Demo不存在"
// @Router /api/v1/demos/{id} [get]
// @Security Bearer
func (c *ModuleDemoController) GetDemoByID(ctx *gin.Context) {
	demoID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid demo ID"})
		return
	}

	demo, err := c.service.GetDemoByID(uint(demoID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Demo not found"})
		return
	}

	ctx.JSON(http.StatusOK, demo)
}

// UpdateDemo 更新演示配置
// @Summary 更新Demo配置
// @Description 更新Demo配置信息
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Param demo body object true "Demo配置信息"
// @Success 200 {object} models.ModuleDemo "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/demos/{id} [put]
// @Security Bearer
func (c *ModuleDemoController) UpdateDemo(ctx *gin.Context) {
	demoID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid demo ID"})
		return
	}

	var req struct {
		Name          string                 `json:"name"`
		Description   string                 `json:"description"`
		UsageNotes    string                 `json:"usage_notes"`
		ConfigData    map[string]interface{} `json:"config_data" binding:"required"`
		ChangeSummary string                 `json:"change_summary"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取用户ID
	var userID *string
	if uid, exists := ctx.Get("user_id"); exists {
		if id, ok := uid.(string); ok {
			userID = &id
		}
	}

	// 准备更新字段
	updates := make(map[string]interface{})
	if req.Name != "" {
		updates["name"] = req.Name
	}
	if req.Description != "" {
		updates["description"] = req.Description
	}
	if req.UsageNotes != "" {
		updates["usage_notes"] = req.UsageNotes
	}

	changeSummary := req.ChangeSummary
	if changeSummary == "" {
		changeSummary = "Updated configuration"
	}

	if err := c.service.UpdateDemo(uint(demoID), updates, req.ConfigData, changeSummary, userID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回更新后的数据
	result, err := c.service.GetDemoByID(uint(demoID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// DeleteDemo 删除演示配置
// @Summary 删除Demo配置
// @Description 删除指定的Demo配置
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Success 200 {object} map[string]string "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的Demo ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/demos/{id} [delete]
// @Security Bearer
func (c *ModuleDemoController) DeleteDemo(ctx *gin.Context) {
	demoID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid demo ID"})
		return
	}

	if err := c.service.DeleteDemo(uint(demoID)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Demo deleted successfully"})
}

// GetVersionsByDemoID 获取演示配置的所有版本
// @Summary 获取Demo版本列表
// @Description 获取指定Demo的所有历史版本
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Success 200 {array} models.ModuleDemoVersion "成功返回版本列表"
// @Failure 400 {object} map[string]interface{} "无效的Demo ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/demos/{id}/versions [get]
// @Security Bearer
func (c *ModuleDemoController) GetVersionsByDemoID(ctx *gin.Context) {
	demoID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid demo ID"})
		return
	}

	versions, err := c.service.GetVersionsByDemoID(uint(demoID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, versions)
}

// GetVersionByID 获取特定版本详情
// @Summary 获取Demo版本详情
// @Description 根据版本ID获取Demo版本详情
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param versionId path int true "版本ID"
// @Success 200 {object} models.ModuleDemoVersion "成功返回版本详情"
// @Failure 400 {object} map[string]interface{} "无效的版本ID"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Router /api/v1/demo-versions/{versionId} [get]
// @Security Bearer
func (c *ModuleDemoController) GetVersionByID(ctx *gin.Context) {
	versionID, err := strconv.ParseUint(ctx.Param("versionId"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version ID"})
		return
	}

	version, err := c.service.GetVersionByID(uint(versionID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}

	ctx.JSON(http.StatusOK, version)
}

// CompareVersions 对比两个版本
// @Summary 对比Demo版本
// @Description 对比两个Demo版本的差异
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Param version1 query int true "版本1 ID"
// @Param version2 query int true "版本2 ID"
// @Success 200 {object} map[string]interface{} "成功返回对比结果"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "对比失败"
// @Router /api/v1/demos/{id}/compare [get]
// @Security Bearer
func (c *ModuleDemoController) CompareVersions(ctx *gin.Context) {
	version1Str := ctx.Query("version1")
	version2Str := ctx.Query("version2")

	if version1Str == "" || version2Str == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Both version1 and version2 are required"})
		return
	}

	version1, err := strconv.ParseUint(version1Str, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version1 ID"})
		return
	}

	version2, err := strconv.ParseUint(version2Str, 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version2 ID"})
		return
	}

	result, err := c.service.CompareVersions(uint(version1), uint(version2))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// RollbackToVersion 回滚到指定版本
// @Summary 回滚Demo版本
// @Description 将Demo回滚到指定的历史版本
// @Tags Module Demo
// @Accept json
// @Produce json
// @Param id path int true "Demo ID"
// @Param body body object true "回滚请求（包含version_id）"
// @Success 200 {object} models.ModuleDemo "回滚成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "回滚失败"
// @Router /api/v1/demos/{id}/rollback [post]
// @Security Bearer
func (c *ModuleDemoController) RollbackToVersion(ctx *gin.Context) {
	demoID, err := strconv.ParseUint(ctx.Param("id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid demo ID"})
		return
	}

	var req struct {
		VersionID uint `json:"version_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取用户ID
	var userID *string
	if uid, exists := ctx.Get("user_id"); exists {
		if id, ok := uid.(string); ok {
			userID = &id
		}
	}

	if err := c.service.RollbackToVersion(uint(demoID), req.VersionID, userID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// 返回更新后的数据
	result, err := c.service.GetDemoByID(uint(demoID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, result)
}
