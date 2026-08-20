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

// loadTaskInPathWorkspace resolves the workspace named by the route and then
// loads a task bound to that exact semantic workspace ID.  Route middleware
// authorizes the workspace path, but task IDs are global, so every nested
// task endpoint must also make this binding before reading or mutating a task.
// On failure it writes the HTTP response and callers must return immediately.
func loadTaskInPathWorkspace(db *gorm.DB, ctx *gin.Context, taskID uint) (*models.Workspace, *models.WorkspaceTask, bool) {
	workspaceIDParam := ctx.Param("id")
	if workspaceIDParam == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid workspace ID"})
		return nil, nil, false
	}

	var workspace models.Workspace
	err := db.Where("workspace_id = ?", workspaceIDParam).First(&workspace).Error
	if err != nil {
		if err := db.Where("id = ?", workspaceIDParam).First(&workspace).Error; err != nil {
			ctx.JSON(http.StatusNotFound, gin.H{"error": "Workspace not found"})
			return nil, nil, false
		}
	}

	var task models.WorkspaceTask
	if err := db.Where("id = ? AND workspace_id = ?", taskID, workspace.WorkspaceID).First(&task).Error; err != nil {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "Task not found"})
		return nil, nil, false
	}
	return &workspace, &task, true
}

// loadTaskInPathWorkspace is kept as a method for existing workspace task
// handlers. AI sub-resources use the shared helper above as well.
func (c *WorkspaceTaskController) loadTaskInPathWorkspace(ctx *gin.Context, taskID uint) (*models.Workspace, *models.WorkspaceTask, bool) {
	return loadTaskInPathWorkspace(c.db, ctx, taskID)
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
// @Summary Create plan task
// @Description Create a Terraform Plan task or Plan+Apply task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param request body object false "Task configuration (description and run_type optional)"
// @Success 201 {object} map[string]interface{} "Task created"
// @Failure 400 {object} map[string]interface{} "Invalid request"
// @Failure 401 {object} map[string]interface{} "Unauthorized"
// @Failure 404 {object} map[string]interface{} "Workspace not found"
// @Failure 500 {object} map[string]interface{} "Creation failed"
// @Router /api/v1/workspaces/{id}/tasks/plan [post]
// @Security BearerAuth
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
		Description        string  `json:"description"`
		RunType            string  `json:"run_type"`             // "plan" 或 "plan_and_apply"
		VariableSnapshotID *string `json:"variable_snapshot_id"` // 可选，API 用户可传已有 vsnap_id
		// Manifest Run: 当前用户草稿上传 (Run 按钮专用,只允许 plan,不允许 plan_and_apply)
		ExternalFiles []struct {
			Path       string `json:"path"`
			ContentB64 string `json:"content_b64"`
		} `json:"external_files,omitempty"`
	}
	if err := ctx.ShouldBindJSON(&req); err != nil {
		// 如果没有请求体，继续执行（description是可选的）
		req.Description = ""
		req.RunType = "plan" // 默认为plan
	}

	// external_files 仅允许 plan 任务(Run 按钮语义)
	if len(req.ExternalFiles) > 0 && req.RunType != "plan" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "external_files only supported for plan tasks (manifest Run)",
		})
		return
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
	// plan-only 任务不受锁影响（不修改 state）
	// plan_and_apply 任务在锁定时排队（不拒绝）
	if workspace.LockID != nil && req.RunType != "plan" {
		// plan_and_apply 任务：排队而非拒绝
		log.Printf("[TaskCreate] Workspace %s is locked, plan_and_apply task will be queued", workspace.WorkspaceID)
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

	// Variable snapshot: use provided vsnap_id or create new one
	var vsnapID *string
	if req.VariableSnapshotID != nil && *req.VariableSnapshotID != "" {
		// 必须属于当前 workspace（防跨 WS 变量/敏感值注入 A-3.2）
		var count int64
		c.db.Model(&models.VariableSnapshot{}).
			Where("vsnap_id = ? AND workspace_id = ?", *req.VariableSnapshotID, workspace.WorkspaceID).
			Count(&count)
		if count == 0 {
			ctx.JSON(http.StatusBadRequest, gin.H{"error": "variable_snapshot_id not found in this workspace"})
			return
		}
		vsnapID = req.VariableSnapshotID
	} else {
		// Auto-create snapshot
		snapshotSvc := services.NewVariableSnapshotService(c.db)
		var snapshotErr error
		vsnapID, _, snapshotErr = snapshotSvc.CreateSnapshot(workspace.WorkspaceID, &uid)
		if snapshotErr != nil {
			log.Printf("[WARN] Failed to create variable snapshot for workspace %s: %v", workspace.WorkspaceID, snapshotErr)
		}
	}

	// external_files (Manifest Run): 序列化为 JSONB 写入 task
	var externalFilesJSONB models.JSONB
	if len(req.ExternalFiles) > 0 {
		efs := make([]map[string]string, 0, len(req.ExternalFiles))
		for _, f := range req.ExternalFiles {
			efs = append(efs, map[string]string{
				"path":        f.Path,
				"content_b64": f.ContentB64,
			})
		}
		externalFilesJSONB = models.JSONB{"files": efs}
	}

	// Manifest deployment variable_overrides 快照: 任务创建时固化当时 active deployment 的
	// 应急覆盖(最高优先级),执行时 overlay。与 vsnap(varset/workspace 变量引用快照)互补。
	var overridesJSONB models.JSONB
	if _, extraOverrides, ovErr := services.NewVariableResolutionService(c.db).
		GetActiveDeploymentExtras(workspace.WorkspaceID); ovErr != nil {
		log.Printf("[WARN] resolve deployment overrides for %s failed: %v", workspace.WorkspaceID, ovErr)
	} else if len(extraOverrides) > 0 {
		overridesJSONB = make(models.JSONB, len(extraOverrides))
		for k, v := range extraOverrides {
			overridesJSONB[k] = v
		}
	}

	// 创建任务（只创建一个任务）
	task := &models.WorkspaceTask{
		WorkspaceID:        workspace.WorkspaceID,
		TaskType:           taskType,
		Status:             models.TaskStatusPending,
		ExecutionMode:      workspace.ExecutionMode,
		CreatedBy:          &uid,
		Stage:              "pending",
		Description:        req.Description,
		VariableSnapshotID: vsnapID,
		ExternalFiles:      externalFilesJSONB,
		VariableOverrides:  overridesJSONB,
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
// @Summary Get task detail
// @Description Get task detail by ID
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Task detail"
// @Failure 400 {object} map[string]interface{} "Invalid parameters"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Router /api/v1/workspaces/{id}/tasks/{task_id} [get]
// @Security BearerAuth
func (c *WorkspaceTaskController) GetTask(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	workspace, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	task := *taskPtr

	// 根据 workspace 配置决定是否排除 plan_json 和快照字段
	if !workspace.ShowUnchangedResources {
		var slim models.WorkspaceTask
		if err := c.db.Where("id = ? AND workspace_id = ?", task.ID, workspace.WorkspaceID).
			Omit("plan_json", "snapshot_resource_versions", "snapshot_provider_config").
			First(&slim).Error; err == nil {
			task = slim
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

	// 只在ShowUnchangedResources为true时包含plan_json等大字段
	if workspace.ShowUnchangedResources {
		taskResponse["plan_json"] = task.PlanJSON
		taskResponse["outputs"] = task.Outputs
		taskResponse["context"] = task.Context
		taskResponse["snapshot_resource_versions"] = task.SnapshotResourceVersions
		taskResponse["snapshot_provider_config"] = task.SnapshotProviderConfig
	}

	// Load snapshot variables from new variable_snapshots table
	taskResponse["snapshot_created_at"] = task.SnapshotCreatedAt
	if task.VariableSnapshotID != nil {
		var snapRefs []models.VariableSnapshot
		c.db.Where("vsnap_id = ?", *task.VariableSnapshotID).Find(&snapRefs)
		taskResponse["variable_snapshot_id"] = *task.VariableSnapshotID
		taskResponse["snapshot_variables"] = snapRefs
	} else {
		taskResponse["variable_snapshot_id"] = nil
	}

	ctx.JSON(http.StatusOK, gin.H{
		"task": taskResponse,
	})
}

// GetTasks 获取任务列表
// @Summary Get task list
// @Description Get workspace task list with pagination, search and filtering
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(10)
// @Param search query string false "Search keyword"
// @Param status query string false "Status filter"
// @Param task_type query string false "Task type filter"
// @Param start_date query string false "Start date (RFC3339)"
// @Param end_date query string false "End date (RFC3339)"
// @Param show_background query bool false "Include background tasks"
// @Success 200 {object} map[string]interface{} "Task list"
// @Failure 400 {object} map[string]interface{} "Invalid workspace ID"
// @Failure 500 {object} map[string]interface{} "Server error"
// @Router /api/v1/workspaces/{id}/tasks [get]
// @Security BearerAuth
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
// @Summary Get task logs
// @Description Get task execution logs
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Task logs"
// @Failure 400 {object} map[string]interface{} "Invalid task ID"
// @Failure 500 {object} map[string]interface{} "Fetch failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/logs [get]
// @Security BearerAuth
func (c *WorkspaceTaskController) GetTaskLogs(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 任务必须属于路径 workspace（防跨 WS 读日志）
	if _, _, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID)); !ok {
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
// @Summary Confirm apply
// @Description Confirm the apply stage of a Plan+Apply task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Param request body object true "Apply description"
// @Success 200 {object} map[string]interface{} "Apply queued"
// @Failure 400 {object} map[string]interface{} "Invalid request or incorrect task status"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Failure 409 {object} map[string]interface{} "Resources changed since plan"
// @Failure 500 {object} map[string]interface{} "Update failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/confirm-apply [post]
// @Security BearerAuth
func (c *WorkspaceTaskController) ConfirmApply(ctx *gin.Context) {
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

	workspace, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	task := *taskPtr
	_ = workspace

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

	if err := c.db.Omit("state_token_hash").Save(&task).Error; err != nil {
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
// @Summary Cancel previous pending tasks
// @Description Cancel all pending tasks before the specified task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Cancellation successful"
// @Failure 400 {object} map[string]interface{} "Invalid parameters or incorrect task status"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Failure 500 {object} map[string]interface{} "Cancellation failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/cancel-previous [post]
// @Security BearerAuth
func (c *WorkspaceTaskController) CancelPreviousTasks(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	workspace, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	currentTask := *taskPtr

	// 只允许对pending状态的任务执行此操作
	if currentTask.Status != models.TaskStatusPending {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Only pending tasks can cancel previous tasks"})
		return
	}

	// 查找所有在当前任务之前创建的需要取消的任务
	// 包括：pending, apply_pending, plan_completed（遗留兼容）, decision_required, waiting（各种等待状态）
	// 不包括：running（正在执行）
	var previousTasks []models.WorkspaceTask
	if err := c.db.Where("workspace_id = ? AND id < ? AND status IN ?",
		workspace.WorkspaceID, taskID, []string{"pending", "apply_pending", "plan_completed", "decision_required", "waiting"}).
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

		if err := c.db.Omit("state_token_hash").Save(&task).Error; err == nil {
			cancelledCount++
			// 检查是否有 plan_and_apply 任务被取消，需要解锁 workspace
			if task.TaskType == models.TaskTypePlanAndApply {
				needUnlockWorkspace = true
			}
		}
	}

	// 如果有 plan_and_apply 任务被取消，检查并解锁 workspace
	if needUnlockWorkspace && workspace.LockID != nil && workspace.LockInfo != nil {
		// Never clear a Terraform HTTP backend lock (has "Operation" key
		// like "OperationTypePlan" or "OperationTypeApply")
		if _, isHTTPLock := workspace.LockInfo["Operation"]; isHTTPLock {
			log.Printf("[CancelPreviousTasks] Workspace %s has active Terraform HTTP lock, not clearing", workspace.WorkspaceID)
		} else {
			// Platform-managed lock: check if it belongs to a cancelled task
			lockInfoStr := ""
			if info, ok := workspace.LockInfo["info"].(string); ok {
				lockInfoStr = info
			}
			for _, task := range previousTasks {
				if task.TaskType == models.TaskTypePlanAndApply {
					expectedLockReason := fmt.Sprintf("Locked for apply (task #%d)", task.ID)
					if strings.Contains(lockInfoStr, expectedLockReason) || strings.Contains(lockInfoStr, fmt.Sprintf("task #%d", task.ID)) {
						workspace.LockID = nil
						workspace.LockInfo = nil
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
	}

	// 没有任务被取消时返回错误
	if cancelledCount == 0 {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":           "No cancellable previous tasks found",
			"cancelled_count": 0,
		})
		return
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
// @Summary Cancel task
// @Description Cancel the specified task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Param force query bool false "Force cancel during apply stage"
// @Success 200 {object} map[string]interface{} "Cancellation successful"
// @Failure 400 {object} map[string]interface{} "Invalid task ID or task already completed"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Failure 409 {object} map[string]interface{} "Task is applying, requires force=true"
// @Failure 500 {object} map[string]interface{} "Cancellation failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/cancel [post]
// @Security BearerAuth
func (c *WorkspaceTaskController) CancelTask(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	_, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	task := *taskPtr

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

	if err := c.db.Omit("state_token_hash").Save(&task).Error; err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to cancel task"})
		return
	}

	// 如果任务是 apply_pending 或 plan_completed 状态，需要解锁 Workspace
	// 因为 Plan 完成后会自动锁定 Workspace，取消任务时需要解锁
	if task.TaskType == models.TaskTypePlanAndApply {
		var workspace models.Workspace
		if err := c.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; err == nil {
			if workspace.LockID != nil && workspace.LockInfo != nil {
				// Never clear a Terraform HTTP backend lock (has "Operation" key
				// like "OperationTypePlan" or "OperationTypeApply")
				if _, isHTTPLock := workspace.LockInfo["Operation"]; isHTTPLock {
					log.Printf("[CancelTask] Workspace %s has active Terraform HTTP lock, not clearing", task.WorkspaceID)
				} else {
					// Platform-managed lock: check if it belongs to this task
					lockInfoStr := ""
					if info, ok := workspace.LockInfo["info"].(string); ok {
						lockInfoStr = info
					}
					expectedLockReason := fmt.Sprintf("Locked for apply (task #%d)", task.ID)
					if strings.Contains(lockInfoStr, expectedLockReason) || strings.Contains(lockInfoStr, fmt.Sprintf("task #%d", task.ID)) {
						workspace.LockID = nil
						workspace.LockInfo = nil
						if err := c.db.Save(&workspace).Error; err != nil {
							log.Printf("[CancelTask] Failed to unlock workspace %s: %v", task.WorkspaceID, err)
						} else {
							log.Printf("[CancelTask] Workspace %s unlocked after cancelling task %d", task.WorkspaceID, task.ID)
						}
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
// @Summary Retry state save
// @Description Retry saving a failed state file. With HTTP state backend, checks if state already exists in DB first.
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "State saved successfully"
// @Failure 400 {object} map[string]interface{} "Task is not in state save failed status"
// @Failure 404 {object} map[string]interface{} "Task or backup file not found"
// @Failure 500 {object} map[string]interface{} "Save failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/retry-state-save [post]
// @Security BearerAuth
func (c *WorkspaceTaskController) RetryStateSave(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	workspace, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	task := *taskPtr

	// 检查是否是State保存失败的任务
	if !strings.Contains(task.ErrorMessage, "state save failed") {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error": "Task is not in state save failed status",
		})
		return
	}

	// Check if state already exists in DB (saved via HTTP state backend POST)
	taskIDUint := uint(taskID)
	var existingState models.WorkspaceStateVersion
	if err := c.db.Where("workspace_id = ? AND task_id = ?", workspace.WorkspaceID, taskIDUint).
		Order("version DESC").First(&existingState).Error; err == nil {
		// State already exists from HTTP state backend - just fix the task status
		log.Printf("[RetryStateSave] State already exists for task %d (version %d), marking task as success", taskID, existingState.Version)

		task.Status = models.TaskStatusSuccess
		task.ErrorMessage = ""
		c.db.Omit("state_token_hash").Save(&task)

		// NOTE: Do not clear workspace lock here. In HTTP backend mode, locks are
		// managed by Terraform itself. In non-HTTP mode, the executor handles
		// lock cleanup. RetryStateSave is a recovery endpoint and should not
		// be unlocking workspaces unconditionally.

		ctx.JSON(http.StatusOK, gin.H{
			"message": "State already saved via HTTP state backend, task status updated",
			"version": existingState.Version,
			"task":    task,
		})
		return
	}

	// Fallback: try to recover from backup file (legacy path)
	backupPath := extractBackupPath(task.ErrorMessage)
	if backupPath == "" {
		ctx.JSON(http.StatusBadRequest, gin.H{
			"error":   "State not found in DB and cannot find backup path in error message",
			"details": "Error message format may be incorrect or backup path is missing",
		})
		return
	}

	// 检查备份文件是否存在
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		ctx.JSON(http.StatusNotFound, gin.H{
			"error":       "State not found in DB and backup file not found",
			"backup_path": backupPath,
			"suggestion":  "The backup file may have been deleted or the backup directory was not created successfully.",
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
	if err := c.executor.SaveStateToDatabase(workspace, &task, stateData); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to save state: %v", err),
		})
		return
	}

	// 更新任务状态
	task.Status = models.TaskStatusSuccess
	task.ErrorMessage = ""
	c.db.Omit("state_token_hash").Save(&task)

	// NOTE: Do not clear workspace lock here. In HTTP backend mode, locks are
	// managed by Terraform itself. In non-HTTP mode, the executor handles
	// lock cleanup. RetryStateSave is a recovery endpoint and should not
	// be unlocking workspaces unconditionally.

	ctx.JSON(http.StatusOK, gin.H{
		"message": "State saved successfully from backup, task status updated",
		"task":    task,
	})
}

// DownloadStateBackup 下载State备份
// @Summary Download state backup
// @Description Download task state backup file
// @Tags Workspace Task
// @Accept json
// @Produce application/octet-stream
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {file} file "State backup file"
// @Failure 400 {object} map[string]interface{} "Backup path not found"
// @Failure 404 {object} map[string]interface{} "Task or backup file not found"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/state-backup [get]
// @Security BearerAuth
func (c *WorkspaceTaskController) DownloadStateBackup(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	_, taskPtr, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID))
	if !ok {
		return
	}
	task := *taskPtr

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
// @Summary Create task comment
// @Description Add a comment to a task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Param request body object true "Comment content"
// @Success 201 {object} map[string]interface{} "Comment created"
// @Failure 400 {object} map[string]interface{} "Invalid request or comment limit exceeded"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Failure 500 {object} map[string]interface{} "Creation failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/comments [post]
// @Security BearerAuth
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

	// 任务必须属于路径中的 workspace（防跨 WS IDOR）
	if _, _, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID)); !ok {
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
// @Summary Get task comments
// @Description Get all comments for a task
// @Tags Workspace Task
// @Accept json
// @Produce json
// @Param id path string true "Workspace ID"
// @Param task_id path int true "Task ID"
// @Success 200 {object} map[string]interface{} "Comment list"
// @Failure 400 {object} map[string]interface{} "Invalid parameters"
// @Failure 404 {object} map[string]interface{} "Task not found"
// @Failure 500 {object} map[string]interface{} "Fetch failed"
// @Router /api/v1/workspaces/{id}/tasks/{task_id}/comments [get]
// @Security BearerAuth
func (c *WorkspaceTaskController) GetComments(ctx *gin.Context) {
	taskID, err := strconv.ParseUint(ctx.Param("task_id"), 10, 32)
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "Invalid task ID"})
		return
	}

	// 任务必须属于路径中的 workspace（防跨 WS IDOR）
	if _, _, ok := c.loadTaskInPathWorkspace(ctx, uint(taskID)); !ok {
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

	// 2. 快照Provider配置（模板模式下动态解析，确保使用最新模板数据）
	// NOTE: Variable snapshots are now created BEFORE task creation via VariableSnapshotService
	providerConfig := workspace.ProviderConfig
	instances := workspace.ProviderInstances.GetProviderInstances()
	if len(instances) > 0 {
		ptService := services.NewProviderTemplateService(db)
		resolved, err := ptService.ResolveProviderConfigFromInstances(instances)
		if err != nil {
			return fmt.Errorf("failed to resolve provider config from templates: %w", err)
		}
		if resolved != nil {
			providerConfig = models.JSONB(resolved)
		}
	}

	// 3. 保存快照到task（使用原始SQL确保JSON格式正确）
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
		    snapshot_provider_config = ?::jsonb,
		    snapshot_created_at = ?
		WHERE id = ?
	`, resourceVersionsJSON, providerConfigJSON, snapshotTime, task.ID).Error; err != nil {
		return fmt.Errorf("failed to save snapshot: %w", err)
	}

	log.Printf("[DEBUG] Snapshot created for task %d: %d resources",
		task.ID, len(resourceVersions))

	return nil
}
