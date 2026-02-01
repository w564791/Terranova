package controllers

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ResourceController 资源管理控制器
type ResourceController struct {
	service        *services.ResourceService
	editingService *services.ResourceEditingService
}

// NewResourceController 创建资源控制器
func NewResourceController(db *gorm.DB, streamManager *services.OutputStreamManager) *ResourceController {
	return &ResourceController{
		service:        services.NewResourceService(db, streamManager),
		editingService: services.NewResourceEditingService(db),
	}
}

// ============================================================================
// 资源CRUD
// ============================================================================

// GetResources 获取资源列表（支持分页）
// @Summary 获取资源列表
// @Description 获取工作空间的资源列表，支持分页、搜索和排序
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param search query string false "搜索关键词"
// @Param sort_by query string false "排序字段" default(created_at)
// @Param sort_order query string false "排序方向（asc/desc）" default(desc)
// @Param include_inactive query bool false "包含已删除资源"
// @Success 200 {object} map[string]interface{} "成功返回资源列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/resources [get]
// @Security Bearer
func (c *ResourceController) GetResources(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	// 获取workspace以获取内部ID
	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 解析分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))

	// 验证分页参数
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 10
	}

	// 解析搜索和过滤参数
	search := ctx.Query("search")
	sortBy := ctx.DefaultQuery("sort_by", "created_at")
	sortOrder := ctx.DefaultQuery("sort_order", "desc")
	includeInactive := ctx.Query("include_inactive") == "true"

	// 调用Service层分页方法 (使用语义化ID)
	result, err := c.service.GetResourcesPaginated(
		workspace.WorkspaceID,
		page,
		pageSize,
		search,
		sortBy,
		sortOrder,
		includeInactive,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch resources"})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// AddResource 添加资源
// @Summary 添加资源
// @Description 向工作空间添加新的Terraform资源
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param request body object true "资源信息"
// @Success 201 {object} map[string]interface{} "资源添加成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "添加失败"
// @Router /api/v1/workspaces/{id}/resources [post]
// @Security Bearer
func (c *ResourceController) AddResource(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		ResourceType string                 `json:"resource_type" binding:"required"`
		ResourceName string                 `json:"resource_name" binding:"required"`
		TFCode       map[string]interface{} `json:"tf_code" binding:"required"`
		Variables    map[string]interface{} `json:"variables"`
		Description  string                 `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	resource, err := c.service.AddResource(
		workspace.WorkspaceID,
		req.ResourceType,
		req.ResourceName,
		req.TFCode,
		req.Variables,
		req.Description,
		uid,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message":  "Resource added successfully",
		"resource": resource,
	})
}

// GetResource 获取资源详情
// @Summary 获取资源详情
// @Description 根据ID获取资源的详细信息
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Success 200 {object} map[string]interface{} "成功返回资源详情"
// @Failure 400 {object} map[string]interface{} "无效的资源ID"
// @Failure 404 {object} map[string]interface{} "资源不存在"
// @Router /api/v1/workspaces/{id}/resources/{resource_id} [get]
// @Security Bearer
func (c *ResourceController) GetResource(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	resource, err := c.service.GetResource(uint(resourceID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Resource not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"resource": resource,
	})
}

// UpdateResource 更新资源
// @Summary 更新资源
// @Description 更新资源配置并创建新版本
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "更新信息"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id} [put]
// @Security Bearer
func (c *ResourceController) UpdateResource(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		TFCode        map[string]interface{} `json:"tf_code" binding:"required"`
		Variables     map[string]interface{} `json:"variables"`
		ChangeSummary string                 `json:"change_summary" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	version, err := c.service.UpdateResource(
		uint(resourceID),
		req.TFCode,
		req.Variables,
		req.ChangeSummary,
		uid,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Resource updated successfully",
		"version": version,
	})
}

// DeleteResource 删除资源
// @Summary 删除资源
// @Description 删除指定的资源（软删除）
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的资源ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id} [delete]
// @Security Bearer
func (c *ResourceController) DeleteResource(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	if err := c.service.DeleteResource(uint(resourceID), uid); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Resource deleted successfully",
	})
}

// RestoreResource 恢复已删除的资源
// @Summary 恢复已删除的资源
// @Description 恢复软删除的资源
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Success 200 {object} map[string]interface{} "恢复成功"
// @Failure 400 {object} map[string]interface{} "无效的资源ID"
// @Failure 500 {object} map[string]interface{} "恢复失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/restore [post]
// @Security Bearer
func (c *ResourceController) RestoreResource(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	if err := c.service.RestoreResource(uint(resourceID)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Resource restored successfully",
	})
}

// ============================================================================
// 版本管理
// ============================================================================

// GetResourceVersions 获取资源版本历史
// @Summary 获取资源版本历史
// @Description 获取资源的所有历史版本
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Success 200 {object} map[string]interface{} "成功返回版本列表"
// @Failure 400 {object} map[string]interface{} "无效的资源ID"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/versions [get]
// @Security Bearer
func (c *ResourceController) GetResourceVersions(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	versions, err := c.service.GetResourceVersions(uint(resourceID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch versions"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"versions": versions,
		"total":    len(versions),
	})
}

// GetResourceVersion 获取特定版本详情
// @Summary 获取资源版本详情
// @Description 获取资源指定版本的详细信息
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param version path int true "版本号"
// @Success 200 {object} map[string]interface{} "成功返回版本详情"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "版本不存在"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/versions/{version} [get]
// @Security Bearer
func (c *ResourceController) GetResourceVersion(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	version, err := strconv.Atoi(ctx.Param("version"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version"})
		return
	}

	ver, err := c.service.GetResourceVersion(uint(resourceID), version)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Version not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"version": ver,
	})
}

// RollbackResource 回滚资源到指定版本
// @Summary 回滚资源版本
// @Description 将资源回滚到指定的历史版本
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param version path int true "目标版本号"
// @Success 200 {object} map[string]interface{} "回滚成功"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 500 {object} map[string]interface{} "回滚失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/versions/{version}/rollback [post]
// @Security Bearer
func (c *ResourceController) RollbackResource(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	version, err := strconv.Atoi(ctx.Param("version"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid version"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	newVersion, err := c.service.RollbackResource(uint(resourceID), version, uid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Resource rolled back successfully",
		"version": newVersion,
	})
}

// CompareVersions 对比两个版本
// @Summary 对比资源版本
// @Description 对比资源的两个版本差异
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param from query int true "源版本号"
// @Param to query int true "目标版本号"
// @Success 200 {object} map[string]interface{} "成功返回对比结果"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 500 {object} map[string]interface{} "对比失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/versions/compare [get]
// @Security Bearer
func (c *ResourceController) CompareVersions(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	fromVersion, err := strconv.Atoi(ctx.Query("from"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid from version"})
		return
	}

	toVersion, err := strconv.Atoi(ctx.Query("to"))
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid to version"})
		return
	}

	comparison, err := c.service.CompareVersions(uint(resourceID), fromVersion, toVersion)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, comparison)
}

// ============================================================================
// 快照管理
// ============================================================================

// CreateSnapshot 创建快照
// @Summary 创建资源快照
// @Description 创建工作空间资源的快照
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param request body object true "快照信息"
// @Success 201 {object} map[string]interface{} "快照创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/snapshots [post]
// @Security Bearer
func (c *ResourceController) CreateSnapshot(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SnapshotName string `json:"snapshot_name" binding:"required"`
		Description  string `json:"description"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	snapshot, err := c.service.CreateSnapshot(
		workspace.WorkspaceID,
		req.SnapshotName,
		req.Description,
		uid,
	)

	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message":  "Snapshot created successfully",
		"snapshot": snapshot,
	})
}

// GetSnapshots 获取快照列表
// @Summary 获取快照列表
// @Description 获取工作空间的所有快照
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Success 200 {object} map[string]interface{} "成功返回快照列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/snapshots [get]
// @Security Bearer
func (c *ResourceController) GetSnapshots(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	snapshots, err := c.service.GetSnapshots(workspace.WorkspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch snapshots"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"snapshots": snapshots,
		"total":     len(snapshots),
	})
}

// GetSnapshot 获取快照详情
// @Summary 获取快照详情
// @Description 根据ID获取快照的详细信息
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param snapshot_id path int true "快照ID"
// @Success 200 {object} map[string]interface{} "成功返回快照详情"
// @Failure 400 {object} map[string]interface{} "无效的快照ID"
// @Failure 404 {object} map[string]interface{} "快照不存在"
// @Router /api/v1/workspaces/{id}/snapshots/{snapshot_id} [get]
// @Security Bearer
func (c *ResourceController) GetSnapshot(ctx *gin.Context) {
	snapshotID, err := strconv.ParseUint(ctx.Param("snapshot_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID"})
		return
	}

	snapshot, err := c.service.GetSnapshot(uint(snapshotID))
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Snapshot not found"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"snapshot": snapshot,
	})
}

// RestoreSnapshot 恢复快照
// @Summary 恢复快照
// @Description 将工作空间资源恢复到快照状态
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param snapshot_id path int true "快照ID"
// @Success 200 {object} map[string]interface{} "恢复成功"
// @Failure 400 {object} map[string]interface{} "无效的快照ID"
// @Failure 500 {object} map[string]interface{} "恢复失败"
// @Router /api/v1/workspaces/{id}/snapshots/{snapshot_id}/restore [post]
// @Security Bearer
func (c *ResourceController) RestoreSnapshot(ctx *gin.Context) {
	snapshotID, err := strconv.ParseUint(ctx.Param("snapshot_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	if err := c.service.RestoreSnapshot(uint(snapshotID), uid); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Snapshot restored successfully",
	})
}

// DeleteSnapshot 删除快照
// @Summary 删除快照
// @Description 删除指定的快照
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param snapshot_id path int true "快照ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "无效的快照ID"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/snapshots/{snapshot_id} [delete]
// @Security Bearer
func (c *ResourceController) DeleteSnapshot(ctx *gin.Context) {
	snapshotID, err := strconv.ParseUint(ctx.Param("snapshot_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid snapshot ID"})
		return
	}

	if err := c.service.DeleteSnapshot(uint(snapshotID)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Snapshot deleted successfully",
	})
}

// ============================================================================
// 依赖关系管理
// ============================================================================

// GetResourceDependencies 获取资源依赖关系
// @Summary 获取资源依赖关系
// @Description 获取资源的依赖关系图
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Success 200 {object} map[string]interface{} "成功返回依赖关系"
// @Failure 400 {object} map[string]interface{} "无效的资源ID"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/dependencies [get]
// @Security Bearer
func (c *ResourceController) GetResourceDependencies(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	deps, err := c.service.GetResourceDependencies(uint(resourceID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch dependencies"})
		return
	}

	ctx.JSON(http.StatusOK, deps)
}

// UpdateDependencies 更新资源依赖关系
// @Summary 更新资源依赖关系
// @Description 更新资源的依赖关系配置
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "依赖关系配置"
// @Success 200 {object} map[string]interface{} "更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/dependencies [put]
// @Security Bearer
func (c *ResourceController) UpdateDependencies(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	var req struct {
		DependsOn []uint `json:"depends_on" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.service.UpdateDependencies(workspace.WorkspaceID, uint(resourceID), req.DependsOn); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Dependencies updated successfully",
	})
}

// ============================================================================
// 批量操作
// ============================================================================

// ImportResources 从TF代码导入资源
// @Summary 导入Terraform资源
// @Description 从Terraform代码批量导入资源
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param request body object true "TF代码"
// @Success 200 {object} map[string]interface{} "导入成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "导入失败"
// @Router /api/v1/workspaces/{id}/resources/import [post]
// @Security Bearer
func (c *ResourceController) ImportResources(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		TFCode map[string]interface{} `json:"tf_code" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	count, err := c.service.ImportResourcesFromTF(workspace.WorkspaceID, req.TFCode, uid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Imported %d resources successfully", count),
		"count":   count,
	})
}

// DeployResources 部署选定的资源
// @Summary 部署资源
// @Description 创建Plan任务部署选定的资源
// @Tags Resource
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param request body object true "资源ID列表"
// @Success 201 {object} map[string]interface{} "部署任务创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/resources/deploy [post]
// @Security Bearer
func (c *ResourceController) DeployResources(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		ResourceIDs []uint `json:"resource_ids" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取资源的resource_id列表
	var resources []models.WorkspaceResource
	c.service.GetResourcesByIDs(req.ResourceIDs, &resources)

	if len(resources) == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "No valid resources found"})
		return
	}

	// 构建target列表
	targets := make([]string, len(resources))
	for i, res := range resources {
		targets[i] = res.ResourceID
	}

	// 创建Plan任务（带target）
	task, err := c.service.CreatePlanTaskWithTargets(workspace.WorkspaceID, targets, uid)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message": fmt.Sprintf("Deployment task created for %d resources", len(resources)),
		"task":    task,
		"targets": targets,
	})
}

// ============================================================================
// 编辑协作管理
// ============================================================================

// StartEditing 开始编辑
// @Summary 开始编辑资源
// @Description 开始编辑资源并获取编辑锁
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "会话信息"
// @Success 200 {object} map[string]interface{} "开始编辑成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "开始编辑失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/start [post]
// @Security Bearer
func (c *ResourceController) StartEditing(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	response, err := c.editingService.StartEditing(uint(resourceID), uid, req.SessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    response,
	})
}

// Heartbeat 心跳更新
// @Summary 编辑会话心跳
// @Description 更新编辑会话心跳保持锁定状态
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "会话信息"
// @Success 200 {object} map[string]interface{} "心跳更新成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "心跳更新失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/heartbeat [post]
// @Security Bearer
func (c *ResourceController) Heartbeat(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.editingService.Heartbeat(uint(resourceID), uid, req.SessionID); err != nil {
		// 如果是锁不存在，返回404而不是500
		if errors.Is(err, gorm.ErrRecordNotFound) {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "锁不存在或已被接管"})
			return
		}
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"lock_valid": true,
		},
	})
}

// EndEditing 结束编辑
// @Summary 结束编辑资源
// @Description 结束编辑会话并释放锁
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "会话信息"
// @Success 200 {object} map[string]interface{} "结束编辑成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "结束编辑失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/end [post]
// @Security Bearer
func (c *ResourceController) EndEditing(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SessionID string `json:"session_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.editingService.EndEditing(uint(resourceID), uid, req.SessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "编辑会话已结束",
	})
}

// GetEditingStatus 获取编辑状态
// @Summary 获取编辑状态
// @Description 获取资源的编辑状态和锁定信息
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param session_id query string true "会话ID"
// @Success 200 {object} map[string]interface{} "成功返回编辑状态"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/editing/status [get]
// @Security Bearer
func (c *ResourceController) GetEditingStatus(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	status, err := c.editingService.GetEditingStatus(uint(resourceID), uid, sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data":    status,
	})
}

// SaveDrift 保存草稿
// @Summary 保存编辑草稿
// @Description 保存资源的编辑草稿
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "草稿内容"
// @Success 200 {object} map[string]interface{} "草稿保存成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "保存失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/drift/save [post]
// @Security Bearer
func (c *ResourceController) SaveDrift(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SessionID    string                 `json:"session_id" binding:"required"`
		DriftContent map[string]interface{} `json:"drift_content" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	drift, err := c.editingService.SaveDrift(uint(resourceID), uid, req.SessionID, req.DriftContent)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"drift_id":     drift.ID,
			"base_version": drift.BaseVersion,
			"saved_at":     drift.UpdatedAt,
		},
	})
}

// GetDrift 获取草稿
// @Summary 获取编辑草稿
// @Description 获取资源的编辑草稿
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param session_id query string true "会话ID"
// @Success 200 {object} map[string]interface{} "成功返回草稿"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/drift [get]
// @Security Bearer
func (c *ResourceController) GetDrift(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	drift, hasVersionConflict, err := c.editingService.GetDrift(uint(resourceID), uid, sessionID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if drift == nil {
		ctx.JSON(http.StatusOK, gin.H{
			"success": true,
			"data":    nil,
		})
		return
	}

	// 获取当前版本
	resource, err := c.service.GetResource(uint(resourceID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get resource"})
		return
	}

	currentVersion := 1
	if resource.CurrentVersion != nil {
		currentVersion = resource.CurrentVersion.Version
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"data": gin.H{
			"drift":                drift,
			"current_version":      currentVersion,
			"has_version_conflict": hasVersionConflict,
		},
	})
}

// DeleteDrift 删除草稿
// @Summary 删除编辑草稿
// @Description 删除资源的编辑草稿
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param session_id query string true "会话ID"
// @Success 200 {object} map[string]interface{} "删除成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "删除失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/drift [delete]
// @Security Bearer
func (c *ResourceController) DeleteDrift(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	sessionID := ctx.Query("session_id")
	if sessionID == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "session_id is required"})
		return
	}

	if err := c.editingService.DeleteDrift(uint(resourceID), uid, sessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "草稿已删除",
	})
}

// TakeoverEditing 接管编辑
// @Summary 接管编辑会话
// @Description 强制接管其他用户的编辑会话
// @Tags Resource Editing
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param resource_id path int true "资源ID"
// @Param request body object true "接管信息"
// @Success 200 {object} map[string]interface{} "接管成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 500 {object} map[string]interface{} "接管失败"
// @Router /api/v1/workspaces/{id}/resources/{resource_id}/drift/takeover [post]
// @Security Bearer
func (c *ResourceController) TakeoverEditing(ctx *gin.Context) {
	resourceID, err := strconv.ParseUint(ctx.Param("resource_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid resource ID"})
		return
	}

	userID, _ := ctx.Get("user_id")
	uid := userID.(string)

	var req struct {
		SessionID    string `json:"session_id" binding:"required"`
		OldSessionID string `json:"old_session_id" binding:"required"`
	}

	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.editingService.TakeoverEditing(uint(resourceID), uid, req.SessionID, req.OldSessionID); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "接管成功",
	})
}

// ============================================================================
// HCL导出
// ============================================================================

// ExportResourcesHCL 导出资源为HCL格式
// @Summary 导出资源为HCL格式
// @Description 将工作空间的所有资源导出为HCL格式的Terraform配置文件
// @Tags Resource
// @Accept json
// @Produce text/plain
// @Param id path string true "工作空间ID"
// @Success 200 {string} string "HCL格式的资源配置"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 403 {object} map[string]interface{} "权限不足"
// @Failure 404 {object} map[string]interface{} "工作空间不存在"
// @Failure 500 {object} map[string]interface{} "导出失败"
// @Router /api/v1/workspaces/{id}/resources/export/hcl [get]
// @Security Bearer
func (c *ResourceController) ExportResourcesHCL(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	// 获取workspace以获取内部ID
	var workspace models.Workspace
	err := c.service.GetDB().Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.service.GetDB().Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 导出HCL
	hclContent, err := c.service.ExportResourcesAsHCL(workspace.WorkspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to export resources: %v", err)})
		return
	}

	// 设置响应头，支持文件下载
	filename := fmt.Sprintf("%s-resources.tf", workspace.WorkspaceID)
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	ctx.Header("Content-Type", "text/plain; charset=utf-8")
	ctx.String(http.StatusOK, hclContent)
}
