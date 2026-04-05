package controllers

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/services"
	"net/http"
	"strconv"
	"strings"
	"time"

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
// @Summary Get plan summary
// @Description Get the AI-generated plan summary for a specific task
// @Tags AI Summary
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Plan summary"
// @Success 202 {object} map[string]interface{} "Summary still processing"
// @Failure 400 {object} map[string]interface{} "Invalid task ID"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Failure 404 {object} map[string]interface{} "Summary not found"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/plan-summary [get]
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
// @Summary Get apply summary
// @Description Get the AI-generated apply summary for a specific task
// @Tags AI Summary
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Apply summary"
// @Success 202 {object} map[string]interface{} "Summary still processing"
// @Failure 400 {object} map[string]interface{} "Invalid task ID"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Failure 404 {object} map[string]interface{} "Summary not found"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/apply-summary [get]
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
// @Summary Retry plan summary generation
// @Description Retry generating the plan summary for a failed task
// @Tags AI Summary
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 202 {object} map[string]interface{} "Retry triggered"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/plan-summary/retry [post]
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
// @Summary Retry apply summary generation
// @Description Retry generating the apply summary for a failed task
// @Tags AI Summary
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 202 {object} map[string]interface{} "Retry triggered"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/apply-summary/retry [post]
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

// ConfirmPlanSummary 提交 Plan Summary 风险决策
// @Summary Confirm plan summary risk decision
// @Description Submit a risk decision for a plan summary that requires confirmation
// @Tags AI Summary
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Param request body object true "Decision with decision_code and optional note"
// @Success 200 {object} map[string]interface{} "Decision submitted"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 403 {object} map[string]interface{} "Forbidden"
// @Failure 404 {object} map[string]interface{} "Summary not found"
// @Failure 409 {object} map[string]interface{} "Decision already submitted"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/plan-summary/confirm [post]
func (c *AISummaryController) ConfirmPlanSummary(ctx *gin.Context) {
	workspaceID := ctx.Param("id")
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 64)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	var req struct {
		DecisionCode string `json:"decision_code" binding:"required"`
		Note         string `json:"note"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "decision_code is required"})
		return
	}

	userID, _ := ctx.Get("user_id")
	username, _ := ctx.Get("username")
	isAdmin, _ := ctx.Get("is_system_admin")

	// 获取 plan summary + 校验
	summary := c.service.GetPlanSummary(uint(taskID))
	if summary == nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "plan summary not found"})
		return
	}
	if summary.WorkspaceID != workspaceID {
		ctx.JSON(http.StatusForbidden, gin.H{"error": "summary does not belong to this workspace"})
		return
	}
	if !summary.RequiresConfirmation {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "this plan summary does not require confirmation"})
		return
	}
	if summary.UserDecisionCode != "" {
		ctx.JSON(http.StatusConflict, gin.H{"error": "decision already submitted"})
		return
	}

	// 权限检查
	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	userIDStr, _ := userID.(string)
	isAdminBool, _ := isAdmin.(bool)
	if !isAdminBool {
		isCreator := task.CreatedBy != nil && *task.CreatedBy == userIDStr
		isApplyConfirmer := task.ApplyConfirmedBy != nil && *task.ApplyConfirmedBy == userIDStr
		if !isCreator && !isApplyConfirmer {
			ctx.JSON(http.StatusForbidden, gin.H{"error": "only task creator, apply confirmer, or admin can confirm decision"})
			return
		}
	}

	// 校验 decision_code 合法性（支持逗号分隔的多个 code）
	if len(summary.DecisionActions) > 0 && req.DecisionCode != "ABORT" {
		var actions []struct {
			Code string `json:"code"`
		}
		json.Unmarshal(summary.DecisionActions, &actions)
		actionSet := make(map[string]bool, len(actions))
		for _, a := range actions {
			actionSet[a.Code] = true
		}
		submittedCodes := strings.Split(req.DecisionCode, ",")
		for _, code := range submittedCodes {
			if !actionSet[code] {
				ctx.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid decision_code: %s", code)})
				return
			}
		}
	}

	// 解析用户名
	usernameStr := "unknown"
	if u, ok := username.(string); ok && u != "" {
		usernameStr = u
	}

	// 写入决策（一次性写入）
	now := time.Now()
	c.db.Model(&models.AIPlanSummary{}).Where("id = ?", summary.ID).Updates(map[string]interface{}{
		"user_decision_code": req.DecisionCode,
		"user_decision_note": req.Note,
		"user_decision_by":   usernameStr,
		"user_decision_at":   now,
	})

	// ABORT → 取消任务；其他 → 恢复 apply_pending
	if req.DecisionCode == "ABORT" {
		c.db.Model(&models.WorkspaceTask{}).
			Where("id = ? AND status = ?", taskID, string(models.TaskStatusDecisionRequired)).
			Updates(map[string]interface{}{
				"status":       string(models.TaskStatusCancelled),
				"stage":        "cancelled",
				"error_message": "用户在风险决策阶段选择终止变更",
				"completed_at":  now,
			})
		// 解锁 workspace
		c.db.Model(&models.Workspace{}).
			Where("workspace_id = ? AND lock_id IS NOT NULL", task.WorkspaceID).
			Updates(map[string]interface{}{
				"lock_id":   nil,
				"lock_info": nil,
			})
	} else {
		c.db.Model(&models.WorkspaceTask{}).
			Where("id = ? AND status = ?", taskID, string(models.TaskStatusDecisionRequired)).
			Updates(map[string]interface{}{
				"status": string(models.TaskStatusApplyPending),
				"stage":  "apply_pending",
			})
	}

	// 自动写入 task comment
	var decisionLabels []string
	if len(summary.DecisionActions) > 0 {
		var actions []struct {
			Code  string `json:"code"`
			Label string `json:"label"`
		}
		json.Unmarshal(summary.DecisionActions, &actions)
		actionMap := make(map[string]string, len(actions))
		for _, a := range actions {
			actionMap[a.Code] = a.Label
		}
		for _, code := range strings.Split(req.DecisionCode, ",") {
			if label, ok := actionMap[code]; ok {
				decisionLabels = append(decisionLabels, label)
			} else {
				decisionLabels = append(decisionLabels, code)
			}
		}
	}
	if len(decisionLabels) == 0 {
		decisionLabels = []string{req.DecisionCode}
	}

	commentContent := fmt.Sprintf("[AI 风险决策] %s 确认：%s", usernameStr, strings.Join(decisionLabels, "；"))
	if req.Note != "" {
		commentContent += fmt.Sprintf("\n补充说明：%s", req.Note)
	}
	c.db.Create(&models.TaskComment{
		TaskID:     uint(taskID),
		UserID:     &userIDStr,
		Username:   usernameStr,
		Comment:    commentContent,
		ActionType: "risk_decision",
	})
	_ = workspaceID // used in workspace validation above

	ctx.JSON(http.StatusOK, gin.H{"message": "决策已提交"})
}
