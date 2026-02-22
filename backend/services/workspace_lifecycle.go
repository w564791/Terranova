package services

import (
	"errors"
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// WorkspaceLifecycleService 工作空间生命周期服务
type WorkspaceLifecycleService struct {
	db *gorm.DB
}

// NewWorkspaceLifecycleService 创建生命周期服务实例
func NewWorkspaceLifecycleService(db *gorm.DB) *WorkspaceLifecycleService {
	return &WorkspaceLifecycleService{db: db}
}

// StateTransition 状态转换规则
type StateTransition struct {
	From    models.WorkspaceState
	To      models.WorkspaceState
	Allowed bool
	Reason  string
}

// 状态转换规则表
var stateTransitions = map[models.WorkspaceState]map[models.WorkspaceState]bool{
	models.WorkspaceStateCreated: {
		models.WorkspaceStatePlanning: true,
	},
	models.WorkspaceStatePlanning: {
		models.WorkspaceStatePlanDone: true,
		models.WorkspaceStateFailed:   true,
	},
	models.WorkspaceStatePlanDone: {
		models.WorkspaceStateWaitingApply: true,
		models.WorkspaceStatePlanning:     true, // 允许重新Plan
	},
	models.WorkspaceStateWaitingApply: {
		models.WorkspaceStateApplying:  true,
		models.WorkspaceStatePlanning:  true, // 允许重新Plan
		models.WorkspaceStateCompleted: true, // Plan-only模式直接完成
	},
	models.WorkspaceStateApplying: {
		models.WorkspaceStateCompleted: true,
		models.WorkspaceStateFailed:    true,
	},
	models.WorkspaceStateFailed: {
		models.WorkspaceStatePlanning: true, // 允许重试
	},
	models.WorkspaceStateCompleted: {
		models.WorkspaceStatePlanning: true, // 允许重新Plan
	},
}

// CanTransition 检查状态转换是否允许
func (s *WorkspaceLifecycleService) CanTransition(from, to models.WorkspaceState) (bool, string) {
	if transitions, ok := stateTransitions[from]; ok {
		if allowed, exists := transitions[to]; exists && allowed {
			return true, ""
		}
	}
	return false, fmt.Sprintf("不允许从 %s 转换到 %s", from, to)
}

// TransitionState 执行状态转换
func (s *WorkspaceLifecycleService) TransitionState(workspaceID string, newState models.WorkspaceState) error {
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return fmt.Errorf("workspace不存在: %w", err)
	}

	// 检查是否锁定
	if workspace.IsLocked {
		return errors.New("workspace已锁定，无法执行状态转换")
	}

	// 检查状态转换是否允许
	allowed, reason := s.CanTransition(workspace.State, newState)
	if !allowed {
		return fmt.Errorf("状态转换失败: %s", reason)
	}

	// 执行状态转换
	if err := s.db.Model(&workspace).Update("state", newState).Error; err != nil {
		return fmt.Errorf("更新状态失败: %w", err)
	}

	return nil
}

// StartPlan 开始Plan任务
func (s *WorkspaceLifecycleService) StartPlan(workspaceID string, userID string) (*models.WorkspaceTask, error) {
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace不存在: %w", err)
	}

	// 检查是否锁定
	if workspace.IsLocked {
		return nil, errors.New("workspace已锁定，无法执行Plan")
	}

	// 检查当前状态是否允许Plan
	validStates := []models.WorkspaceState{
		models.WorkspaceStateCreated,
		models.WorkspaceStatePlanDone,
		models.WorkspaceStateWaitingApply,
		models.WorkspaceStateFailed,
		models.WorkspaceStateCompleted,
	}

	isValid := false
	for _, state := range validStates {
		if workspace.State == state {
			isValid = true
			break
		}
	}

	if !isValid {
		return nil, fmt.Errorf("当前状态 %s 不允许执行Plan", workspace.State)
	}

	// 转换状态为Planning
	if err := s.TransitionState(workspaceID, models.WorkspaceStatePlanning); err != nil {
		return nil, err
	}

	// 创建Plan任务
	task := &models.WorkspaceTask{
		WorkspaceID:   workspace.WorkspaceID, // 使用语义化ID
		TaskType:      models.TaskTypePlan,
		Status:        models.TaskStatusPending,
		ExecutionMode: workspace.ExecutionMode,
		// AgentID 将由 TaskQueueManager 在分配任务时设置
		MaxRetries:    workspace.MaxRetries,
		CreatedBy:     &userID,
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("创建Plan任务失败: %w", err)
	}

	return task, nil
}

// CompletePlan 完成Plan任务
func (s *WorkspaceLifecycleService) CompletePlan(taskID uint, success bool, output string, errorMsg string) error {
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("任务不存在: %w", err)
	}

	// 获取workspace的语义化ID
	var workspace models.Workspace
	if err := s.db.Select("workspace_id").First(&workspace, task.WorkspaceID).Error; err != nil {
		return fmt.Errorf("获取workspace失败: %w", err)
	}

	// 更新任务状态
	now := time.Now()
	updates := map[string]interface{}{
		"completed_at": now,
		"plan_output":  output,
	}

	if success {
		updates["status"] = models.TaskStatusSuccess
		// 转换workspace状态为PlanDone
		if err := s.TransitionState(workspace.WorkspaceID, models.WorkspaceStatePlanDone); err != nil {
			return err
		}
	} else {
		updates["status"] = models.TaskStatusFailed
		updates["error_message"] = errorMsg
		// 转换workspace状态为Failed
		if err := s.TransitionState(workspace.WorkspaceID, models.WorkspaceStateFailed); err != nil {
			return err
		}
	}

	// 计算执行时长
	if task.StartedAt != nil {
		duration := int(now.Sub(*task.StartedAt).Seconds())
		updates["duration"] = duration
	}

	if err := s.db.Model(&task).Updates(updates).Error; err != nil {
		return fmt.Errorf("更新任务状态失败: %w", err)
	}

	return nil
}

// StartApply / CompleteApply — 已废弃，不再使用。
// 当前平台没有独立的 Apply 流程，所有 Apply 均通过 plan_and_apply + ConfirmApply 两阶段工作流完成。
// 这两个函数没有任何调用方，属于死代码。
// func (s *WorkspaceLifecycleService) StartApply(workspaceID string, userID string) (*models.WorkspaceTask, error) { ... }
// func (s *WorkspaceLifecycleService) CompleteApply(taskID uint, success bool, output string, errorMsg string) error { ... }

// LockWorkspace 锁定workspace
func (s *WorkspaceLifecycleService) LockWorkspace(workspaceID string, userID string, reason string) error {
	now := time.Now()
	updates := map[string]interface{}{
		"is_locked":   true,
		"locked_by":   userID,
		"locked_at":   now,
		"lock_reason": reason,
	}

	if err := s.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(updates).Error; err != nil {
		return fmt.Errorf("锁定workspace失败: %w", err)
	}

	return nil
}

// UnlockWorkspace 解锁workspace
func (s *WorkspaceLifecycleService) UnlockWorkspace(workspaceID string) error {
	updates := map[string]interface{}{
		"is_locked":   false,
		"locked_by":   nil,
		"locked_at":   nil,
		"lock_reason": "",
	}

	if err := s.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(updates).Error; err != nil {
		return fmt.Errorf("解锁workspace失败: %w", err)
	}

	return nil
}

// GetWorkspaceState 获取workspace当前状态
func (s *WorkspaceLifecycleService) GetWorkspaceState(workspaceID string) (models.WorkspaceState, error) {
	var workspace models.Workspace
	if err := s.db.Select("state").Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return "", fmt.Errorf("获取workspace状态失败: %w", err)
	}
	return workspace.State, nil
}

// GetCurrentRun 获取当前正在运行或pending的任务
func (s *WorkspaceLifecycleService) GetCurrentRun(workspaceID string) (*models.WorkspaceTask, error) {
	// 先获取workspace的内部ID
	var workspace models.Workspace
	if err := s.db.Select("id").Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace不存在: %w", err)
	}

	var task models.WorkspaceTask
	err := s.db.Where("workspace_id = ? AND status IN ?", workspace.WorkspaceID, []models.TaskStatus{
		models.TaskStatusPending,
		models.TaskStatusRunning,
	}).Order("created_at DESC").First(&task).Error

	if err != nil {
		return nil, err
	}

	return &task, nil
}

// GetWorkspaceTasks 获取workspace的任务列表
func (s *WorkspaceLifecycleService) GetWorkspaceTasks(workspaceID string) ([]models.WorkspaceTask, error) {
	// 先获取workspace的内部ID
	var workspace models.Workspace
	if err := s.db.Select("id").Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace不存在: %w", err)
	}

	var tasks []models.WorkspaceTask
	if err := s.db.Where("workspace_id = ?", workspace.WorkspaceID).Order("created_at DESC").Find(&tasks).Error; err != nil {
		return nil, fmt.Errorf("获取任务列表失败: %w", err)
	}
	return tasks, nil
}

// GetWorkspaceTasksWithFilter 获取workspace的任务列表（支持过滤和分页）
func (s *WorkspaceLifecycleService) GetWorkspaceTasksWithFilter(workspaceID string, filter string, page, size int) ([]models.WorkspaceTask, int64, error) {
	// 先获取workspace的内部ID
	var workspace models.Workspace
	if err := s.db.Select("id").Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, 0, fmt.Errorf("workspace不存在: %w", err)
	}

	query := s.db.Where("workspace_id = ?", workspace.WorkspaceID)

	// 应用过滤器
	switch filter {
	case "needs_attention":
		// 需要关注：失败的或等待审批的
		query = query.Where("status IN ?", []models.TaskStatus{
			models.TaskStatusFailed,
			// 未来可以添加 waiting_approval 状态
		})
	case "errored":
		// 失败的任务
		query = query.Where("status = ?", models.TaskStatusFailed)
	case "running":
		// 正在运行的任务
		query = query.Where("status IN ?", []models.TaskStatus{
			models.TaskStatusPending,
			models.TaskStatusRunning,
		})
	case "on_hold":
		// 等待中的任务（pending）
		query = query.Where("status = ?", models.TaskStatusPending)
	case "success":
		// 成功的任务
		query = query.Where("status = ?", models.TaskStatusSuccess)
	case "all":
		// 所有任务，不添加额外过滤
	default:
		return nil, 0, fmt.Errorf("无效的过滤器: %s", filter)
	}

	// 获取总数
	var total int64
	if err := query.Model(&models.WorkspaceTask{}).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("获取任务总数失败: %w", err)
	}

	// 分页查询
	var tasks []models.WorkspaceTask
	offset := (page - 1) * size
	if err := query.Order("created_at DESC").Offset(offset).Limit(size).Find(&tasks).Error; err != nil {
		return nil, 0, fmt.Errorf("获取任务列表失败: %w", err)
	}

	return tasks, total, nil
}

// GetTask 获取任务详情
func (s *WorkspaceLifecycleService) GetTask(taskID uint) (*models.WorkspaceTask, error) {
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return nil, fmt.Errorf("获取任务详情失败: %w", err)
	}
	return &task, nil
}

// CancelTask 取消任务
func (s *WorkspaceLifecycleService) CancelTask(taskID uint) error {
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("任务不存在: %w", err)
	}

	// 只能取消pending或running状态的任务
	if task.Status != models.TaskStatusPending && task.Status != models.TaskStatusRunning {
		return fmt.Errorf("任务状态 %s 不允许取消", task.Status)
	}

	if err := s.db.Model(&task).Update("status", models.TaskStatusCancelled).Error; err != nil {
		return fmt.Errorf("取消任务失败: %w", err)
	}

	return nil
}
