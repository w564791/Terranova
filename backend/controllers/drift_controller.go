package controllers

import (
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// DriftController Drift 检测控制器
type DriftController struct {
	db             *gorm.DB
	driftService   *services.DriftCheckService
	driftScheduler *services.DriftCheckScheduler
}

// NewDriftController 创建 Drift 检测控制器
func NewDriftController(db *gorm.DB, scheduler *services.DriftCheckScheduler) *DriftController {
	return &DriftController{
		db:             db,
		driftService:   services.NewDriftCheckService(db),
		driftScheduler: scheduler,
	}
}

// GetDriftConfig 获取 workspace 的 drift 检测配置
// @Summary Get drift detection config
// @Description Get drift detection configuration for a workspace
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} models.DriftConfigResponse
// @Failure 404 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/drift-config [get]
// @Security BearerAuth
func (c *DriftController) GetDriftConfig(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	config, err := c.driftService.GetDriftConfig(workspaceID)
	if err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, config)
}

// UpdateDriftConfig 更新 workspace 的 drift 检测配置
// @Summary Update drift detection config
// @Description Update drift detection configuration for a workspace
// @Tags Drift
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param config body models.DriftConfigUpdateRequest true "Drift config"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/drift-config [put]
// @Security BearerAuth
func (c *DriftController) UpdateDriftConfig(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	var req models.DriftConfigUpdateRequest
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := c.driftService.UpdateDriftConfigFull(workspaceID, &req); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Drift config updated successfully"})
}

// GetDriftStatus 获取 workspace 的 drift 检测状态
// @Summary Get drift detection status
// @Description Get drift detection status and latest result for a workspace
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} models.WorkspaceDriftResult
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/drift-status [get]
// @Security BearerAuth
func (c *DriftController) GetDriftStatus(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	result, err := c.driftService.GetDriftResult(workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if result == nil {
		// 没有检测结果，返回默认状态
		ctx.JSON(http.StatusOK, gin.H{
			"workspace_id":  workspaceID,
			"has_drift":     false,
			"drift_count":   0,
			"check_status":  "pending",
			"last_check_at": nil,
		})
		return
	}

	ctx.JSON(http.StatusOK, result)
}

// TriggerDriftCheck 手动触发 drift 检测
// @Summary Trigger drift check
// @Description Manually trigger a drift detection check for a workspace
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]string
// @Failure 400 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/drift-check [post]
// @Security BearerAuth
func (c *DriftController) TriggerDriftCheck(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	// 使用 DriftCheckService 的手动触发方法
	if err := c.driftService.TriggerManualDriftCheck(workspaceID); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{"message": "Drift check triggered successfully"})
}

// GetResourceDriftStatuses 获取 workspace 下所有资源的 drift 状态
// @Summary Get resource drift statuses
// @Description Get drift statuses for all resources in a workspace
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} models.ResourceDriftStatus
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/resources-drift [get]
// @Security BearerAuth
func (c *DriftController) GetResourceDriftStatuses(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	statuses, err := c.driftService.GetResourceDriftStatuses(workspaceID)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, statuses)
}

// CancelDriftCheck 取消正在进行的 drift 检测
// @Summary Cancel drift check
// @Description Cancel an ongoing drift detection check
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]string
// @Failure 500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/drift-check [delete]
// @Security BearerAuth
func (c *DriftController) CancelDriftCheck(ctx *gin.Context) {
	workspaceID := ctx.Param("id")

	// 取消 pending/running 的 drift_check 任务
	result := c.db.Model(&models.WorkspaceTask{}).
		Where("workspace_id = ? AND task_type = ? AND status IN ?", workspaceID,
			models.TaskTypeDriftCheck,
			[]string{"pending", "running"}).
		Updates(map[string]interface{}{
			"status":        models.TaskStatusCancelled,
			"error_message": "Cancelled by user",
		})

	if result.Error != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": result.Error.Error()})
		return
	}

	// 更新 drift 状态
	c.driftService.UpdateDriftStatus(workspaceID, models.DriftCheckStatusFailed, "Cancelled by user")

	ctx.JSON(http.StatusOK, gin.H{
		"message":       "Drift check cancelled",
		"tasks_updated": result.RowsAffected,
	})
}
