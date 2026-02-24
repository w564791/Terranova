package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"iac-platform/internal/models"
	"iac-platform/services"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WorkspaceTaskController 工作空间任务控制器
type WorkspaceTaskController struct {
	db                 *gorm.DB
	executor           *services.TerraformExecutor
	streamManager      *services.OutputStreamManager
	queueManager       *services.TaskQueueManager
	notificationSender *services.NotificationSender
	agentCCHandler     interface {
		CancelTaskOnAgent(agentID string, taskID uint) error
	}
}

// NewWorkspaceTaskController 创建任务控制器
func NewWorkspaceTaskController(
	db *gorm.DB,
	streamManager *services.OutputStreamManager,
	queueManager *services.TaskQueueManager,
	agentCCHandler interface {
		CancelTaskOnAgent(agentID string, taskID uint) error
	},
) *WorkspaceTaskController {
	executor := services.NewTerraformExecutor(db, streamManager)
	// 从平台配置获取 baseURL 用于通知链接
	platformConfigService := services.NewPlatformConfigService(db)
	baseURL := platformConfigService.GetBaseURL()
	notificationSender := services.NewNotificationSender(db, baseURL)

	return &WorkspaceTaskController{
		db:                 db,
		executor:           executor,
		streamManager:      streamManager,
		queueManager:       queueManager, // 使用传入的全局 queueManager
		notificationSender: notificationSender,
		agentCCHandler:     agentCCHandler,
	}
}

// CreatePlanTask 创建Plan任务
// @Summary 创建Plan任务
// @Description 创建Terraform Plan任务或Plan+Apply任务
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param request body object false "任务配置（description和run_type可选）"
// @Success 201 {object} map[string]interface{} "任务创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效"
// @Failure 401 {object} map[string]interface{} "未授权"
// @Failure 404 {object} map[string]interface{} "工作空间不存在"
// @Failure 423 {object} map[string]interface{} "工作空间已锁定"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/tasks/plan [post]
// @Security Bearer
func (c *WorkspaceTaskController) CreatePlanTask(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	// 获取当前用户ID（从JWT中间件）
	userID, exists := ctx.Get("user_id")
	if !exists {
		ctx.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}
	uid := userID.(string)

	log.Printf("[DEBUG] CreatePlanTask called: workspace=%s, user=%s", workspaceIDParam, uid)

	// 解析请求体
	var req struct {
		Description string `json:"description"`
		RunType     string `json:"run_type"` // "plan" 或 "plan_and_apply"
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// 如果没有请求体，继续执行（description是可选的）
		req.Description = ""
		req.RunType = "plan" // 默认为plan
	}

	// 如果没有指定run_type，默认为plan
	if req.RunType == "" {
		req.RunType = "plan"
	}

	// 验证run_type
	if req.RunType != "plan" && req.RunType != "plan_and_apply" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Invalid run_type. Must be 'plan' or 'plan_and_apply'",
		})
		return
	}

	// 检查workspace是否存在
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 检查workspace是否被锁定
	if workspace.IsLocked {
		ctx.JSON(http.StatusLocked, gin.H{
			"error":       "Workspace is locked",
			"locked_by":   workspace.LockedBy,
			"lock_reason": workspace.LockReason,
		})
		return
	}

	// Provider配置可选 - 如果没有配置provider，terraform将使用module自带配置或环境变量
	if workspace.ProviderConfig == nil || len(workspace.ProviderConfig) == 0 {
		log.Printf("Workspace %s has no provider config, tasks will run without provider.tf.json", workspace.WorkspaceID)
	}

	// 根据run_type确定任务类型
	var taskType models.TaskType
	if req.RunType == "plan_and_apply" {
		taskType = models.TaskTypePlanAndApply
	} else {
		taskType = models.TaskTypePlan
	}

	// 创建任务（只创建一个任务）
	task := &models.WorkspaceTask{
		WorkspaceID:   workspace.WorkspaceID,
		TaskType:      taskType,
		Status:        models.TaskStatusPending,
		ExecutionMode: workspace.ExecutionMode,
		CreatedBy:     &uid,
		Stage:         "pending",
		Description:   req.Description,
	}

	if err := c.db.Create(task).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create task"})
		return
	}

	// 发送任务创建通知
	go func() {
		if err := c.notificationSender.TriggerNotifications(
			context.Background(),
			workspace.WorkspaceID,
			models.NotificationEventTaskCreated,
			task,
		); err != nil {
			log.Printf("[Notification] Failed to send task_created notification for task %d: %v", task.ID, err)
		}
	}()

	// 立即创建快照（在任务创建时，而不是等Plan执行完成）
	// 这样即使任务被取消或失败，快照也会存在，可用于审计和调试
	log.Printf("[DEBUG] Creating snapshot for task %d at creation time", task.ID)
	if err := createTaskSnapshot(c.db, task, &workspace); err != nil {
		log.Printf("[WARN] Failed to create snapshot for task %d: %v", task.ID, err)
		// 不阻塞任务创建，快照创建失败只记录警告
	} else {
		log.Printf("[DEBUG] Snapshot created successfully for task %d", task.ID)
	}

	// 通知队列管理器尝试执行任务
	// 使用带重试的goroutine确保任务能被调度
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] TryExecuteNextTask panicked in CreatePlanTask for workspace %s: %v", workspace.WorkspaceID, r)
			}
		}()

		// 添加重试机制：最多重试3次，每次间隔递增
		maxRetries := 3
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				// 指数退避：1s, 2s, 4s
				waitTime := time.Duration(1<<uint(attempt-1)) * time.Second
				log.Printf("[TaskQueue] Retry attempt %d/%d for workspace %s after %v", attempt, maxRetries, workspace.WorkspaceID, waitTime)
				time.Sleep(waitTime)
			}

			err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID)
			if err == nil {
				// 成功，退出重试循环
				log.Printf("[TaskQueue] Successfully triggered task execution for workspace %s (attempt %d)", workspace.WorkspaceID, attempt+1)
				return
			}

			log.Printf("[ERROR] Failed to start task execution for workspace %s (attempt %d/%d): %v", workspace.WorkspaceID, attempt+1, maxRetries+1, err)

			// 如果是最后一次尝试，记录严重错误
			if attempt == maxRetries {
				log.Printf("[CRITICAL] All %d attempts failed to trigger task execution for workspace %s. Task %d may be stuck in pending state.", maxRetries+1, workspace.WorkspaceID, task.ID)
			}
		}
	}()

	// 返回创建的任务信息
	var message string
	if taskType == models.TaskTypePlanAndApply {
		message = "Plan+Apply task created successfully"
	} else {
		message = "Plan task created successfully"
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message": message,
		"task":    task,
	})
}

// CreateApplyTask — 已废弃，不再使用。
// 当前平台没有独立的 Apply 流程，所有 Apply 均通过 plan_and_apply + ConfirmApply 两阶段工作流完成：
//   1. POST /api/v1/workspaces/{id}/tasks/plan (run_type="plan_and_apply")
//   2. POST /api/v1/workspaces/{id}/tasks/{task_id}/confirm-apply
// 此函数未注册到任何 router，属于死代码。
// 保留原因：如果未来需要支持独立 Apply（基于已有 Plan 的二次 Apply），可参考此实现。
// func (c *WorkspaceTaskController) CreateApplyTask(ctx *gin.Context) { ... }

// GetTask 获取任务详情
// @Summary 获取任务详情
// @Description 根据ID获取任务的详细信息
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "成功返回任务详情"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Router /api/v1/workspaces/{id}/tasks/{task_id} [get]
// @Security Bearer
func (c *WorkspaceTaskController) GetTask(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// log.Printf("[DEBUG] GetTask: workspace_id=%s, task_id=%d, using workspace.WorkspaceID=%s",
	// 	workspaceIDParam, taskID, workspace.WorkspaceID)

	var task models.WorkspaceTask

	// 根据workspace配置决定是否排除plan_json和快照字段
	if workspace.ShowUnchangedResources {
		// 返回完整数据（包括plan_json）
		if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).
			First(&task).Error; err != nil {
			log.Printf("[ERROR] GetTask query failed: id=%d, workspace_id=%s, error=%v",
				taskID, workspace.WorkspaceID, err)
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
			return
		}
	} else {
		// 排除plan_json和快照字段以减少响应大小
		if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).
			Omit("plan_json", "snapshot_variables", "snapshot_resource_versions", "snapshot_provider_config").
			First(&task).Error; err != nil {
			log.Printf("[ERROR] GetTask query failed: id=%d, workspace_id=%s, error=%v",
				taskID, workspace.WorkspaceID, err)
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
			return
		}
	}

	// 获取创建者用户名
	var createdByUsername string
	if task.CreatedBy != nil {
		var user models.User
		if err := c.db.Where("user_id = ?", *task.CreatedBy).First(&user).Error; err == nil {
			createdByUsername = user.Username
		}
	}

	// 构建响应 - 将username添加到task对象中
	taskResponse := map[string]interface{}{
		"id":                  task.ID,
		"workspace_id":        task.WorkspaceID,
		"created_by":          task.CreatedBy,
		"created_by_username": createdByUsername,
		"created_at":          task.CreatedAt,
		"updated_at":          task.UpdatedAt,
		"description":         task.Description,
		"task_type":           task.TaskType,
		"status":              task.Status,
		"execution_mode":      task.ExecutionMode,
		"agent_id":            task.AgentID,
		"k8s_config_id":       task.K8sConfigID,
		"k8s_pod_name":        task.K8sPodName,
		"k8s_namespace":       task.K8sNamespace,
		"execution_node":      task.ExecutionNode,
		"locked_by":           task.LockedBy,
		"locked_at":           task.LockedAt,
		"lock_expires_at":     task.LockExpiresAt,
		"plan_output":         task.PlanOutput,
		"apply_output":        task.ApplyOutput,
		"error_message":       task.ErrorMessage,
		"started_at":          task.StartedAt,
		"completed_at":        task.CompletedAt,
		"duration":            task.Duration,
		"retry_count":         task.RetryCount,
		"max_retries":         task.MaxRetries,
		"changes_add":         task.ChangesAdd,
		"changes_change":      task.ChangesChange,
		"changes_destroy":     task.ChangesDestroy,
		"plan_task_id":        task.PlanTaskID,
		"stage":               task.Stage,
		"snapshot_id":         task.SnapshotID,
		"apply_description":   task.ApplyDescription,
		// Apply confirmation audit fields
		"apply_confirmed_by": task.ApplyConfirmedBy,
		"apply_confirmed_at": task.ApplyConfirmedAt,
	}

	// 只在ShowUnchangedResources为true时包含plan_json
	if workspace.ShowUnchangedResources {
		taskResponse["plan_json"] = task.PlanJSON
		taskResponse["outputs"] = task.Outputs
		taskResponse["context"] = task.Context
		taskResponse["snapshot_resource_versions"] = task.SnapshotResourceVersions
		taskResponse["snapshot_variables"] = task.SnapshotVariables
		taskResponse["snapshot_provider_config"] = task.SnapshotProviderConfig
		taskResponse["snapshot_created_at"] = task.SnapshotCreatedAt
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task": taskResponse,
	})
}

// GetTasks 获取任务列表
// @Summary 获取任务列表
// @Description 获取工作空间的任务列表，支持分页、搜索和过滤
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param page query int false "页码" default(1)
// @Param page_size query int false "每页数量" default(10)
// @Param search query string false "搜索关键词"
// @Param status query string false "状态过滤"
// @Param task_type query string false "任务类型过滤"
// @Param start_date query string false "开始日期（RFC3339格式）"
// @Param end_date query string false "结束日期（RFC3339格式）"
// @Success 200 {object} map[string]interface{} "成功返回任务列表"
// @Failure 400 {object} map[string]interface{} "无效的工作空间ID"
// @Failure 500 {object} map[string]interface{} "服务器错误"
// @Router /api/v1/workspaces/{id}/tasks [get]
// @Security Bearer
func (c *WorkspaceTaskController) GetTasks(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	// 获取workspace以获取内部ID (支持语义化ID和数字ID)
	var workspace models.Workspace
	err := c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		// 尝试作为数字ID查询
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 分页参数
	page, _ := strconv.Atoi(ctx.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(ctx.DefaultQuery("page_size", "10"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 10000 {
		pageSize = 10000 // 提高上限以支持获取所有数据
	}

	var tasks []models.WorkspaceTask
	var total int64

	// 基础查询 (使用语义化ID)
	// 默认排除后台任务（如 drift_check），除非明确请求
	showBackground := ctx.Query("show_background") == "true"
	query := c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspace.WorkspaceID)
	if !showBackground {
		query = query.Where("is_background = ? OR is_background IS NULL", false)
	}

	// 搜索参数 - 支持搜索description, ID, task_type
	search := ctx.Query("search")
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(
			"description LIKE ? OR CAST(id AS TEXT) LIKE ? OR task_type LIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// 时间范围过滤
	// 前端发送 UTC 时间（toISOString()），但 DB 列是 timestamp without time zone，
	// 存的是本地时间。pgx 对 timestamp 列会 discardTimeZone（只保留数值丢弃时区），
	// 所以必须先将 UTC 转为本地时区，确保查询条件和存储值在同一时区基准。
	startDate := ctx.Query("start_date")
	if startDate != "" {
		startTime, err := time.Parse(time.RFC3339, startDate)
		if err != nil {
			log.Printf("Failed to parse start_date: %v", err)
		} else {
			startTime = startTime.Local()
			query = query.Where("created_at >= ?", startTime)
			log.Printf("Time filter: start_date=%s (local)", startTime.Format(time.RFC3339))
		}
	}
	endDate := ctx.Query("end_date")
	if endDate != "" {
		endTime, err := time.Parse(time.RFC3339, endDate)
		if err != nil {
			log.Printf("Failed to parse end_date: %v", err)
		} else {
			endTime = endTime.Local()
			query = query.Where("created_at <= ?", endTime)
			log.Printf("Time filter: end_date=%s (local)", endTime.Format(time.RFC3339))
		}
	}

	// 状态过滤 - 支持前端的filter类型
	statusFilter := ctx.Query("status")
	if statusFilter != "" && statusFilter != "all" {
		switch statusFilter {
		case "needs_attention":
			query = query.Where("status IN ?", []string{"requires_approval", "apply_pending"})
		case "errored":
			query = query.Where("status = ?", "failed")
		case "success":
			query = query.Where("status IN ?", []string{"success", "applied"})
		case "cancelled":
			query = query.Where("status = ?", "cancelled")
		case "running":
			query = query.Where("status = ?", "running")
		case "on_hold":
			query = query.Where("status IN ?", []string{"on_hold", "pending", "apply_pending"})
		default:
			// 直接使用状态值
			query = query.Where("status = ?", statusFilter)
		}
	}

	// 任务类型过滤（保留原有功能）
	if taskType := ctx.Query("task_type"); taskType != "" {
		query = query.Where("task_type = ?", taskType)
	}

	// 获取过滤后的总数
	if err := query.Count(&total).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to count tasks"})
		return
	}

	// 计算filter counts - 统一使用 scope 确保条件一致
	// countBase 封装 workspace + background + search + time 过滤，与主查询条件对齐
	countBase := func() *gorm.DB {
		q := c.db.Model(&models.WorkspaceTask{}).Where("workspace_id = ?", workspace.WorkspaceID)
		if !showBackground {
			q = q.Where("is_background = ? OR is_background IS NULL", false)
		}
		return q.Scopes(applySearchAndTimeFilters(search, startDate, endDate))
	}

	filterCounts := map[string]int64{
		"all":             0,
		"needs_attention": 0,
		"errored":         0,
		"running":         0,
		"on_hold":         0,
		"success":         0,
		"cancelled":       0,
	}

	var count int64

	countBase().Count(&count)
	filterCounts["all"] = count

	countBase().Where("status = ?", "failed").Count(&count)
	filterCounts["errored"] = count

	countBase().Where("status = ?", "running").Count(&count)
	filterCounts["running"] = count

	countBase().Where("status IN ?", []string{"on_hold", "pending", "apply_pending"}).Count(&count)
	filterCounts["on_hold"] = count

	countBase().Where("status = ?", "cancelled").Count(&count)
	filterCounts["cancelled"] = count

	countBase().Where("status IN ?", []string{"success", "applied"}).Count(&count)
	filterCounts["success"] = count

	countBase().Where("status IN ?", []string{"requires_approval", "apply_pending"}).Count(&count)
	filterCounts["needs_attention"] = count

	// 分页查询 - 只选择列表页需要的字段，排除大字段
	offset := (page - 1) * pageSize
	if err := query.
		Select("id", "workspace_id", "task_type", "status", "created_at", "created_by",
			"description", "changes_add", "changes_change", "changes_destroy",
			"stage", "started_at", "completed_at").
		Order("created_at DESC").
		Limit(pageSize).
		Offset(offset).
		Find(&tasks).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch tasks"})
		return
	}

	// 批量查询创建者用户名
	userIDs := make([]string, 0)
	for _, t := range tasks {
		if t.CreatedBy != nil && *t.CreatedBy != "" {
			userIDs = append(userIDs, *t.CreatedBy)
		}
	}
	usernameMap := make(map[string]string)
	if len(userIDs) > 0 {
		var users []models.User
		if err := c.db.Where("user_id IN ?", userIDs).Select("user_id", "username").Find(&users).Error; err == nil {
			for _, u := range users {
				usernameMap[u.ID] = u.Username
			}
		}
	}

	// 构建带 username 的响应
	type taskWithUsername struct {
		models.WorkspaceTask
		CreatedByUsername string `json:"created_by_username"`
	}
	tasksResp := make([]taskWithUsername, len(tasks))
	for i, t := range tasks {
		tasksResp[i].WorkspaceTask = t
		if t.CreatedBy != nil {
			tasksResp[i].CreatedByUsername = usernameMap[*t.CreatedBy]
		}
	}

	ctx.JSON(http.StatusOK, gin.H{
		"tasks":         tasksResp,
		"total":         total,
		"page":          page,
		"page_size":     pageSize,
		"pages":         (int(total) + pageSize - 1) / pageSize,
		"filter_counts": filterCounts,
	})
}

// applySearchAndTimeFilters 应用搜索和时间范围过滤的辅助函数
func applySearchAndTimeFilters(search, startDate, endDate string) func(*gorm.DB) *gorm.DB {
	return func(db *gorm.DB) *gorm.DB {
		if search != "" {
			searchPattern := "%" + search + "%"
			db = db.Where(
				"description LIKE ? OR CAST(id AS TEXT) LIKE ? OR task_type LIKE ?",
				searchPattern, searchPattern, searchPattern,
			)
		}
		if startDate != "" {
			if startTime, err := time.Parse(time.RFC3339, startDate); err == nil {
				db = db.Where("created_at >= ?", startTime.Local())
			}
		}
		if endDate != "" {
			if endTime, err := time.Parse(time.RFC3339, endDate); err == nil {
				db = db.Where("created_at <= ?", endTime.Local())
			}
		}
		return db
	}
}

// GetTaskLogs 获取任务日志
// @Summary 获取任务日志
// @Description 获取任务的执行日志
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "成功返回日志"
// @Failure 400 {object} map[string]interface{} "无效的任务ID"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/logs [get]
// @Security Bearer
func (c *WorkspaceTaskController) GetTaskLogs(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	logs, err := c.executor.GetTaskLogs(uint(taskID))
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch logs"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"total": len(logs),
	})
}

// ConfirmApply 确认执行Apply
// @Summary 确认执行Apply
// @Description 确认执行Plan+Apply任务的Apply阶段
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Param request body object true "Apply描述"
// @Success 200 {object} map[string]interface{} "Apply已加入队列"
// @Failure 400 {object} map[string]interface{} "请求参数无效或任务状态不正确"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Failure 409 {object} map[string]interface{} "资源已变更"
// @Failure 500 {object} map[string]interface{} "更新失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/confirm-apply [post]
// @Security Bearer
func (c *WorkspaceTaskController) ConfirmApply(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 解析请求体
	var req struct {
		ApplyDescription string `json:"apply_description"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "apply_description is required"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 获取任务
	var task models.WorkspaceTask
	if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).
		First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 验证任务类型
	if task.TaskType != models.TaskTypePlanAndApply {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Only plan_and_apply tasks can be confirmed",
		})
		return
	}

	// 【新增】如果plan_task_id为空，自动设置为任务自身ID（防御性编程）
	if task.PlanTaskID == nil {
		log.Printf("[WARN] Task %d plan_task_id is nil, auto-setting to self", task.ID)
		task.PlanTaskID = &task.ID
		// 立即保存到数据库
		if err := c.db.Model(&task).Update("plan_task_id", task.ID).Error; err != nil {
			log.Printf("[ERROR] Failed to set plan_task_id for task %d: %v", task.ID, err)
			ctx.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to set plan_task_id",
			})
			return
		}
		log.Printf("[INFO] Task %d plan_task_id auto-fixed to %d", task.ID, task.ID)
	}

	// 验证任务状态
	if task.Status != models.TaskStatusApplyPending {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":          "Task is not in apply_pending status",
			"current_status": task.Status,
		})
		return
	}

	// 验证资源版本快照（使用新的快照验证方法）
	// 创建一个简单的logger用于验证过程
	stream := c.streamManager.GetOrCreate(task.ID)
	logger := services.NewTerraformLoggerWithLevelAndMode(stream, "info", false)

	if err := c.executor.ValidateResourceVersionSnapshot(&task, logger); err != nil {
		c.streamManager.Close(task.ID)
		ctx.JSON(http.StatusConflict, gin.H{
			"error":   "Resources have changed since plan",
			"details": err.Error(),
		})
		return
	}
	c.streamManager.Close(task.ID)

	// 获取当前用户ID（用于审计）
	userID, exists := ctx.Get("user_id")
	if exists {
		uid := userID.(string)
		task.ApplyConfirmedBy = &uid
		now := time.Now()
		task.ApplyConfirmedAt = &now
		log.Printf("[ConfirmApply] Task %d confirmed by user %s at %v", task.ID, uid, now)
	} else {
		log.Printf("[WARN] ConfirmApply: No user_id in context for task %d", task.ID)
	}

	// 更新任务
	task.ApplyDescription = req.ApplyDescription
	task.Status = models.TaskStatusApplyPending
	task.Stage = "apply_pending"
	// 设置 PlanTaskID 指向自己（plan_and_apply 任务的 plan 数据在自己身上）
	task.PlanTaskID = &task.ID

	if err := c.db.Save(&task).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update task"})
		return
	}

	// 使用专门的ExecuteConfirmedApply方法来执行已确认的apply任务
	// 这个方法会验证任务已被确认，并直接执行，不依赖GetNextExecutableTask
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[PANIC] ExecuteConfirmedApply panicked for workspace %s: %v", workspace.WorkspaceID, r)
			}
		}()

		// 添加重试机制：最多重试3次，每次间隔递增
		maxRetries := 3
		for attempt := 0; attempt <= maxRetries; attempt++ {
			if attempt > 0 {
				// 指数退避：1s, 2s, 4s
				waitTime := time.Duration(1<<uint(attempt-1)) * time.Second
				log.Printf("[TaskQueue] Retry attempt %d/%d for confirmed apply task %d after %v", attempt, maxRetries, task.ID, waitTime)
				time.Sleep(waitTime)
			}

			err := c.queueManager.ExecuteConfirmedApply(workspace.WorkspaceID, task.ID)
			if err == nil {
				// 成功，退出重试循环
				log.Printf("[TaskQueue] Successfully triggered confirmed apply execution for task %d (attempt %d)", task.ID, attempt+1)
				return
			}

			log.Printf("[ERROR] Failed to execute confirmed apply for task %d (attempt %d/%d): %v", task.ID, attempt+1, maxRetries+1, err)

			// 如果是最后一次尝试，记录严重错误
			if attempt == maxRetries {
				log.Printf("[CRITICAL] All %d attempts failed to execute confirmed apply for task %d", maxRetries+1, task.ID)
			}
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Apply queued for execution",
		"task":    task,
	})
}

// CancelPreviousTasks 取消当前任务之前的所有等待任务
// @Summary 取消之前的等待任务
// @Description 取消当前任务之前的所有等待中的任务
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "取消成功"
// @Failure 400 {object} map[string]interface{} "无效的参数或任务状态不正确"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Failure 500 {object} map[string]interface{} "取消失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/cancel-previous [post]
// @Security Bearer
func (c *WorkspaceTaskController) CancelPreviousTasks(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	// 获取当前任务
	var currentTask models.WorkspaceTask
	if err := c.db.First(&currentTask, taskID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 只允许对pending状态的任务执行此操作
	if currentTask.Status != models.TaskStatusPending {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Only pending tasks can cancel previous tasks"})
		return
	}

	// 查找所有在当前任务之前创建的需要取消的任务
	// 包括：pending, apply_pending, plan_completed（等待Apply确认）
	// 不包括：running（正在执行）, requires_approval（需要人工确认）
	var previousTasks []models.WorkspaceTask
	if err := c.db.Where("workspace_id = ? AND id < ? AND status IN ?",
		workspace.WorkspaceID, taskID, []string{"pending", "apply_pending", "plan_completed"}).
		Find(&previousTasks).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to find previous tasks"})
		return
	}

	// 取消所有之前的任务
	cancelledCount := 0
	needUnlockWorkspace := false
	for _, task := range previousTasks {
		task.Status = models.TaskStatusCancelled
		task.CompletedAt = timePtr(time.Now())
		task.ErrorMessage = "Cancelled by user to prioritize later task"

		if err := c.db.Save(&task).Error; err == nil {
			cancelledCount++
			// 检查是否有 plan_and_apply 任务被取消，需要解锁 workspace
			if task.TaskType == models.TaskTypePlanAndApply {
				needUnlockWorkspace = true
			}
		}
	}

	// 如果有 plan_and_apply 任务被取消，检查并解锁 workspace
	if needUnlockWorkspace && workspace.IsLocked {
		// 检查锁定原因是否与被取消的任务相关
		for _, task := range previousTasks {
			if task.TaskType == models.TaskTypePlanAndApply {
				expectedLockReason := fmt.Sprintf("Locked for apply (task #%d)", task.ID)
				if strings.Contains(workspace.LockReason, expectedLockReason) || strings.Contains(workspace.LockReason, fmt.Sprintf("task #%d", task.ID)) {
					workspace.IsLocked = false
					workspace.LockedBy = nil
					workspace.LockedAt = nil
					workspace.LockReason = ""
					if err := c.db.Save(&workspace).Error; err != nil {
						log.Printf("[CancelPreviousTasks] Failed to unlock workspace %s: %v", workspace.WorkspaceID, err)
					} else {
						log.Printf("[CancelPreviousTasks] Workspace %s unlocked after cancelling task %d", workspace.WorkspaceID, task.ID)
					}
					break // 只需要解锁一次
				}
			}
		}
	}

	// 取消之前的任务后，尝试执行当前任务
	go func() {
		if err := c.queueManager.TryExecuteNextTask(workspace.WorkspaceID); err != nil {
			log.Printf("Failed to start task execution after cancelling previous tasks: %v", err)
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"message":         "Previous tasks cancelled successfully",
		"cancelled_count": cancelledCount,
	})
}

// CancelTask 取消任务
// @Summary 取消任务
// @Description 取消指定的任务
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path int true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "取消成功"
// @Failure 400 {object} map[string]interface{} "无效的任务ID或任务已完成"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Failure 500 {object} map[string]interface{} "取消失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/cancel [post]
// @Security Bearer
func (c *WorkspaceTaskController) CancelTask(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 只能取消未完成的任务（不能取消success、applied、failed、cancelled）
	if task.Status == models.TaskStatusSuccess ||
		task.Status == models.TaskStatusApplied ||
		task.Status == models.TaskStatusFailed ||
		task.Status == models.TaskStatusCancelled {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot cancel completed, failed or already cancelled tasks",
		})
		return
	}

	// 【安全】applying 阶段取消可能导致 Terraform state 损坏，需要 force 确认
	if task.Stage == "applying" {
		forceStr := ctx.Query("force")
		if forceStr != "true" {
			ctx.JSON(http.StatusConflict, gin.H{
				"error":          "Task is currently applying. Cancelling during apply may corrupt Terraform state. Use force=true to confirm.",
				"requires_force": true,
				"stage":          task.Stage,
			})
			return
		}
		log.Printf("[CancelTask]  Force cancelling task %d during apply stage (may cause state corruption)", taskID)
	}

	// 如果任务正在Agent上运行，发送取消信号
	if task.Status == models.TaskStatusRunning && task.AgentID != nil {
		// 获取Agent信息 - 注意：agent_id是主键，不是id
		var agent models.Agent
		if err := c.db.Where("agent_id = ?", *task.AgentID).First(&agent).Error; err == nil {
			// 发送取消信号到agent via C&C channel
			if c.agentCCHandler != nil {
				if err := c.agentCCHandler.CancelTaskOnAgent(agent.AgentID, uint(taskID)); err != nil {
					log.Printf("[CancelTask] Failed to send cancel signal to agent %s: %v", agent.AgentID, err)
					// 即使agent通知失败也继续更新数据库
				} else {
					log.Printf("[CancelTask]  Sent cancel signal to agent %s for task %d", agent.AgentID, taskID)
				}
			} else {
				log.Printf("[CancelTask] ❌ agentCCHandler is nil, cannot send cancel signal to agent")
			}
		} else {
			log.Printf("[CancelTask] ❌ Failed to get agent info for task %d (agent_id=%s): %v", taskID, *task.AgentID, err)
		}
	}

	// Cancel the execution context for locally-running tasks so that
	// long-running waits (e.g. RunTask callback polling) abort promptly.
	if task.Status == models.TaskStatusRunning {
		c.queueManager.CancelTaskExecution(uint(taskID))
	}

	// 从OutputStreamManager获取当前日志（如果任务正在运行）
	stream := c.streamManager.Get(uint(taskID))
	if stream != nil {
		bufferedLogs := stream.GetBufferedLogs()

		if bufferedLogs != "" {
			// 根据任务类型保存到对应字段
			if task.TaskType == models.TaskTypePlan || task.TaskType == models.TaskTypePlanAndApply {
				task.PlanOutput = bufferedLogs
				log.Printf("Saved %d bytes of plan logs for cancelled task %d", len(bufferedLogs), taskID)
			} else if task.TaskType == models.TaskTypeApply {
				task.ApplyOutput = bufferedLogs
				log.Printf("Saved %d bytes of apply logs for cancelled task %d", len(bufferedLogs), taskID)
			}
		}
	}

	// 更新任务状态
	task.Status = models.TaskStatusCancelled
	task.CompletedAt = timePtr(time.Now())
	task.ErrorMessage = "Task cancelled by user"

	if err := c.db.Save(&task).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
		return
	}

	// 如果任务是 apply_pending 或 plan_completed 状态，需要解锁 Workspace
	// 因为 Plan 完成后会自动锁定 Workspace，取消任务时需要解锁
	if task.TaskType == models.TaskTypePlanAndApply {
		var workspace models.Workspace
		if err := c.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; err == nil {
			if workspace.IsLocked {
				// 检查锁定原因是否与当前任务相关
				expectedLockReason := fmt.Sprintf("Locked for apply (task #%d)", task.ID)
				if strings.Contains(workspace.LockReason, expectedLockReason) || strings.Contains(workspace.LockReason, fmt.Sprintf("task #%d", task.ID)) {
					workspace.IsLocked = false
					workspace.LockedBy = nil
					workspace.LockedAt = nil
					workspace.LockReason = ""
					if err := c.db.Save(&workspace).Error; err != nil {
						log.Printf("[CancelTask] Failed to unlock workspace %s: %v", task.WorkspaceID, err)
					} else {
						log.Printf("[CancelTask] Workspace %s unlocked after cancelling task %d", task.WorkspaceID, task.ID)
					}
				}
			}
		}
	}

	// 发送任务取消通知
	go func() {
		if err := c.notificationSender.TriggerNotifications(
			context.Background(),
			task.WorkspaceID,
			models.NotificationEventTaskCancelled,
			&task,
		); err != nil {
			log.Printf("[Notification] Failed to send task_cancelled notification for task %d: %v", task.ID, err)
		}
	}()

	// 任务取消后，尝试执行下一个任务
	go func() {
		if err := c.queueManager.TryExecuteNextTask(task.WorkspaceID); err != nil {
			log.Printf("Failed to start next task after cancellation: %v", err)
		}
	}()

	ctx.JSON(http.StatusOK, gin.H{
		"message": "Task cancelled successfully",
		"task":    task,
	})
}

// timePtr 返回时间指针
func timePtr(t time.Time) *time.Time {
	return &t
}

// RetryStateSave 重试State保存
// @Summary 重试State保存
// @Description 重试保存失败的State文件
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "State保存成功"
// @Failure 400 {object} map[string]interface{} "任务不是State保存失败状态"
// @Failure 404 {object} map[string]interface{} "任务或备份文件不存在"
// @Failure 500 {object} map[string]interface{} "保存失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/retry-state-save [post]
// @Security Bearer
func (c *WorkspaceTaskController) RetryStateSave(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	var task models.WorkspaceTask
	if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).
		First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 检查是否是State保存失败的任务
	if !strings.Contains(task.ErrorMessage, "state save failed") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Task is not in state save failed status",
		})
		return
	}

	// 从错误信息中提取备份路径
	backupPath := extractBackupPath(task.ErrorMessage)
	if backupPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "Cannot find backup path in error message",
			"details": "Error message format may be incorrect or backup path is missing",
		})
		return
	}

	// 检查备份文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error":       "Backup file not found",
			"backup_path": backupPath,
			"suggestion":  "The backup file may have been deleted or the backup directory was not created successfully. Please check the backup directory permissions.",
		})
		return
	}

	// 读取备份文件
	stateData, err := os.ReadFile(backupPath)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error":       fmt.Sprintf("Failed to read backup file: %v", err),
			"backup_path": backupPath,
		})
		return
	}

	// 重新保存到数据库
	if err := c.executor.SaveStateToDatabase(&workspace, &task, stateData); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save state: %v", err),
		})
		return
	}

	// 更新任务状态
	task.Status = models.TaskStatusSuccess
	task.ErrorMessage = ""
	c.db.Save(&task)

	// 解锁workspace
	workspace.IsLocked = false
	workspace.LockedBy = nil
	workspace.LockedAt = nil
	workspace.LockReason = ""
	c.db.Save(&workspace)

	ctx.JSON(http.StatusOK, gin.H{
		"message": "State saved successfully, workspace unlocked",
		"task":    task,
	})
}

// DownloadStateBackup 下载State备份
// @Summary 下载State备份文件
// @Description 下载任务的State备份文件
// @Tags Workspace Task
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {file} file "State备份文件"
// @Failure 400 {object} map[string]interface{} "无法找到备份路径"
// @Failure 404 {object} map[string]interface{} "任务或备份文件不存在"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/state-backup [get]
// @Security Bearer
func (c *WorkspaceTaskController) DownloadStateBackup(ctx *gin.Context) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return
	}

	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取workspace (支持语义化ID和数字ID)
	var workspace models.Workspace
	err = c.db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := c.db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return
		}
	}

	var task models.WorkspaceTask
	if err := c.db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).
		First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 从错误信息中提取备份路径
	backupPath := extractBackupPath(task.ErrorMessage)
	if backupPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Cannot find backup path in error message",
		})
		return
	}

	// 检查文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error": "Backup file not found",
		})
		return
	}

	// 返回文件
	ctx.Header("Content-Disposition", fmt.Sprintf("attachment; filename=terraform_%d.tfstate", taskID))
	ctx.File(backupPath)
}

// extractBackupPath 从错误信息中提取备份路径
func extractBackupPath(errorMessage string) string {
	// "backup at: /var/backup/states/ws_10_task_63_1760251780.tfstate"
	parts := strings.Split(errorMessage, "backup at: ")
	if len(parts) > 1 {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

// CreateComment 添加任务评论
// @Summary 添加任务评论
// @Description 为任务添加评论
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Param request body object true "评论内容"
// @Success 201 {object} map[string]interface{} "评论创建成功"
// @Failure 400 {object} map[string]interface{} "请求参数无效或评论数量超限"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Failure 500 {object} map[string]interface{} "创建失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/comments [post]
// @Security Bearer
func (c *WorkspaceTaskController) CreateComment(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 获取当前用户信息（从JWT中间件）
	userID, exists := ctx.Get("user_id")
	var uid *string
	if exists {
		u := userID.(string)
		uid = &u
	}

	// 解析请求体
	var req struct {
		Comment    string `json:"comment" binding:"required"`
		ActionType string `json:"action_type"` // comment, confirm_apply, cancel, cancel_previous
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Comment is required"})
		return
	}

	// 验证任务是否存在（task_id是唯一的，不需要workspace验证）
	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 检查评论数量限制（最多20条）
	var commentCount int64
	c.db.Model(&models.TaskComment{}).Where("task_id = ?", taskID).Count(&commentCount)
	if commentCount >= 20 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Maximum 20 comments allowed per task",
		})
		return
	}

	// 创建评论
	comment := &models.TaskComment{
		TaskID:     uint(taskID),
		UserID:     uid,
		Username:   "User", // 默认用户名，实际应该从用户系统获取
		Comment:    req.Comment,
		ActionType: req.ActionType,
		CreatedAt:  time.Now(),
	}

	if err := c.db.Create(comment).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create comment"})
		return
	}

	ctx.JSON(http.StatusCreated, gin.H{
		"message": "Comment created successfully",
		"comment": comment,
	})
}

// GetComments 获取任务评论列表
// @Summary 获取任务评论列表
// @Description 获取任务的所有评论
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "工作空间ID"
// @Param task_id path int true "任务ID"
// @Success 200 {object} map[string]interface{} "成功返回评论列表"
// @Failure 400 {object} map[string]interface{} "无效的参数"
// @Failure 404 {object} map[string]interface{} "任务不存在"
// @Failure 500 {object} map[string]interface{} "获取失败"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/comments [get]
// @Security Bearer
func (c *WorkspaceTaskController) GetComments(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 验证任务是否存在（task_id是唯一的，不需要workspace验证）
	var task models.WorkspaceTask
	if err := c.db.First(&task, taskID).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return
	}

	// 获取评论列表（按时间倒序）
	var comments []models.TaskComment
	if err := c.db.Where("task_id = ?", taskID).
		Order("created_at DESC").
		Find(&comments).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to fetch comments"})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"comments": comments,
		"total":    len(comments),
	})
}

// createTaskSnapshot 在任务创建时立即创建快照
// 这样即使任务被取消或失败，快照也会存在，可用于审计和调试
func createTaskSnapshot(db *gorm.DB, task *models.WorkspaceTask, workspace *models.Workspace) error {
	snapshotTime := time.Now()

	// 1. 快照资源版本号
	var resources []models.WorkspaceResource
	if err := db.Where("workspace_id = ? AND is_active = true", workspace.WorkspaceID).
		Find(&resources).Error; err != nil {
		return fmt.Errorf("failed to get resources: %w", err)
	}

	// 加载每个资源的CurrentVersion
	for i := range resources {
		if resources[i].CurrentVersionID != nil {
			var version models.ResourceCodeVersion
			if err := db.First(&version, *resources[i].CurrentVersionID).Error; err == nil {
				resources[i].CurrentVersion = &version
			}
		}
	}

	resourceVersions := make(map[string]interface{})
	for _, r := range resources {
		if r.CurrentVersion != nil {
			// 注意：version_id 应该存储 resource_code_versions.id（用于后续查询）
			// version 存储实际的版本号（用于显示）
			// 但在验证时，我们需要通过 resource.ID 和 version.ID 来查询
			resourceVersions[r.ResourceID] = map[string]interface{}{
				"resource_db_id": r.ID,                     // workspace_resources.id（数字ID）
				"version_id":     r.CurrentVersion.ID,      // resource_code_versions.id
				"version":        r.CurrentVersion.Version, // 版本号（用于显示）
			}
		}
	}

	// 2. 快照变量（只保存variable_id和version引用）
	// 只获取最新版本为未删除状态的变量
	var variables []models.WorkspaceVariable
	if err := db.Raw(`
		SELECT wv.*
		FROM workspace_variables wv
		WHERE wv.workspace_id = ? 
		  AND wv.is_deleted = false
		  AND wv.version = (
			SELECT MAX(version)
			FROM workspace_variables
			WHERE workspace_id = wv.workspace_id 
			  AND variable_id = wv.variable_id
			  AND is_deleted = false
		  )
	`, workspace.WorkspaceID).Scan(&variables).Error; err != nil {
		return fmt.Errorf("failed to get latest non-deleted variables: %w", err)
	}

	// 构建变量快照：只保存必要字段（workspace_id, variable_id, version, variable_type）
	// 使用 map 而不是结构体，避免 JSON 序列化包含零值字段
	variableSnapshots := make([]map[string]interface{}, 0, len(variables))
	for _, v := range variables {
		variableSnapshots = append(variableSnapshots, map[string]interface{}{
			"workspace_id":  v.WorkspaceID,
			"variable_id":   v.VariableID,
			"version":       v.Version,
			"variable_type": string(v.VariableType),
		})
	}

	// 3. 快照Provider配置（模板模式下动态解析，确保使用最新模板数据）
	providerConfig := workspace.ProviderConfig
	templateIDs := workspace.ProviderTemplateIDs.GetTemplateIDs()
	if len(templateIDs) > 0 {
		ptService := services.NewProviderTemplateService(db)
		resolved, err := ptService.ResolveProviderConfig(templateIDs, workspace.ProviderOverrides.GetOverridesMap())
		if err != nil {
			return fmt.Errorf("failed to resolve provider config from templates: %w", err)
		}
		if resolved != nil {
			providerConfig = models.JSONB(resolved)
		}
	}

	// 4. 序列化变量快照为JSON
	variablesJSON, err := json.Marshal(variableSnapshots)
	if err != nil {
		return fmt.Errorf("failed to marshal variable snapshots: %w", err)
	}

	// 【调试】打印实际序列化的JSON
	log.Printf("[DEBUG] Task %d snapshot JSON (first 200 chars): %s", task.ID, string(variablesJSON)[:min(200, len(variablesJSON))])
	log.Printf("[DEBUG] Task %d snapshot JSON length: %d bytes", task.ID, len(variablesJSON))

	// 5. 保存快照到task（使用原始SQL确保JSON数组格式正确）
	resourceVersionsJSON, err := json.Marshal(models.JSONB(resourceVersions))
	if err != nil {
		return fmt.Errorf("failed to marshal resource versions: %w", err)
	}

	providerConfigJSON, err := json.Marshal(models.JSONB(providerConfig))
	if err != nil {
		return fmt.Errorf("failed to marshal provider config: %w", err)
	}

	if err := db.Exec(`
		UPDATE workspace_tasks 
		SET snapshot_resource_versions = ?::jsonb,
		    snapshot_variables = ?::jsonb,
		    snapshot_provider_config = ?::jsonb,
		    snapshot_created_at = ?
		WHERE id = ?
	`, resourceVersionsJSON, variablesJSON, providerConfigJSON, snapshotTime, task.ID).Error; err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	log.Printf("[DEBUG] Snapshot created for task %d: %d resources, %d variable references",
		task.ID, len(resourceVersions), len(variableSnapshots))

	return nil
}
