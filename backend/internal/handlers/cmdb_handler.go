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
// @Summary 搜索资源
// @Description 根据资源ID、名称或描述搜索资源
// @Tags CMDB
// @Accept json
// @Produce json
// @Param q query string true "搜索关键词"
// @Param workspace_id query string false "限定workspace"
// @Param resource_type query string false "限定资源类型"
// @Param limit query int false "返回数量限制" default(20)
// @Success 200 {object} map[string]interface{}
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
// @Summary 获取workspace资源树
// @Description 获取指定workspace的资源树状结构
// @Tags CMDB
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} models.WorkspaceResourceTree
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
// @Summary 获取资源详情
// @Description 获取指定资源的详细信息
// @Tags CMDB
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param address query string true "Terraform地址"
// @Success 200 {object} models.ResourceIndex
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

// GetCMDBStats 获取CMDB统计信息
// @Summary 获取CMDB统计信息
// @Description 获取CMDB的整体统计信息
// @Tags CMDB
// @Accept json
// @Produce json
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
// @Summary 同步workspace资源索引
// @Description 手动触发同步指定workspace的资源索引
// @Tags CMDB
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
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
// @Summary 同步所有workspace资源索引
// @Description 手动触发同步所有workspace的资源索引（异步执行）
// @Tags CMDB
// @Accept json
// @Produce json
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
// @Summary 获取所有资源类型
// @Description 获取CMDB中所有的资源类型列表
// @Tags CMDB
// @Accept json
// @Produce json
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
// @Summary 获取所有workspace的资源数量
// @Description 获取CMDB中所有workspace的资源数量统计
// @Tags CMDB
// @Accept json
// @Produce json
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
// @Summary 获取搜索建议
// @Description 根据输入前缀返回匹配的资源ID、名称或描述建议
// @Tags CMDB
// @Accept json
// @Produce json
// @Param q query string true "搜索前缀"
// @Param limit query int false "返回数量限制" default(10)
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
