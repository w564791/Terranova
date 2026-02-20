package controllers

import (
	"fmt"
	"strings"
	"time"

	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TaskLogController 任务日志控制器
type TaskLogController struct {
	db *gorm.DB
}

// NewTaskLogController 创建控制器
func NewTaskLogController(db *gorm.DB) *TaskLogController {
	return &TaskLogController{
		db: db,
	}
}

// GetTaskLogs 获取历史任务日志
// @Summary 获取任务日志
// @Description 获取任务的历史日志，支持JSON和文本格式
// @Tags Task Log
// @Accept json
// @Produce json
// @Param task_id path int true "任务ID"
// @Param type query string false "日志类型（plan/apply/all）" default(all)
// @Param format query string false "输出格式（json/text）" default(json)
// @Success 200 {object} map[string]interface{} "成功返回日志"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Router /api/v1/tasks/{task_id}/logs [get]
// @Security Bearer
func (c *TaskLogController) GetTaskLogs(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	logType := ctx.DefaultQuery("type", "all")
	format := ctx.DefaultQuery("format", "json")

	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(404, gin.H{"error": "task not found"})
		return
	}

	// TODO: 检查权限
	// userID := ctx.GetUint("user_id")
	// if !c.checkPermission(userID, task.WorkspaceID) {
	//     ctx.JSON(403, gin.H{"error": "permission denied"})
	//     return
	// }

	if format == "text" {
		// 返回纯文本格式
		c.returnTextLogs(ctx, &task, logType)
		return
	}

	// 返回JSON格式
	response := gin.H{
		"task_id":      task.ID,
		"task_type":    task.TaskType,
		"status":       task.Status,
		"created_at":   task.CreatedAt,
		"completed_at": task.CompletedAt,
		"duration":     task.Duration,
		"logs":         gin.H{},
	}

	if logType == "plan" || logType == "all" {
		if task.PlanOutput != "" {
			response["logs"].(gin.H)["plan"] = gin.H{
				"output":     task.PlanOutput,
				"line_count": strings.Count(task.PlanOutput, "\n"),
				"size_bytes": len(task.PlanOutput),
			}
		}
	}

	if logType == "apply" || logType == "all" {
		if task.ApplyOutput != "" {
			response["logs"].(gin.H)["apply"] = gin.H{
				"output":     task.ApplyOutput,
				"line_count": strings.Count(task.ApplyOutput, "\n"),
				"size_bytes": len(task.ApplyOutput),
			}
		}
	}

	ctx.JSON(200, response)
}

// returnTextLogs 返回纯文本格式日志
func (c *TaskLogController) returnTextLogs(
	ctx *gin.Context,
	task *models.WorkspaceTask,
	logType string,
) {
	var output strings.Builder

	if logType == "plan" || logType == "all" {
		if task.PlanOutput != "" {
			output.WriteString("=== PLAN OUTPUT ===\n")
			output.WriteString(task.PlanOutput)
			output.WriteString("\n\n")
		}
	}

	if logType == "apply" || logType == "all" {
		if task.ApplyOutput != "" {
			output.WriteString("=== APPLY OUTPUT ===\n")
			output.WriteString(task.ApplyOutput)
			output.WriteString("\n\n")
		}
	}

	if task.ErrorMessage != "" {
		output.WriteString("=== ERROR ===\n")
		output.WriteString(task.ErrorMessage)
	}

	ctx.Header("Content-Type", "text/plain; charset=utf-8")
	ctx.String(200, output.String())
}

// DownloadTaskLogs 下载任务日志
// @Summary 下载任务日志文件
// @Description 下载任务日志为文本文件
// @Tags Task Log
// @Accept json
// @Produce text/plain
// @Param task_id path int true "任务ID"
// @Param type query string false "日志类型（plan/apply/all）" default(all)
// @Success 200 {file} file "日志文件"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Router /api/v1/tasks/{task_id}/logs/download [get]
// @Security Bearer
func (c *TaskLogController) DownloadTaskLogs(ctx *gin.Context) {
	taskID := ctx.Param("task_id")
	logType := ctx.DefaultQuery("type", "all")

	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(404, gin.H{"error": "task not found"})
		return
	}

	// TODO: 检查权限

	var output strings.Builder

	// 添加元数据
	output.WriteString(fmt.Sprintf("Task ID: %d\n", task.ID))
	output.WriteString(fmt.Sprintf("Task Type: %s\n", task.TaskType))
	output.WriteString(fmt.Sprintf("Status: %s\n", task.Status))
	output.WriteString(fmt.Sprintf("Created: %s\n", task.CreatedAt.Format(time.RFC3339)))
	if task.CompletedAt != nil {
		output.WriteString(fmt.Sprintf("Completed: %s\n", task.CompletedAt.Format(time.RFC3339)))
		output.WriteString(fmt.Sprintf("Duration: %ds\n", task.Duration))
	}
	output.WriteString("\n" + strings.Repeat("=", 80) + "\n\n")

	// 添加日志内容
	if logType == "plan" || logType == "all" {
		if task.PlanOutput != "" {
			output.WriteString("PLAN OUTPUT:\n")
			output.WriteString(strings.Repeat("-", 80) + "\n")
			output.WriteString(task.PlanOutput)
			output.WriteString("\n\n")
		}
	}

	if logType == "apply" || logType == "all" {
		if task.ApplyOutput != "" {
			output.WriteString("APPLY OUTPUT:\n")
			output.WriteString(strings.Repeat("-", 80) + "\n")
			output.WriteString(task.ApplyOutput)
			output.WriteString("\n\n")
		}
	}

	if task.ErrorMessage != "" {
		output.WriteString("ERROR:\n")
		output.WriteString(strings.Repeat("-", 80) + "\n")
		output.WriteString(task.ErrorMessage)
	}

	filename := fmt.Sprintf("task-%d-logs.txt", task.ID)
	ctx.Header("Content-Type", "application/octet-stream")
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=\"%s\"", filename))
	ctx.String(200, output.String())
}
