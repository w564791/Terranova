package handlers

import (
	"fmt"
	"net/http"
	"strings"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// RunTaskCallbackHandler handles run task callback requests
type RunTaskCallbackHandler struct {
	db       *gorm.DB
	executor *services.RunTaskExecutor
}

// NewRunTaskCallbackHandler creates a new run task callback handler
func NewRunTaskCallbackHandler(db *gorm.DB, executor *services.RunTaskExecutor) *RunTaskCallbackHandler {
	return &RunTaskCallbackHandler{db: db, executor: executor}
}

// HandleCallback handles callback from external run task service
// @Summary Handle run task callback
// @Description Receive callback from external run task service
// @Tags Run Task Callback
// @Accept json
// @Produce json
// @Param result_id path string true "Result ID"
// @Param request body models.RunTaskCallbackPayload true "Callback data"
// @Success 200 {object} map[string]interface{}
// @Failure 400,404,410,500 {object} map[string]interface{}
// @Router /api/v1/run-task-results/{result_id}/callback [patch]
func (h *RunTaskCallbackHandler) HandleCallback(c *gin.Context) {
	resultID := c.Param("result_id")
	if resultID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "result_id is required"})
		return
	}

	var callbackData models.RunTaskCallbackPayload
	if err := c.ShouldBindJSON(&callbackData); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate callback data
	if callbackData.Data.Type != "task-results" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid data type"})
		return
	}

	status := callbackData.Data.Attributes.Status
	if status != "running" && status != "passed" && status != "failed" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status"})
		return
	}

	// Process callback
	if err := h.executor.HandleCallback(resultID, &callbackData); err != nil {
		errMsg := err.Error()
		if strings.Contains(errMsg, "result not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
			return
		}
		if strings.Contains(errMsg, "result already completed") {
			c.JSON(http.StatusGone, gin.H{"error": errMsg})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": errMsg})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "callback processed successfully"})
}

// GetRunTaskResults gets run task results for a task
// @Summary Get run task results
// @Description Get all run task results for a workspace task
// @Tags Run Task Results
// @Produce json
// @Param workspace_id path string true "Workspace ID"
// @Param task_id path string true "Task ID"
// @Success 200 {object} map[string]interface{}
// @Router /api/v1/workspaces/{workspace_id}/tasks/{task_id}/run-task-results [get]
func (h *RunTaskCallbackHandler) GetRunTaskResults(c *gin.Context) {
	taskIDStr := c.Param("task_id")
	if taskIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "task_id is required"})
		return
	}

	var taskID uint
	if _, err := fmt.Sscanf(taskIDStr, "%d", &taskID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid task_id"})
		return
	}

	results, err := h.executor.GetResultsForTask(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get results"})
		return
	}

	responses := make([]models.RunTaskResultResponse, 0, len(results))
	for _, r := range results {
		responses = append(responses, r.ToResponse())
	}

	c.JSON(http.StatusOK, gin.H{"run_task_results": responses})
}

// GetRunTaskResult gets a single run task result
// @Summary Get run task result
// @Description Get a single run task result by ID
// @Tags Run Task Results
// @Produce json
// @Param result_id path string true "Result ID"
// @Success 200 {object} models.RunTaskResultResponse
// @Router /api/v1/run-task-results/{result_id} [get]
func (h *RunTaskCallbackHandler) GetRunTaskResult(c *gin.Context) {
	resultID := c.Param("result_id")
	if resultID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "result_id is required"})
		return
	}

	result, err := h.executor.GetResultByID(resultID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "result not found"})
		return
	}

	c.JSON(http.StatusOK, result.ToResponse())
}
