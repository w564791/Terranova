package handlers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// CMDBHandler CMDB处理器
type CMDBHandler struct {
	cmdbService *services.CMDBService
}

// NewCMDBHandler 创建CMDB处理器
func NewCMDBHandler(cmdbService *services.CMDBService) *CMDBHandler {
	return &CMDBHandler{
		cmdbService: cmdbService,
	}
}

// SearchResources 搜索资源
// @Summary Search resources
// @Description Search resources by resource ID, name, or description
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param q query string true "Search keyword"
// @Param workspace_id query string false "Filter by workspace"
// @Param resource_type query string false "Filter by resource type"
// @Param limit query int false "Result limit" default(20)
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/cmdb/search [get]
func (h *CMDBHandler) SearchResources(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "搜索关键词不能为空"})
		return
	}

	workspaceID := c.Query("workspace_id")
	resourceType := c.Query("resource_type")
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))

	results, err := h.cmdbService.SearchResources(query, workspaceID, resourceType, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

// GetWorkspaceResourceTree 获取workspace资源树
// @Summary Get workspace resource tree
// @Description Get the resource tree structure for a specified workspace
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} models.WorkspaceResourceTree
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/cmdb/workspaces/{workspace_id}/tree [get]
func (h *CMDBHandler) GetWorkspaceResourceTree(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id不能为空"})
		return
	}

	tree, err := h.cmdbService.GetWorkspaceResourceTree(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, tree)
}

// GetResourceDetail 获取资源详情
// @Summary Get resource detail
// @Description Get detailed information for a specified resource
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param workspace_id path string true "Workspace ID"
// @Param address query string true "Terraform address"
// @Success 200 {object} models.ResourceIndex
// @Failure 400 {object} map[string]interface{}
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/cmdb/workspaces/{workspace_id}/resources [get]
func (h *CMDBHandler) GetResourceDetail(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	address := c.Query("address")

	if workspaceID == "" || address == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id和address不能为空"})
		return
	}

	resource, err := h.cmdbService.GetResourceDetail(workspaceID, address)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "资源不存在"})
		return
	}

	c.JSON(http.StatusOK, resource)
}

// GetCMDBOverview 获取 CMDB 观测面板数据
// @Summary Get CMDB overview dashboard data
// @Description Get CMDB observation dashboard data including stats and trends
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/overview [get]
func (h *CMDBHandler) GetCMDBOverview(c *gin.Context) {
	overview, err := h.cmdbService.GetCMDBOverview()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, overview)
}

// GetSyncHistory 获取同步历史（分页）
// @Summary Get sync history
// @Description Get paginated CMDB sync history
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param page query int false "Page number" default(1)
// @Param size query int false "Page size" default(10)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/sync-history [get]
func (h *CMDBHandler) GetSyncHistory(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("size", "10"))

	result, err := h.cmdbService.GetSyncHistory(page, size)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetSearchAnalytics 获取搜索召回质量分析数据
// @Summary Get search analytics
// @Description Get search recall quality analysis data
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param period query string false "Time period" default(7d)
// @Param source query string false "Search source" default(manual)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/search-analytics [get]
func (h *CMDBHandler) GetSearchAnalytics(c *gin.Context) {
	period := c.DefaultQuery("period", "7d")
	source := c.DefaultQuery("source", "manual")

	analytics, err := h.cmdbService.GetSearchAnalytics(period, source)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, analytics)
}

// GetCMDBStats 获取CMDB统计信息
// @Summary Get CMDB statistics
// @Description Get overall CMDB statistics
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Success 200 {object} models.CMDBStats
// @Router /api/v1/cmdb/stats [get]
func (h *CMDBHandler) GetCMDBStats(c *gin.Context) {
	stats, err := h.cmdbService.GetCMDBStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// SyncWorkspace 同步workspace资源索引
// @Summary Sync workspace resource index
// @Description Manually trigger sync for a specified workspace's resource index
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/cmdb/workspaces/{workspace_id}/sync [post]
func (h *CMDBHandler) SyncWorkspace(c *gin.Context) {
	workspaceID := c.Param("workspace_id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id不能为空"})
		return
	}

	if err := h.cmdbService.SyncWorkspaceResources(workspaceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "同步成功",
		"workspace_id": workspaceID,
	})
}

// SyncAllWorkspaces 同步所有workspace资源索引
// @Summary Sync all workspace resource indexes
// @Description Manually trigger sync for all workspace resource indexes (async)
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Success 202 {object} map[string]interface{}
// @Router /api/v1/cmdb/sync-all [post]
func (h *CMDBHandler) SyncAllWorkspaces(c *gin.Context) {
	// 异步执行同步，避免阻塞请求
	go func() {
		if err := h.cmdbService.SyncAllWorkspaces(); err != nil {
			// 记录错误日志
			println("CMDB sync-all failed:", err.Error())
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"message": "全量同步任务已启动，将在后台执行",
		"status":  "accepted",
	})
}

// GetResourceTypes 获取所有资源类型
// @Summary Get all resource types
// @Description Get the list of all resource types in CMDB
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/resource-types [get]
func (h *CMDBHandler) GetResourceTypes(c *gin.Context) {
	stats, err := h.cmdbService.GetCMDBStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"resource_types": stats.ResourceTypeStats,
	})
}

// GetWorkspaceResourceCounts 获取所有workspace的资源数量
// @Summary Get workspace resource counts
// @Description Get resource count statistics for all workspaces in CMDB
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/workspace-counts [get]
func (h *CMDBHandler) GetWorkspaceResourceCounts(c *gin.Context) {
	counts, err := h.cmdbService.GetWorkspaceResourceCounts()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"counts": counts,
	})
}

// GetSearchSuggestions 获取搜索建议
// @Summary Get search suggestions
// @Description Return matching resource ID, name, or description suggestions based on input prefix
// @Tags CMDB
// @Produce json
// @Security BearerAuth
// @Param q query string true "Search prefix"
// @Param limit query int false "Result limit" default(10)
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/cmdb/suggestions [get]
func (h *CMDBHandler) GetSearchSuggestions(c *gin.Context) {
	prefix := c.Query("q")
	if prefix == "" {
		c.JSON(http.StatusOK, gin.H{
			"suggestions": []interface{}{},
		})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	suggestions, err := h.cmdbService.GetSearchSuggestions(prefix, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"suggestions": suggestions,
	})
}
