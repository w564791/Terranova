package controllers

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"iac-platform/internal/middleware"
	"iac-platform/internal/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// TaskLogController 任务日志控制器
// 全局 /tasks/:id/logs 路径必须在加载 task 后按 workspace 鉴权（防水平越权 C2）
type TaskLogController struct {
	db  *gorm.DB
	iam *middleware.IAMPermissionMiddleware
}

// NewTaskLogController 创建控制器
func NewTaskLogController(db *gorm.DB, iam *middleware.IAMPermissionMiddleware) *TaskLogController {
	return &TaskLogController{db: db, iam: iam}
}

// loadTaskWithWorkspaceAccess 加载任务并对所属 workspace 做 READ 校验
func (c *TaskLogController) loadTaskWithWorkspaceAccess(ctx *gin.Context) (*models.WorkspaceTask, bool) {
	return loadAndAuthorizeTaskWorkspace(ctx, c.db, c.iam)
}

// GetTaskLogs 获取历史任务日志
// @Summary 获取任务日志
// @Description 获取任务的历史日志（须对任务所属 workspace 有 READ）
// @Tags Task Log
// @Accept json
// @Produce json
// @Param task_id path int true "任务ID"
// @Param type query string false "日志类型（plan/apply/all）" default(all)
// @Param format query string false "输出格式（json/text）" default(json)
// @Success 200 {object} map[string]interface{} "成功返回日志"
// @Failure 403 {object} map[string]interface{} "无 workspace 权限"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Router /api/v1/tasks/{task_id}/logs [get]
// @Security BearerAuth
func (c *TaskLogController) GetTaskLogs(ctx *gin.Context) {
	logType := ctx.DefaultQuery("type", "all")
	format := ctx.DefaultQuery("format", "json")

	task, ok := c.loadTaskWithWorkspaceAccess(ctx)
	if !ok {
		return
	}

	if format == "text" {
		c.returnTextLogs(ctx, task, logType)
		return
	}

	response := gin.H{
		"task_id":      task.ID,
		"workspace_id": task.WorkspaceID,
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

	ctx.JSON(http.StatusOK, response)
}

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
	ctx.String(http.StatusOK, output.String())
}

// DownloadTaskLogs 下载任务日志
// @Summary 下载任务日志文件
// @Description 下载任务日志为文本文件（须对任务所属 workspace 有 READ）
// @Tags Task Log
// @Accept json
// @Produce text/plain
// @Param task_id path int true "任务ID"
// @Param type query string false "日志类型（plan/apply/all）" default(all)
// @Success 200 {file} file "日志文件"
// @Failure 403 {object} map[string]interface{} "无 workspace 权限"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Router /api/v1/tasks/{task_id}/logs/download [get]
// @Security BearerAuth
func (c *TaskLogController) DownloadTaskLogs(ctx *gin.Context) {
	logType := ctx.DefaultQuery("type", "all")

	task, ok := c.loadTaskWithWorkspaceAccess(ctx)
	if !ok {
		return
	}

	var output strings.Builder
	output.WriteString(fmt.Sprintf("Task ID: %d\n", task.ID))
	output.WriteString(fmt.Sprintf("Workspace: %s\n", task.WorkspaceID))
	output.WriteString(fmt.Sprintf("Task Type: %s\n", task.TaskType))
	output.WriteString(fmt.Sprintf("Status: %s\n", task.Status))
	output.WriteString(fmt.Sprintf("Created: %s\n", task.CreatedAt.Format(time.RFC3339)))
	output.WriteString("\n")

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

	filename := fmt.Sprintf("task_%d_logs.txt", task.ID)
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	ctx.Header("Content-Type", "text/plain; charset=utf-8")
	ctx.String(http.StatusOK, output.String())
}
