package handlers

import (
	"context"
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// CMDBExternalSourceHandler 外部CMDB数据源处理器
type CMDBExternalSourceHandler struct {
	db      *gorm.DB
	service *services.CMDBExternalSourceService
}

// NewCMDBExternalSourceHandler 创建外部数据源处理器
func NewCMDBExternalSourceHandler(db *gorm.DB) *CMDBExternalSourceHandler {
	return &CMDBExternalSourceHandler{
		db:      db,
		service: services.NewCMDBExternalSourceService(db),
	}
}

// CreateExternalSource 创建外部数据源
// @Summary 创建外部CMDB数据源
// @Description 创建一个新的外部CMDB数据源配置
// @Tags CMDB External Sources
// @Accept json
// @Produce json
// @Param request body models.CreateExternalSourceRequest true "创建请求"
// @Success 201 {object} models.ExternalSourceResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources [post]
func (h *CMDBExternalSourceHandler) CreateExternalSource(c *gin.Context) {
	var req models.CreateExternalSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "system"
	}

	ctx := context.Background()
	source, err := h.service.CreateExternalSource(ctx, &req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response, err := source.ToResponse()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, response)
}

// ListExternalSources 列出外部数据源
// @Summary 列出所有外部CMDB数据源
// @Description 获取所有外部CMDB数据源配置列表
// @Tags CMDB External Sources
// @Produce json
// @Success 200 {object} models.ExternalSourceListResponse
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources [get]
func (h *CMDBExternalSourceHandler) ListExternalSources(c *gin.Context) {
	ctx := context.Background()
	sources, err := h.service.ListExternalSources(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var responses []models.ExternalSourceResponse
	for _, source := range sources {
		resp, err := source.ToResponse()
		if err != nil {
			continue
		}
		responses = append(responses, *resp)
	}

	c.JSON(http.StatusOK, models.ExternalSourceListResponse{
		Sources: responses,
		Total:   len(responses),
	})
}

// GetExternalSource 获取外部数据源详情
// @Summary 获取外部CMDB数据源详情
// @Description 根据ID获取外部CMDB数据源的详细配置
// @Tags CMDB External Sources
// @Produce json
// @Param source_id path string true "数据源ID"
// @Success 200 {object} models.ExternalSourceResponse
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id} [get]
func (h *CMDBExternalSourceHandler) GetExternalSource(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	ctx := context.Background()
	source, err := h.service.GetExternalSource(ctx, sourceID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	response, err := source.ToResponse()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// UpdateExternalSource 更新外部数据源
// @Summary 更新外部CMDB数据源
// @Description 更新外部CMDB数据源的配置
// @Tags CMDB External Sources
// @Accept json
// @Produce json
// @Param source_id path string true "数据源ID"
// @Param request body models.UpdateExternalSourceRequest true "更新请求"
// @Success 200 {object} models.ExternalSourceResponse
// @Failure 400 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id} [put]
func (h *CMDBExternalSourceHandler) UpdateExternalSource(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	var req models.UpdateExternalSourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户ID
	userID := c.GetString("user_id")
	if userID == "" {
		userID = "system"
	}

	ctx := context.Background()
	source, err := h.service.UpdateExternalSource(ctx, sourceID, &req, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response, err := source.ToResponse()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, response)
}

// DeleteExternalSource 删除外部数据源
// @Summary 删除外部CMDB数据源
// @Description 删除指定的外部CMDB数据源及其同步的数据
// @Tags CMDB External Sources
// @Produce json
// @Param source_id path string true "数据源ID"
// @Success 204 "No Content"
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id} [delete]
func (h *CMDBExternalSourceHandler) DeleteExternalSource(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	ctx := context.Background()
	if err := h.service.DeleteExternalSource(ctx, sourceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.Status(http.StatusNoContent)
}

// SyncExternalSource 同步外部数据源
// @Summary 手动触发外部数据源同步
// @Description 手动触发指定外部CMDB数据源的数据同步
// @Tags CMDB External Sources
// @Produce json
// @Param source_id path string true "数据源ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id}/sync [post]
func (h *CMDBExternalSourceHandler) SyncExternalSource(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	ctx := context.Background()
	if err := h.service.SyncExternalSource(ctx, sourceID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Sync completed successfully"})
}

// TestConnection 测试外部数据源连接
// @Summary 测试外部数据源连接
// @Description 测试外部CMDB数据源的API连接是否正常
// @Tags CMDB External Sources
// @Produce json
// @Param source_id path string true "数据源ID"
// @Success 200 {object} models.TestConnectionResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id}/test [post]
func (h *CMDBExternalSourceHandler) TestConnection(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	ctx := context.Background()
	result, err := h.service.TestConnection(ctx, sourceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, result)
}

// GetSyncLogs 获取同步日志
// @Summary 获取外部数据源同步日志
// @Description 获取指定外部CMDB数据源的同步历史日志
// @Tags CMDB External Sources
// @Produce json
// @Param source_id path string true "数据源ID"
// @Param limit query int false "返回数量限制" default(20)
// @Success 200 {object} models.SyncLogListResponse
// @Failure 400 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /api/v1/cmdb/external-sources/{source_id}/sync-logs [get]
func (h *CMDBExternalSourceHandler) GetSyncLogs(c *gin.Context) {
	sourceID := c.Param("source_id")
	if sourceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_id is required"})
		return
	}

	limit := 20
	if limitStr := c.Query("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	ctx := context.Background()
	logs, err := h.service.GetSyncLogs(ctx, sourceID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var responses []models.SyncLogResponse
	for _, log := range logs {
		responses = append(responses, *log.ToResponse())
	}

	c.JSON(http.StatusOK, models.SyncLogListResponse{
		Logs:  responses,
		Total: len(responses),
	})
}
