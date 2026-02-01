package services

import (
	"context"
	"fmt"
	"log"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// RunTriggerService 处理 workspace 之间的触发逻辑
type RunTriggerService struct {
	db *gorm.DB
}

// NewRunTriggerService 创建 RunTriggerService 实例
func NewRunTriggerService(db *gorm.DB) *RunTriggerService {
	return &RunTriggerService{db: db}
}

// GetRunTriggersBySource 获取源 workspace 配置的所有触发器
func (s *RunTriggerService) GetRunTriggersBySource(sourceWorkspaceID string) ([]models.RunTrigger, error) {
	var triggers []models.RunTrigger
	err := s.db.Where("source_workspace_id = ?", sourceWorkspaceID).
		Preload("TargetWorkspace").
		Find(&triggers).Error
	return triggers, err
}

// GetRunTriggersByTarget 获取目标 workspace 被哪些 workspace 触发
func (s *RunTriggerService) GetRunTriggersByTarget(targetWorkspaceID string) ([]models.RunTrigger, error) {
	var triggers []models.RunTrigger
	err := s.db.Where("target_workspace_id = ?", targetWorkspaceID).
		Preload("SourceWorkspace").
		Find(&triggers).Error
	return triggers, err
}

// CreateRunTrigger 创建触发器配置
func (s *RunTriggerService) CreateRunTrigger(trigger *models.RunTrigger) error {
	// 检查是否已存在相同的触发器
	var existing models.RunTrigger
	err := s.db.Where("source_workspace_id = ? AND target_workspace_id = ?",
		trigger.SourceWorkspaceID, trigger.TargetWorkspaceID).First(&existing).Error
	if err == nil {
		return fmt.Errorf("trigger already exists between these workspaces")
	}

	// 检查是否会形成循环触发
	if s.wouldCreateCycle(trigger.SourceWorkspaceID, trigger.TargetWorkspaceID) {
		return fmt.Errorf("creating this trigger would create a circular dependency")
	}

	return s.db.Create(trigger).Error
}

// UpdateRunTrigger 更新触发器配置
func (s *RunTriggerService) UpdateRunTrigger(id uint, updates map[string]interface{}) error {
	return s.db.Model(&models.RunTrigger{}).Where("id = ?", id).Updates(updates).Error
}

// DeleteRunTrigger 删除触发器配置
func (s *RunTriggerService) DeleteRunTrigger(id uint) error {
	return s.db.Delete(&models.RunTrigger{}, id).Error
}

// GetRunTrigger 获取单个触发器
func (s *RunTriggerService) GetRunTrigger(id uint) (*models.RunTrigger, error) {
	var trigger models.RunTrigger
	err := s.db.Preload("SourceWorkspace").Preload("TargetWorkspace").First(&trigger, id).Error
	if err != nil {
		return nil, err
	}
	return &trigger, nil
}

// wouldCreateCycle 检查是否会形成循环触发
func (s *RunTriggerService) wouldCreateCycle(sourceID, targetID string) bool {
	// 使用 BFS 检查从 targetID 出发是否能到达 sourceID
	visited := make(map[string]bool)
	queue := []string{targetID}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if current == sourceID {
			return true
		}

		if visited[current] {
			continue
		}
		visited[current] = true

		var triggers []models.RunTrigger
		s.db.Where("source_workspace_id = ? AND enabled = true", current).Find(&triggers)
		for _, t := range triggers {
			queue = append(queue, t.TargetWorkspaceID)
		}
	}

	return false
}

// PrepareTaskTriggerExecutions 为任务准备触发执行记录
// 在任务创建时调用，创建所有可能的触发执行记录
func (s *RunTriggerService) PrepareTaskTriggerExecutions(taskID uint, workspaceID string) error {
	// 获取该 workspace 配置的所有启用的触发器
	var triggers []models.RunTrigger
	err := s.db.Where("source_workspace_id = ? AND enabled = true", workspaceID).Find(&triggers).Error
	if err != nil {
		return err
	}

	// 为每个触发器创建执行记录
	for _, trigger := range triggers {
		execution := &models.TaskTriggerExecution{
			SourceTaskID: taskID,
			RunTriggerID: trigger.ID,
			Status:       models.TriggerStatusPending,
		}
		if err := s.db.Create(execution).Error; err != nil {
			log.Printf("[RunTrigger] Failed to create trigger execution for task %d, trigger %d: %v",
				taskID, trigger.ID, err)
		}
	}

	return nil
}

// GetTaskTriggerExecutions 获取任务的所有触发执行记录
func (s *RunTriggerService) GetTaskTriggerExecutions(taskID uint) ([]models.TaskTriggerExecution, error) {
	var executions []models.TaskTriggerExecution
	err := s.db.Where("source_task_id = ?", taskID).
		Preload("RunTrigger").
		Preload("RunTrigger.TargetWorkspace").
		Preload("TargetTask").
		Find(&executions).Error
	return executions, err
}

// ToggleTriggerExecution 临时启用/禁用触发执行
func (s *RunTriggerService) ToggleTriggerExecution(executionID uint, disabled bool, userID string) error {
	updates := map[string]interface{}{
		"temporarily_disabled": disabled,
		"updated_at":           time.Now(),
	}

	if disabled {
		now := time.Now()
		updates["disabled_by"] = userID
		updates["disabled_at"] = now
	} else {
		updates["disabled_by"] = nil
		updates["disabled_at"] = nil
	}

	return s.db.Model(&models.TaskTriggerExecution{}).Where("id = ?", executionID).Updates(updates).Error
}

// ExecuteTriggersCreateOnly 执行任务完成后的触发（只创建任务，不执行）
// 在任务成功完成后调用，只创建下游任务，不调用 TryExecuteNextTask
// 这样可以避免在没有完整初始化的 TaskQueueManager 中执行任务
func (s *RunTriggerService) ExecuteTriggersCreateOnly(ctx context.Context, task *models.WorkspaceTask) error {
	// 只有 apply 成功才触发
	if task.Status != models.TaskStatusApplied {
		log.Printf("[RunTrigger] Task %d status is %s, not applied, skipping triggers", task.ID, task.Status)
		return nil
	}

	log.Printf("[RunTrigger] Executing triggers for task %d (workspace %s)", task.ID, task.WorkspaceID)

	// 首先尝试从 task_trigger_executions 表获取预先创建的执行记录
	var executions []models.TaskTriggerExecution
	err := s.db.Where("source_task_id = ? AND status = ?", task.ID, models.TriggerStatusPending).
		Preload("RunTrigger").
		Preload("RunTrigger.TargetWorkspace").
		Find(&executions).Error
	if err != nil {
		log.Printf("[RunTrigger] Error querying task_trigger_executions: %v", err)
		return err
	}

	// 如果没有预先创建的执行记录，直接从 run_triggers 表查询
	if len(executions) == 0 {
		log.Printf("[RunTrigger] No pre-created executions found, querying run_triggers directly")

		var triggers []models.RunTrigger
		err := s.db.Where("source_workspace_id = ? AND enabled = true", task.WorkspaceID).
			Preload("TargetWorkspace").
			Find(&triggers).Error
		if err != nil {
			log.Printf("[RunTrigger] Error querying run_triggers: %v", err)
			return err
		}

		log.Printf("[RunTrigger] Found %d triggers for workspace %s", len(triggers), task.WorkspaceID)

		for _, trigger := range triggers {
			log.Printf("[RunTrigger] Processing trigger %d: %s -> %s",
				trigger.ID, trigger.SourceWorkspaceID, trigger.TargetWorkspaceID)

			// 创建执行记录
			execution := &models.TaskTriggerExecution{
				SourceTaskID: task.ID,
				RunTriggerID: trigger.ID,
				Status:       models.TriggerStatusPending,
			}

			// 创建目标 workspace 的任务
			targetTask, err := s.createTriggeredTask(trigger.TargetWorkspaceID, task)
			if err != nil {
				execution.Status = models.TriggerStatusFailed
				execution.ErrorMessage = err.Error()
				s.db.Create(execution)
				log.Printf("[RunTrigger] Failed to create triggered task for workspace %s: %v",
					trigger.TargetWorkspaceID, err)
				continue
			}

			// 更新执行记录
			execution.Status = models.TriggerStatusTriggered
			execution.TargetTaskID = &targetTask.ID
			s.db.Create(execution)

			log.Printf("[RunTrigger] Successfully triggered task %d for workspace %s from task %d",
				targetTask.ID, trigger.TargetWorkspaceID, task.ID)
			// 注意：不调用 TryExecuteNextTask，任务会被现有的任务队列机制自动执行
		}

		return nil
	}

	// 处理预先创建的执行记录
	log.Printf("[RunTrigger] Found %d pre-created executions for task %d", len(executions), task.ID)

	for _, execution := range executions {
		// 检查是否被临时禁用
		if execution.TemporarilyDisabled {
			execution.Status = models.TriggerStatusSkipped
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Trigger execution %d skipped (temporarily disabled)", execution.ID)
			continue
		}

		// 检查触发器是否仍然启用
		if !execution.RunTrigger.Enabled {
			execution.Status = models.TriggerStatusSkipped
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Trigger execution %d skipped (trigger disabled)", execution.ID)
			continue
		}

		// 创建目标 workspace 的任务
		targetTask, err := s.createTriggeredTask(execution.RunTrigger.TargetWorkspaceID, task)
		if err != nil {
			execution.Status = models.TriggerStatusFailed
			execution.ErrorMessage = err.Error()
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Failed to create triggered task for workspace %s: %v",
				execution.RunTrigger.TargetWorkspaceID, err)
			continue
		}

		// 更新执行记录
		execution.Status = models.TriggerStatusTriggered
		execution.TargetTaskID = &targetTask.ID
		s.db.Save(&execution)

		log.Printf("[RunTrigger] Successfully triggered task %d for workspace %s from task %d",
			targetTask.ID, execution.RunTrigger.TargetWorkspaceID, task.ID)
		// 注意：不调用 TryExecuteNextTask，任务会被现有的任务队列机制自动执行
	}

	return nil
}

// ExecuteTriggers 执行任务完成后的触发
// 在任务成功完成后调用（需要完整初始化的 TaskQueueManager）
func (s *RunTriggerService) ExecuteTriggers(ctx context.Context, task *models.WorkspaceTask, queueManager *TaskQueueManager) error {
	// 只有 apply 成功才触发
	if task.Status != models.TaskStatusApplied {
		log.Printf("[RunTrigger] Task %d status is %s, not applied, skipping triggers", task.ID, task.Status)
		return nil
	}

	log.Printf("[RunTrigger] Executing triggers for task %d (workspace %s)", task.ID, task.WorkspaceID)

	// 首先尝试从 task_trigger_executions 表获取预先创建的执行记录
	var executions []models.TaskTriggerExecution
	err := s.db.Where("source_task_id = ? AND status = ?", task.ID, models.TriggerStatusPending).
		Preload("RunTrigger").
		Preload("RunTrigger.TargetWorkspace").
		Find(&executions).Error
	if err != nil {
		log.Printf("[RunTrigger] Error querying task_trigger_executions: %v", err)
		return err
	}

	// 如果没有预先创建的执行记录，直接从 run_triggers 表查询
	if len(executions) == 0 {
		log.Printf("[RunTrigger] No pre-created executions found, querying run_triggers directly")

		var triggers []models.RunTrigger
		err := s.db.Where("source_workspace_id = ? AND enabled = true", task.WorkspaceID).
			Preload("TargetWorkspace").
			Find(&triggers).Error
		if err != nil {
			log.Printf("[RunTrigger] Error querying run_triggers: %v", err)
			return err
		}

		log.Printf("[RunTrigger] Found %d triggers for workspace %s", len(triggers), task.WorkspaceID)

		for _, trigger := range triggers {
			log.Printf("[RunTrigger] Processing trigger %d: %s -> %s",
				trigger.ID, trigger.SourceWorkspaceID, trigger.TargetWorkspaceID)

			// 创建执行记录
			execution := &models.TaskTriggerExecution{
				SourceTaskID: task.ID,
				RunTriggerID: trigger.ID,
				Status:       models.TriggerStatusPending,
			}

			// 创建目标 workspace 的任务
			targetTask, err := s.createTriggeredTask(trigger.TargetWorkspaceID, task)
			if err != nil {
				execution.Status = models.TriggerStatusFailed
				execution.ErrorMessage = err.Error()
				s.db.Create(execution)
				log.Printf("[RunTrigger] Failed to create triggered task for workspace %s: %v",
					trigger.TargetWorkspaceID, err)
				continue
			}

			// 更新执行记录
			execution.Status = models.TriggerStatusTriggered
			execution.TargetTaskID = &targetTask.ID
			s.db.Create(execution)

			log.Printf("[RunTrigger] Successfully triggered task %d for workspace %s from task %d",
				targetTask.ID, trigger.TargetWorkspaceID, task.ID)

			// 通知队列管理器执行新任务
			go func(wsID string) {
				if err := queueManager.TryExecuteNextTask(wsID); err != nil {
					log.Printf("[RunTrigger] Failed to start triggered task execution for workspace %s: %v", wsID, err)
				}
			}(trigger.TargetWorkspaceID)
		}

		return nil
	}

	// 处理预先创建的执行记录
	log.Printf("[RunTrigger] Found %d pre-created executions for task %d", len(executions), task.ID)

	for _, execution := range executions {
		// 检查是否被临时禁用
		if execution.TemporarilyDisabled {
			execution.Status = models.TriggerStatusSkipped
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Trigger execution %d skipped (temporarily disabled)", execution.ID)
			continue
		}

		// 检查触发器是否仍然启用
		if !execution.RunTrigger.Enabled {
			execution.Status = models.TriggerStatusSkipped
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Trigger execution %d skipped (trigger disabled)", execution.ID)
			continue
		}

		// 创建目标 workspace 的任务
		targetTask, err := s.createTriggeredTask(execution.RunTrigger.TargetWorkspaceID, task)
		if err != nil {
			execution.Status = models.TriggerStatusFailed
			execution.ErrorMessage = err.Error()
			s.db.Save(&execution)
			log.Printf("[RunTrigger] Failed to create triggered task for workspace %s: %v",
				execution.RunTrigger.TargetWorkspaceID, err)
			continue
		}

		// 更新执行记录
		execution.Status = models.TriggerStatusTriggered
		execution.TargetTaskID = &targetTask.ID
		s.db.Save(&execution)

		log.Printf("[RunTrigger] Successfully triggered task %d for workspace %s from task %d",
			targetTask.ID, execution.RunTrigger.TargetWorkspaceID, task.ID)

		// 通知队列管理器执行新任务
		go func(wsID string) {
			if err := queueManager.TryExecuteNextTask(wsID); err != nil {
				log.Printf("[RunTrigger] Failed to start triggered task execution for workspace %s: %v", wsID, err)
			}
		}(execution.RunTrigger.TargetWorkspaceID)
	}

	return nil
}

// createTriggeredTask 创建被触发的任务
func (s *RunTriggerService) createTriggeredTask(targetWorkspaceID string, sourceTask *models.WorkspaceTask) (*models.WorkspaceTask, error) {
	// 获取目标 workspace
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", targetWorkspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("target workspace not found: %w", err)
	}

	// 检查 workspace 是否被锁定
	if workspace.IsLocked {
		return nil, fmt.Errorf("target workspace is locked")
	}

	// 检查 workspace 是否配置了 provider
	if workspace.ProviderConfig == nil || len(workspace.ProviderConfig) == 0 {
		return nil, fmt.Errorf("target workspace has no provider configuration")
	}

	// 创建任务描述
	description := fmt.Sprintf("Triggered by workspace %s (task #%d)", sourceTask.WorkspaceID, sourceTask.ID)

	// 创建 plan_and_apply 任务
	task := &models.WorkspaceTask{
		WorkspaceID:   workspace.WorkspaceID,
		TaskType:      models.TaskTypePlanAndApply,
		Status:        models.TaskStatusPending,
		ExecutionMode: workspace.ExecutionMode,
		Stage:         "pending",
		Description:   description,
	}

	if err := s.db.Create(task).Error; err != nil {
		return nil, fmt.Errorf("failed to create task: %w", err)
	}

	return task, nil
}

// GetTargetWorkspaceAutoApplyWarning 检查目标 workspace 是否开启了 auto_apply
// 返回需要警告的 workspace 列表
func (s *RunTriggerService) GetTargetWorkspaceAutoApplyWarning(sourceWorkspaceID string) ([]models.Workspace, error) {
	var triggers []models.RunTrigger
	err := s.db.Where("source_workspace_id = ? AND enabled = true", sourceWorkspaceID).
		Preload("TargetWorkspace").
		Find(&triggers).Error
	if err != nil {
		return nil, err
	}

	var warnings []models.Workspace
	for _, t := range triggers {
		if t.TargetWorkspace != nil && t.TargetWorkspace.AutoApply {
			warnings = append(warnings, *t.TargetWorkspace)
		}
	}

	return warnings, nil
}

// GetAvailableTargetWorkspaces 获取可以作为触发目标的 workspace 列表
// 排除已经配置的和会形成循环的
func (s *RunTriggerService) GetAvailableTargetWorkspaces(sourceWorkspaceID string) ([]models.Workspace, error) {
	// 获取已配置的目标
	var existingTriggers []models.RunTrigger
	s.db.Where("source_workspace_id = ?", sourceWorkspaceID).Find(&existingTriggers)

	existingTargets := make(map[string]bool)
	for _, t := range existingTriggers {
		existingTargets[t.TargetWorkspaceID] = true
	}

	// 获取所有 workspace
	var workspaces []models.Workspace
	s.db.Find(&workspaces)

	var available []models.Workspace
	for _, ws := range workspaces {
		// 排除自己
		if ws.WorkspaceID == sourceWorkspaceID {
			continue
		}
		// 排除已配置的
		if existingTargets[ws.WorkspaceID] {
			continue
		}
		// 排除会形成循环的
		if s.wouldCreateCycle(sourceWorkspaceID, ws.WorkspaceID) {
			continue
		}
		available = append(available, ws)
	}

	return available, nil
}

// GetAvailableSourceWorkspaces 获取可以作为触发源的 workspace 列表
// 排除已经配置的和会形成循环的
func (s *RunTriggerService) GetAvailableSourceWorkspaces(targetWorkspaceID string) ([]models.Workspace, error) {
	// 获取已配置的源
	var existingTriggers []models.RunTrigger
	s.db.Where("target_workspace_id = ?", targetWorkspaceID).Find(&existingTriggers)

	existingSources := make(map[string]bool)
	for _, t := range existingTriggers {
		existingSources[t.SourceWorkspaceID] = true
	}

	// 获取所有 workspace
	var workspaces []models.Workspace
	s.db.Find(&workspaces)

	var available []models.Workspace
	for _, ws := range workspaces {
		// 排除自己
		if ws.WorkspaceID == targetWorkspaceID {
			continue
		}
		// 排除已配置的
		if existingSources[ws.WorkspaceID] {
			continue
		}
		// 排除会形成循环的
		if s.wouldCreateCycle(ws.WorkspaceID, targetWorkspaceID) {
			continue
		}
		available = append(available, ws)
	}

	return available, nil
}
