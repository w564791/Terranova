package controllers

import (
	"iac-platform/services"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// ManualParsePlanWithDB 手动触发Plan解析（测试用）
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
