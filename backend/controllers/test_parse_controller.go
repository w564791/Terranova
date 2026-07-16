package controllers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ManualParsePlanWithDB 手动触发Plan解析（测试用）
// @Summary Manually parse plan changes
// @Description Manually trigger plan JSON parsing and store resource changes for a task (admin/debug)
// @Tags Workspace Task
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Plan parsed successfully"
// @Failure 400 {object} map[string]interface{} "Invalid task ID"
// @Failure 500 {object} map[string]interface{} "Parse failed"
// @Security BearerAuth
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/parse-plan [post]
func ManualParsePlanWithDB(c *gin.Context, db *gorm.DB) {

	taskID, err := strconv.ParseUint(c.Param("task_id"), 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 创建解析服务
	planParserService := services.NewPlanParserService(db)

	// 执行解析
	if err := planParserService.ParseAndStorePlanChanges(uint(taskID)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "Failed to parse plan",
			"message": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Plan parsed successfully",
		"task_id": taskID,
	})
}
