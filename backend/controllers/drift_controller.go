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
// @Summary 获取 drift 检测配置
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} models.DriftConfigResponse
// @Router /api/workspaces/{id}/drift-config [get]
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
// @Summary 更新 drift 检测配置
// @Tags Drift
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param config body models.DriftConfigUpdateRequest true "Drift 配置"
// @Success 200 {object} map[string]string
// @Router /api/workspaces/{id}/drift-config [put]
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
// @Summary 获取 drift 检测状态
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} models.WorkspaceDriftResult
// @Router /api/workspaces/{id}/drift-status [get]
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
// @Summary 手动触发 drift 检测
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]string
// @Router /api/workspaces/{id}/drift-check [post]
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
// @Summary 获取资源 drift 状态列表
// @Tags Drift
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {array} models.ResourceDriftStatus
// @Router /api/workspaces/{id}/resources-drift [get]
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
// @Summary 取消 drift 检测
// @Tags Drift
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]string
// @Router /api/workspaces/{workspace_id}/drift-check [delete]
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
