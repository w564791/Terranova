package handlers

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WorkspaceRunTaskHandler handles workspace run task-related HTTP requests
type WorkspaceRunTaskHandler struct {
	db *gorm.DB
}

// NewWorkspaceRunTaskHandler creates a new workspace run task handler
func NewWorkspaceRunTaskHandler(db *gorm.DB) *WorkspaceRunTaskHandler {
	return &WorkspaceRunTaskHandler{db: db}
}

// generateWorkspaceRunTaskID generates a semantic workspace run task ID
func generateWorkspaceRunTaskID() (string, error) {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	const length = 16
	b := make([]byte, length)
	charsetLen := big.NewInt(int64(len(charset)))
	for i := range b {
		num, err := rand.Int(rand.Reader, charsetLen)
		if err != nil {
			return "", fmt.Errorf("failed to generate random number: %w", err)
		}
		b[i] = charset[num.Int64()]
	}
	return fmt.Sprintf("wrt-%s", string(b)), nil
}

// AddRunTaskToWorkspace adds a run task to a workspace
// @Summary Add run task to workspace
// @Description Associate a run task with a workspace
// @Tags Workspace Run Task
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param request body models.AddWorkspaceRunTaskRequest true "Run task configuration"
// @Success 201 {object} models.WorkspaceRunTaskResponse
// @Failure 400,404,409,500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/run-tasks [post]
func (h *WorkspaceRunTaskHandler) AddRunTaskToWorkspace(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	var req models.CreateWorkspaceRunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate stage
	validStages := map[models.RunTaskStage]bool{
		models.RunTaskStagePrePlan:   true,
		models.RunTaskStagePostPlan:  true,
		models.RunTaskStagePreApply:  true,
		models.RunTaskStagePostApply: true,
	}
	if !validStages[req.Stage] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stage"})
		return
	}

	// Validate enforcement level
	if req.EnforcementLevel != "" && req.EnforcementLevel != models.RunTaskEnforcementAdvisory && req.EnforcementLevel != models.RunTaskEnforcementMandatory {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid enforcement_level"})
		return
	}
	if req.EnforcementLevel == "" {
		req.EnforcementLevel = models.RunTaskEnforcementAdvisory
	}

	// Check workspace exists
	var workspace models.Workspace
	if err := h.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check workspace"})
		return
	}

	// Check run task exists
	var runTask models.RunTask
	if err := h.db.Where("run_task_id = ?", req.RunTaskID).First(&runTask).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "run task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check run task"})
		return
	}

	// Check for duplicate
	var existing models.WorkspaceRunTask
	if err := h.db.Where("workspace_id = ? AND run_task_id = ? AND stage = ?", workspaceID, req.RunTaskID, req.Stage).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "run task already configured for this stage"})
		return
	}

	// Generate ID
	wrtID, err := generateWorkspaceRunTaskID()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate ID"})
		return
	}

	userID, _ := c.Get("user_id")
	createdBy := ""
	if userID != nil {
		createdBy = userID.(string)
	}

	wrt := &models.WorkspaceRunTask{
		WorkspaceRunTaskID: wrtID,
		WorkspaceID:        workspaceID,
		RunTaskID:          req.RunTaskID,
		Stage:              req.Stage,
		EnforcementLevel:   req.EnforcementLevel,
		Enabled:            true,
		CreatedBy:          &createdBy,
		CreatedAt:          time.Now(),
		UpdatedAt:          time.Now(),
	}

	if err := h.db.Create(wrt).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create workspace run task"})
		return
	}

	wrt.RunTask = &runTask
	c.JSON(http.StatusCreated, wrt.ToResponse())
}

// ListWorkspaceRunTasks lists all run tasks for a workspace (including global run tasks)
// @Summary List workspace run tasks
// @Description Get all run tasks configured for a workspace, including global run tasks
// @Tags Workspace Run Task
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/run-tasks [get]
func (h *WorkspaceRunTaskHandler) ListWorkspaceRunTasks(c *gin.Context) {
	workspaceID := c.Param("id")
	if workspaceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id is required"})
		return
	}

	// Get workspace-specific run tasks
	var wrts []models.WorkspaceRunTask
	if err := h.db.Preload("RunTask").Where("workspace_id = ?", workspaceID).Order("stage, created_at").Find(&wrts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve workspace run tasks"})
		return
	}

	responses := make([]models.WorkspaceRunTaskResponse, 0, len(wrts))
	for _, wrt := range wrts {
		responses = append(responses, wrt.ToResponse())
	}

	// Get global run tasks (is_global = true and enabled = true)
	var globalRunTasks []models.RunTask
	if err := h.db.Where("is_global = ? AND enabled = ?", true, true).Find(&globalRunTasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve global run tasks"})
		return
	}

	// Convert global run tasks to response format
	globalResponses := make([]map[string]interface{}, 0, len(globalRunTasks))
	for _, rt := range globalRunTasks {
		globalResponses = append(globalResponses, map[string]interface{}{
			"run_task_id":              rt.RunTaskID,
			"name":                     rt.Name,
			"description":              rt.Description,
			"endpoint_url":             rt.EndpointURL,
			"enabled":                  rt.Enabled,
			"is_global":                true,
			"global_stages":            rt.GlobalStages,
			"global_enforcement_level": rt.GlobalEnforcementLevel,
			"timeout_seconds":          rt.TimeoutSeconds,
			"max_run_seconds":          rt.MaxRunSeconds,
			"created_at":               rt.CreatedAt,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"workspace_run_tasks": responses,
		"global_run_tasks":    globalResponses,
	})
}

// UpdateWorkspaceRunTask updates a workspace run task configuration
// @Summary Update workspace run task
// @Description Update run task configuration for a workspace
// @Tags Workspace Run Task
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param workspace_run_task_id path string true "Workspace Run Task ID"
// @Param request body models.UpdateWorkspaceRunTaskRequest true "Updated configuration"
// @Success 200 {object} models.WorkspaceRunTaskResponse
// @Router /api/v1/workspaces/{workspace_id}/run-tasks/{workspace_run_task_id} [put]
func (h *WorkspaceRunTaskHandler) UpdateWorkspaceRunTask(c *gin.Context) {
	workspaceID := c.Param("id")
	wrtID := c.Param("workspace_run_task_id")

	var req models.UpdateWorkspaceRunTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	var wrt models.WorkspaceRunTask
	if err := h.db.Preload("RunTask").Where("workspace_run_task_id = ? AND workspace_id = ?", wrtID, workspaceID).First(&wrt).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "workspace run task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve workspace run task"})
		return
	}

	updates := map[string]interface{}{"updated_at": time.Now()}

	if req.Stage != nil {
		validStages := map[models.RunTaskStage]bool{
			models.RunTaskStagePrePlan:   true,
			models.RunTaskStagePostPlan:  true,
			models.RunTaskStagePreApply:  true,
			models.RunTaskStagePostApply: true,
		}
		if !validStages[*req.Stage] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid stage"})
			return
		}
		updates["stage"] = *req.Stage
	}

	if req.EnforcementLevel != nil {
		if *req.EnforcementLevel != models.RunTaskEnforcementAdvisory && *req.EnforcementLevel != models.RunTaskEnforcementMandatory {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid enforcement_level"})
			return
		}
		updates["enforcement_level"] = *req.EnforcementLevel
	}

	if req.Enabled != nil {
		updates["enabled"] = *req.Enabled
	}

	if err := h.db.Model(&wrt).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update workspace run task"})
		return
	}

	h.db.Preload("RunTask").Where("workspace_run_task_id = ?", wrtID).First(&wrt)
	c.JSON(http.StatusOK, wrt.ToResponse())
}

// DeleteWorkspaceRunTask removes a run task from a workspace
// @Summary Delete workspace run task
// @Description Remove a run task from a workspace
// @Tags Workspace Run Task
// @Param workspace_id path string true "Workspace ID"
// @Param workspace_run_task_id path string true "Workspace Run Task ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/run-tasks/{workspace_run_task_id} [delete]
func (h *WorkspaceRunTaskHandler) DeleteWorkspaceRunTask(c *gin.Context) {
	workspaceID := c.Param("id")
	wrtID := c.Param("workspace_run_task_id")

	result := h.db.Where("workspace_run_task_id = ? AND workspace_id = ?", wrtID, workspaceID).Delete(&models.WorkspaceRunTask{})
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete workspace run task"})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "workspace run task not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "workspace run task deleted successfully"})
}

// OverrideRunTasks overrides failed advisory run tasks and continues the task
// @Summary Override run tasks
// @Description Override failed advisory run tasks and continue with apply
// @Tags Workspace Run Task
// @Accept json
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param task_id path string true "Task ID"
// @Param request body map[string]string true "Override request with comment"
// @Success 200 {object} map[string]interface{}
// @Failure 400,403,404,500 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/tasks/{task_id}/override-run-tasks [post]
func (h *WorkspaceRunTaskHandler) OverrideRunTasks(c *gin.Context) {
	workspaceID := c.Param("id")
	taskIDStr := c.Param("task_id")

	if workspaceID == "" || taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and task_id are required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	var req struct {
		Comment string `json:"comment"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Verify task exists and belongs to workspace
	var task models.WorkspaceTask
	if err := h.db.Where("id = ? AND workspace_id = ?", taskID, workspaceID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check task"})
		return
	}

	// Check task is in apply_pending or plan_completed status
	if task.Status != "apply_pending" && task.Status != "plan_completed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task is not in apply_pending or plan_completed status"})
		return
	}

	// Get run task results for this task
	var results []models.RunTaskResult
	if err := h.db.Preload("WorkspaceRunTask").
		Where("task_id = ?", taskID).
		Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve run task results"})
		return
	}

	// Check for mandatory failures - cannot override
	for _, result := range results {
		if result.Status == models.RunTaskResultFailed || result.Status == models.RunTaskResultError || result.Status == models.RunTaskResultTimeout {
			var enforcementLevel models.RunTaskEnforcementLevel
			if result.WorkspaceRunTaskID != nil && result.WorkspaceRunTask != nil {
				enforcementLevel = result.WorkspaceRunTask.EnforcementLevel
			} else if result.RunTaskID != nil {
				// Global run task
				var rt models.RunTask
				if err := h.db.Where("run_task_id = ?", *result.RunTaskID).First(&rt).Error; err == nil {
					enforcementLevel = rt.GlobalEnforcementLevel
				}
			}
			if enforcementLevel == models.RunTaskEnforcementMandatory {
				c.JSON(http.StatusForbidden, gin.H{"error": "cannot override mandatory run task failures"})
				return
			}
		}
	}

	// Mark all failed advisory run tasks as overridden
	userID, _ := c.Get("user_id")
	username, _ := c.Get("username")
	now := time.Now()

	overriddenCount := 0
	for _, result := range results {
		if result.Status == models.RunTaskResultFailed || result.Status == models.RunTaskResultError || result.Status == models.RunTaskResultTimeout {
			// Update result status to indicate it was overridden
			// 同时更新 is_overridden, override_by, override_at 字段
			updates := map[string]interface{}{
				"status":        models.RunTaskResultOverridden,
				"message":       fmt.Sprintf("Overridden by %v: %s", username, req.Comment),
				"is_overridden": true,
				"override_at":   now,
				"updated_at":    now,
			}
			if userID != nil {
				updates["override_by"] = userID.(string)
			}
			h.db.Model(&result).Updates(updates)
			overriddenCount++
		}
	}

	// Add a comment to the task
	comment := &models.TaskComment{
		TaskID:     taskID,
		Comment:    fmt.Sprintf("Run tasks overridden: %s", req.Comment),
		ActionType: "override",
		CreatedAt:  now,
	}
	if userID != nil {
		uid := userID.(string)
		comment.UserID = &uid
	}
	h.db.Create(comment)

	// Update task status to apply_pending if it was plan_completed
	if task.Status == "plan_completed" {
		h.db.Model(&task).Updates(map[string]interface{}{
			"status":     "apply_pending",
			"stage":      "apply_pending",
			"updated_at": now,
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"message":       "run tasks overridden successfully",
		"overridden":    true,
		"overridden_by": username,
		"comment":       req.Comment,
	})
}

// GetTaskRunTaskResults gets all run task results for a specific task
// @Summary Get task run task results
// @Description Get all run task results for a specific workspace task
// @Tags Workspace Run Task
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/tasks/{task_id}/run-task-results [get]
func (h *WorkspaceRunTaskHandler) GetTaskRunTaskResults(c *gin.Context) {
	workspaceID := c.Param("id")
	taskIDStr := c.Param("task_id")

	if workspaceID == "" || taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workspace_id and task_id are required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id format"})
		return
	}

	// Verify task exists and belongs to workspace
	var task models.WorkspaceTask
	if err := h.db.Where("id = ? AND workspace_id = ?", taskID, workspaceID).First(&task).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check task"})
		return
	}

	// Get run task results for this task
	var results []models.RunTaskResult
	if err := h.db.Preload("WorkspaceRunTask.RunTask").Preload("Outcomes").
		Where("task_id = ?", taskID).
		Order("stage, created_at").
		Find(&results).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve run task results"})
		return
	}

	// Convert to response format with additional info for global run tasks
	responses := make([]map[string]interface{}, 0, len(results))
	for _, result := range results {
		resp := map[string]interface{}{
			"result_id":     result.ResultID,
			"task_id":       result.TaskID,
			"stage":         result.Stage,
			"status":        result.Status,
			"message":       result.Message,
			"url":           result.URL,
			"started_at":    result.StartedAt,
			"completed_at":  result.CompletedAt,
			"created_at":    result.CreatedAt,
			"outcomes":      result.Outcomes,
			"is_overridden": result.IsOverridden,
			"override_by":   result.OverrideBy,
			"override_at":   result.OverrideAt,
		}

		// Handle workspace run task (non-global)
		if result.WorkspaceRunTaskID != nil && result.WorkspaceRunTask != nil {
			resp["workspace_run_task_id"] = *result.WorkspaceRunTaskID
			resp["enforcement_level"] = result.WorkspaceRunTask.EnforcementLevel
			if result.WorkspaceRunTask.RunTask != nil {
				resp["run_task"] = map[string]interface{}{
					"run_task_id": result.WorkspaceRunTask.RunTask.RunTaskID,
					"name":        result.WorkspaceRunTask.RunTask.Name,
					"description": result.WorkspaceRunTask.RunTask.Description,
				}
			}
		}

		// Handle global run task
		if result.RunTaskID != nil {
			resp["run_task_id"] = *result.RunTaskID
			resp["is_global"] = true

			// Load global run task info
			var globalRunTask models.RunTask
			if err := h.db.Where("run_task_id = ?", *result.RunTaskID).First(&globalRunTask).Error; err == nil {
				resp["run_task"] = map[string]interface{}{
					"run_task_id": globalRunTask.RunTaskID,
					"name":        globalRunTask.Name,
					"description": globalRunTask.Description,
				}
				resp["enforcement_level"] = globalRunTask.GlobalEnforcementLevel
			}
		}

		responses = append(responses, resp)
	}

	c.JSON(http.StatusOK, gin.H{
		"run_task_results": responses,
		"total":            len(responses),
	})
}
