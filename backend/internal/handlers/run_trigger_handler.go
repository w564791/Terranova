package handlers

import (
	"net/http"
	"strconv"
	"time"

	"iac-platform/internal/middleware"
	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RunTriggerHandler 处理 Run Trigger 相关的 HTTP 请求
type RunTriggerHandler struct {
	db      *gorm.DB
	service *services.RunTriggerService
	iam     *middleware.IAMPermissionMiddleware
}

// NewRunTriggerHandler 创建 RunTriggerHandler 实例
func NewRunTriggerHandler(db *gorm.DB, iam *middleware.IAMPermissionMiddleware) *RunTriggerHandler {
	return &RunTriggerHandler{
		db:      db,
		service: services.NewRunTriggerService(db),
		iam:     iam,
	}
}

// ListRunTriggers 获取 workspace 配置的所有触发器
// @Summary 获取 workspace 的 Run Triggers
// @Description 获取指定 workspace 配置的所有触发器（作为源 workspace）
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers [get]
// @Security BearerAuth
func (h *RunTriggerHandler) ListRunTriggers(c *gin.Context) {
	workspaceID := c.Param("id")

	// 获取作为源的触发器
	triggers, err := h.service.GetRunTriggersBySource(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch run triggers"})
		return
	}

	// 获取 auto_apply 警告
	warnings, _ := h.service.GetTargetWorkspaceAutoApplyWarning(workspaceID)

	c.JSON(http.StatusOK, gin.H{
		"run_triggers":        triggers,
		"auto_apply_warnings": warnings,
	})
}

// ListInboundTriggers 获取哪些 workspace 会触发当前 workspace
// @Summary 获取入站触发器
// @Description 获取哪些 workspace 会触发当前 workspace
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/inbound [get]
// @Security BearerAuth
func (h *RunTriggerHandler) ListInboundTriggers(c *gin.Context) {
	workspaceID := c.Param("id")

	triggers, err := h.service.GetRunTriggersByTarget(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch inbound triggers"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"inbound_triggers": triggers,
	})
}

// GetAvailableTargets 获取可以作为触发目标的 workspace 列表
// @Summary 获取可用的触发目标
// @Description 获取可以作为触发目标的 workspace 列表（排除已配置的和会形成循环的）
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/available-targets [get]
// @Security BearerAuth
func (h *RunTriggerHandler) GetAvailableTargets(c *gin.Context) {
	workspaceID := c.Param("id")

	workspaces, err := h.service.GetAvailableTargetWorkspaces(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch available targets"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available_workspaces": workspaces,
	})
}

// GetAvailableSources 获取可以作为触发源的 workspace 列表
// @Summary 获取可用的触发源
// @Description 获取可以作为触发源的 workspace 列表（排除已配置的和会形成循环的）
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/available-sources [get]
// @Security BearerAuth
func (h *RunTriggerHandler) GetAvailableSources(c *gin.Context) {
	workspaceID := c.Param("id")

	workspaces, err := h.service.GetAvailableSourceWorkspaces(workspaceID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch available sources"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"available_workspaces": workspaces,
	})
}

// CreateInboundTrigger 创建入站触发器（允许某个 workspace 触发当前 workspace）
// @Summary 创建入站触发器
// @Description 允许某个 workspace 触发当前 workspace
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Target Workspace ID"
// @Param request body object true "触发器配置"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/inbound [post]
// @Security BearerAuth
func (h *RunTriggerHandler) CreateInboundTrigger(c *gin.Context) {
	targetWorkspaceID := c.Param("id")

	var req struct {
		SourceWorkspaceID string `json:"source_workspace_id" binding:"required"`
		Enabled           *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_workspace_id is required"})
		return
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	var createdBy *string
	if uid, ok := userID.(string); ok {
		createdBy = &uid
	}

	// 检查源 workspace 是否存在
	var sourceWorkspace models.Workspace
	if err := h.db.Where("workspace_id = ?", req.SourceWorkspaceID).First(&sourceWorkspace).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Source workspace not found"})
		return
	}

	// 检查目标 workspace 是否存在
	var targetWorkspace models.Workspace
	if err := h.db.Where("workspace_id = ?", targetWorkspaceID).First(&targetWorkspace).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target workspace not found"})
		return
	}

	// 对 source 写权限（被允许触发本 WS 的源）
	if h.iam == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "IAM not configured"})
		return
	}
	if !h.iam.RequireWorkspacePermission(c, req.SourceWorkspaceID, "WRITE") {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	trigger := &models.RunTrigger{
		SourceWorkspaceID: req.SourceWorkspaceID,
		TargetWorkspaceID: targetWorkspaceID,
		Enabled:           enabled,
		TriggerCondition:  models.TriggerConditionApplySuccess,
		CreatedBy:         createdBy,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := h.service.CreateRunTrigger(trigger); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查目标 workspace 是否开启了 auto_apply
	var warning string
	if targetWorkspace.AutoApply {
		warning = "Warning: This workspace has auto_apply enabled. Triggered tasks will automatically apply changes without manual confirmation."
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":     "Run trigger created successfully",
		"run_trigger": trigger,
		"warning":     warning,
	})
}

// CreateRunTrigger 创建触发器
// @Summary 创建 Run Trigger
// @Description 创建新的 workspace 触发器
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Source Workspace ID"
// @Param request body object true "触发器配置"
// @Success 201 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers [post]
// @Security BearerAuth
func (h *RunTriggerHandler) CreateRunTrigger(c *gin.Context) {
	sourceWorkspaceID := c.Param("id")

	var req struct {
		TargetWorkspaceID string `json:"target_workspace_id" binding:"required"`
		Enabled           *bool  `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_workspace_id is required"})
		return
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	var createdBy *string
	if uid, ok := userID.(string); ok {
		createdBy = &uid
	}

	// 检查目标 workspace 是否存在
	var targetWorkspace models.Workspace
	if err := h.db.Where("workspace_id = ?", req.TargetWorkspaceID).First(&targetWorkspace).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target workspace not found"})
		return
	}

	// 必须对 target 有 WRITE（防止把触发链指到无权限的 WS）C3
	if h.iam == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "IAM not configured"})
		return
	}
	if !h.iam.RequireWorkspacePermission(c, req.TargetWorkspaceID, "WRITE") {
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	trigger := &models.RunTrigger{
		SourceWorkspaceID: sourceWorkspaceID,
		TargetWorkspaceID: req.TargetWorkspaceID,
		Enabled:           enabled,
		TriggerCondition:  models.TriggerConditionApplySuccess,
		CreatedBy:         createdBy,
		CreatedAt:         time.Now(),
		UpdatedAt:         time.Now(),
	}

	if err := h.service.CreateRunTrigger(trigger); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 检查目标 workspace 是否开启了 auto_apply
	var warning string
	if targetWorkspace.AutoApply {
		warning = "Warning: Target workspace has auto_apply enabled. Triggered tasks will automatically apply changes without manual confirmation."
	}

	c.JSON(http.StatusCreated, gin.H{
		"message":     "Run trigger created successfully",
		"run_trigger": trigger,
		"warning":     warning,
	})
}

// UpdateRunTrigger 更新触发器
// @Summary 更新 Run Trigger
// @Description 更新触发器配置
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param trigger_id path int true "Trigger ID"
// @Param request body object true "更新内容"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/{trigger_id} [put]
// @Security BearerAuth
func (h *RunTriggerHandler) UpdateRunTrigger(c *gin.Context) {
	sourceWorkspaceID := c.Param("id")
	triggerID, err := strconv.ParseUint(c.Param("trigger_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trigger ID"})
		return
	}

	var req struct {
		Enabled *bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.service.UpdateRunTriggerInSource(uint(triggerID), sourceWorkspaceID, updates); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Run trigger not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update run trigger"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Run trigger updated successfully",
	})
}

// DeleteRunTrigger 删除触发器
// @Summary 删除 Run Trigger
// @Description 删除触发器配置
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param trigger_id path int true "Trigger ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/run-triggers/{trigger_id} [delete]
// @Security BearerAuth
func (h *RunTriggerHandler) DeleteRunTrigger(c *gin.Context) {
	sourceWorkspaceID := c.Param("id")
	triggerID, err := strconv.ParseUint(c.Param("trigger_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid trigger ID"})
		return
	}

	if err := h.service.DeleteRunTriggerInSource(uint(triggerID), sourceWorkspaceID); err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "Run trigger not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete run trigger"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Run trigger deleted successfully",
	})
}

// GetTaskTriggerExecutions 获取任务的触发执行记录
// @Summary 获取任务的触发执行记录
// @Description 获取任务完成后将要触发的 workspace 列表及其状态
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/trigger-executions [get]
// @Security BearerAuth
func (h *RunTriggerHandler) GetTaskTriggerExecutions(c *gin.Context) {
	workspaceID := c.Param("id")
	taskID, err := strconv.ParseUint(c.Param("task_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// task 必须属于 path workspace
	var task models.WorkspaceTask
	if err := h.db.Select("id", "workspace_id").Where("id = ? AND workspace_id = ?", taskID, workspaceID).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 首先尝试获取任务的 trigger executions
	executions, err := h.service.GetTaskTriggerExecutions(uint(taskID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch trigger executions"})
		return
	}

	// 如果没有 trigger executions，则直接查询 run_triggers 表
	// 这样即使任务是在 run trigger 创建之前创建的，也能显示当前的 triggers
	if len(executions) == 0 {
		triggers, err := h.service.GetRunTriggersBySource(workspaceID)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch run triggers"})
			return
		}

		// 将 triggers 转换为 executions 格式
		var virtualExecutions []map[string]interface{}
		for _, trigger := range triggers {
			if trigger.Enabled {
				virtualExecutions = append(virtualExecutions, map[string]interface{}{
					"id":                   0, // 虚拟 ID
					"source_task_id":       taskID,
					"run_trigger_id":       trigger.ID,
					"status":               "pending",
					"temporarily_disabled": false,
					"run_trigger":          trigger,
				})
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"trigger_executions": virtualExecutions,
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"trigger_executions": executions,
	})
}

// ToggleTriggerExecution 临时启用/禁用触发执行
// @Summary 临时启用/禁用触发执行
// @Description 在任务执行期间临时启用或禁用某个触发
// @Tags Workspace Run Trigger
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Param execution_id path int true "Execution ID"
// @Param request body object true "启用/禁用状态"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/trigger-executions/{execution_id}/toggle [post]
// @Security BearerAuth
func (h *RunTriggerHandler) ToggleTriggerExecution(c *gin.Context) {
	workspaceID := c.Param("id")
	taskID, err := strconv.ParseUint(c.Param("task_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}
	executionID, err := strconv.ParseUint(c.Param("execution_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid execution ID"})
		return
	}

	// task 必须属于 path workspace
	var task models.WorkspaceTask
	if err := h.db.Select("id").Where("id = ? AND workspace_id = ?", taskID, workspaceID).First(&task).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}
	// execution 必须属于该 task
	var execCount int64
	if err := h.db.Table("run_trigger_executions").
		Where("id = ? AND source_task_id = ?", executionID, taskID).
		Count(&execCount).Error; err != nil || execCount == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "Trigger execution not found"})
		return
	}

	var req struct {
		Disabled bool `json:"disabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// 获取当前用户
	userID, _ := c.Get("user_id")
	uid := ""
	if u, ok := userID.(string); ok {
		uid = u
	}

	if err := h.service.ToggleTriggerExecution(uint(executionID), req.Disabled, uid); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to toggle trigger execution"})
		return
	}

	action := "enabled"
	if req.Disabled {
		action = "disabled"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Trigger execution " + action + " successfully",
	})
}
