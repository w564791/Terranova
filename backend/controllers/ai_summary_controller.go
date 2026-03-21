package controllers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// AISummaryController AI 摘要 API 控制器
type AISummaryController struct {
	db      *gorm.DB
	service *services.AISummaryService
}

// NewAISummaryController 创建控制器
func NewAISummaryController(db *gorm.DB) *AISummaryController {
	return &AISummaryController{
		db:      db,
		service: services.NewAISummaryService(db),
	}
}

// GetPlanSummary 获取 Plan 阶段摘要
func (c *AISummaryController) GetPlanSummary(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	summary := c.service.GetPlanSummary(uint(taskID))
	if summary == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "plan summary not found"})
		return
	}

	// 校验 workspace 归属，防止越权访问
	if summary.WorkspaceID != workspaceID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "summary does not belong to this workspace"})
		return
	}

	if summary.Status == "running" || summary.Status == "pending" {
		ctx.JSON(http.StatusAccepted, summary)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}

// GetApplySummary 获取 Apply 阶段摘要
func (c *AISummaryController) GetApplySummary(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	summary := c.service.GetApplySummary(uint(taskID))
	if summary == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "apply summary not found"})
		return
	}

	// 校验 workspace 归属，防止越权访问
	if summary.WorkspaceID != workspaceID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "summary does not belong to this workspace"})
		return
	}

	if summary.Status == "running" || summary.Status == "pending" {
		ctx.JSON(http.StatusAccepted, summary)
		return
	}

	ctx.JSON(http.StatusOK, summary)
}

// RetryPlanSummary 重试 Plan Summary
func (c *AISummaryController) RetryPlanSummary(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	if err := c.service.RetryPlanSummary(uint(taskID)); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"message": "重试已触发"})
}

// RetryApplySummary 重试 Apply Summary
func (c *AISummaryController) RetryApplySummary(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	if err := c.service.RetryApplySummary(uint(taskID)); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusAccepted, gin.H{"message": "重试已触发"})
}
