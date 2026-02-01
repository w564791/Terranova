package services

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"sync"
	"time"

	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// TaskQueueManager 任务队列管理器
type TaskQueueManager struct {
	db               *gorm.DB
	executor         *TerraformExecutor
	k8sJobService    *K8sJobService
	k8sDeploymentSvc *K8sDeploymentService // K8s Pod管理服务（用于槽位管理）
	agentCCHandler   AgentCCHandler        // Interface for Agent C&C communication
	workspaceLocks   sync.Map              // workspace_id -> *sync.Mutex
}

// AgentCCHandler interface for sending tasks to agents
type AgentCCHandler interface {
	SendTaskToAgent(agentID string, taskID uint, workspaceID string, action string) error
	IsAgentAvailable(agentID string, taskType models.TaskType) bool
	GetConnectedAgents() []string
}

// NewTaskQueueManager 创建任务队列管理器
func NewTaskQueueManager(db *gorm.DB, executor *TerraformExecutor) *TaskQueueManager {
	// Try to initialize K8s Job Service (may fail if not in K8s environment)
	k8sJobService, err := NewK8sJobService(db)
	if err != nil {
		log.Printf("[TaskQueue] K8s Job Service not available: %v", err)
		k8sJobService = nil
	}

	return &TaskQueueManager{
		db:            db,
		executor:      executor,
		k8sJobService: k8sJobService,
	}
}

// SetAgentCCHandler sets the Agent C&C handler (called from main.go after initialization)
func (m *TaskQueueManager) SetAgentCCHandler(handler AgentCCHandler) {
	m.agentCCHandler = handler
	log.Println("[TaskQueue] Agent C&C handler configured")
}

// SetK8sDeploymentService sets the K8s Deployment Service (called from main.go after initialization)
// This is needed for slot-based task allocation in K8s mode
func (m *TaskQueueManager) SetK8sDeploymentService(svc *K8sDeploymentService) {
	m.k8sDeploymentSvc = svc
	log.Println("[TaskQueue] K8s Deployment Service configured for slot management")
}

// CanExecuteNewTask 检查workspace是否可以执行新任务
// plan任务可以并发，plan_and_apply任务必须串行
// 注意: 此方法目前未被使用,但保持逻辑与GetNextExecutableTask一致
func (m *TaskQueueManager) CanExecuteNewTask(workspaceID string) (bool, string) {
	// 1. 检查workspace是否被lock
	var workspace models.Workspace
	if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return false, fmt.Sprintf("无法获取workspace信息: %v", err)
	}

	if workspace.IsLocked {
		lockedBy := "unknown"
		if workspace.LockedBy != nil {
			lockedBy = *workspace.LockedBy
		}
		return false, fmt.Sprintf("workspace被%s锁定", lockedBy)
	}

	// 2. 检查是否有plan_and_apply任务处于非最终状态
	// 非最终状态：pending, running, apply_pending
	// 最终状态：success, applied, failed, cancelled
	var blockingTaskCount int64
	m.db.Model(&models.WorkspaceTask{}).
		Where("workspace_id = ? AND task_type = ? AND status NOT IN (?)",
			workspaceID,
			models.TaskTypePlanAndApply,
			[]string{"success", "applied", "failed", "cancelled"}).
		Count(&blockingTaskCount)

	if blockingTaskCount > 0 {
		return false, "有plan_and_apply任务正在进行中"
	}

	return true, ""
}

// GetNextExecutableTask 获取下一个可执行的任务
// 注意：apply_pending 任务需要用户确认，不会被自动返回
// 只有通过 ConfirmApply 显式触发时才会执行
//
// 任务执行规则:
// 0. workspace被lock时,所有任务都要等待(最高优先级)
// 1. plan任务完全独立,可以并发执行,不受任何plan_and_apply任务阻塞
// 2. plan_and_apply任务之间必须串行执行
//   - running状态的plan_and_apply阻塞其他plan_and_apply任务
//   - pending/apply_pending状态的plan_and_apply阻塞其他plan_and_apply任务
func (m *TaskQueueManager) GetNextExecutableTask(workspaceID string) (*models.WorkspaceTask, error) {
	log.Printf("[TaskQueue] GetNextExecutableTask for workspace %s", workspaceID)

	// 0. 首先检查workspace是否被lock
	var workspace models.Workspace
	if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("failed to get workspace: %w", err)
	}

	if workspace.IsLocked {
		lockedBy := "unknown"
		if workspace.LockedBy != nil {
			lockedBy = *workspace.LockedBy
		}
		log.Printf("[TaskQueue] Workspace %s is locked by %s, all tasks must wait", workspaceID, lockedBy)
		return nil, nil
	}

	// 1. 检查plan_and_apply pending任务（排除apply_pending）
	// 注意: apply_pending任务需要用户通过ConfirmApply API显式确认,不会被自动返回
	// 只有pending状态的plan_and_apply任务才会被自动调度
	var planAndApplyTask models.WorkspaceTask
	err := m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypePlanAndApply,
		models.TaskStatusPending).
		Order("created_at ASC").
		First(&planAndApplyTask).Error

	if err == nil {
		// 找到plan_and_apply pending任务,检查是否有running/pending/apply_pending的plan_and_apply任务阻塞它
		var otherBlockingCount int64
		m.db.Model(&models.WorkspaceTask{}).
			Where("workspace_id = ? AND task_type = ? AND id < ? AND status IN (?)",
				workspaceID,
				models.TaskTypePlanAndApply,
				planAndApplyTask.ID,
				[]models.TaskStatus{models.TaskStatusPending, models.TaskStatusRunning, models.TaskStatusApplyPending}).
			Count(&otherBlockingCount)

		if otherBlockingCount > 0 {
			log.Printf("[TaskQueue] Plan_and_apply task %d is blocked by %d earlier plan_and_apply tasks",
				planAndApplyTask.ID, otherBlockingCount)
			// plan_and_apply被阻塞,但plan任务可以执行
			// 继续检查plan任务
		} else {
			log.Printf("[TaskQueue] Found plan_and_apply pending task %d for workspace %s (no blocking tasks)",
				planAndApplyTask.ID, workspaceID)
			return &planAndApplyTask, nil
		}
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("[TaskQueue] Error checking plan_and_apply tasks: %v", err)
		return nil, err
	}

	// 2. 获取plan任务（完全独立,可以并发执行,不受任何plan_and_apply任务阻塞）
	var planTask models.WorkspaceTask
	err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypePlan, models.TaskStatusPending).
		Order("created_at ASC").
		First(&planTask).Error

	if err == nil {
		log.Printf("[TaskQueue] Found plan pending task %d for workspace %s (can execute concurrently, completely independent)", planTask.ID, workspaceID)
		return &planTask, nil
	} else if err != gorm.ErrRecordNotFound {
		log.Printf("[TaskQueue] Error checking plan tasks: %v", err)
		return nil, err
	}

	// 3. 获取drift_check任务（后台任务,可以并发执行）
	var driftCheckTask models.WorkspaceTask
	err = m.db.Where("workspace_id = ? AND task_type = ? AND status = ?",
		workspaceID, models.TaskTypeDriftCheck, models.TaskStatusPending).
		Order("created_at ASC").
		First(&driftCheckTask).Error

	if err == gorm.ErrRecordNotFound {
		log.Printf("[TaskQueue] No pending tasks found for workspace %s", workspaceID)
		return nil, nil // 没有pending任务
	}

	if err != nil {
		log.Printf("[TaskQueue] Error checking drift_check tasks: %v", err)
		return nil, err
	}

	log.Printf("[TaskQueue] Found drift_check pending task %d for workspace %s (background task)", driftCheckTask.ID, workspaceID)
	return &driftCheckTask, nil
}

// TryExecuteNextTask 尝试执行下一个任务
// 优化: Plan任务不持有锁,只有Plan+Apply任务才持有锁
// 这样Plan任务就不会阻塞Plan+Apply任务的调度
func (m *TaskQueueManager) TryExecuteNextTask(workspaceID string) error {
	log.Printf("[TaskQueue] ===== TryExecuteNextTask START for workspace %s =====", workspaceID)
	defer log.Printf("[TaskQueue] ===== TryExecuteNextTask END for workspace %s =====", workspaceID)

	// 添加panic recovery
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[TaskQueue] ❌ PANIC in TryExecuteNextTask for workspace %s: %v", workspaceID, r)
			// 打印堆栈信息
			log.Printf("[TaskQueue] Stack trace: %s", debug.Stack())
		}
	}()

	log.Printf("[TaskQueue] TryExecuteNextTask called for workspace %s", workspaceID)

	// 1. 先获取下一个任务（不加锁）
	task, err := m.GetNextExecutableTask(workspaceID)
	if err != nil {
		log.Printf("[TaskQueue] Error getting next task for workspace %s: %v", workspaceID, err)
		return err
	}

	if task == nil {
		log.Printf("[TaskQueue] No executable tasks for workspace %s", workspaceID)
		return nil
	}

	// 2. 根据任务类型决定是否加锁
	// Plan任务：不加锁，可以并发执行
	// Plan+Apply任务：加锁，必须串行执行
	if task.TaskType == models.TaskTypePlanAndApply {
		log.Printf("[TaskQueue] Plan+Apply task %d requires workspace lock", task.ID)

		// 获取workspace锁
		lockKey := fmt.Sprintf("ws_%s", workspaceID)
		lock, _ := m.workspaceLocks.LoadOrStore(lockKey, &sync.Mutex{})
		mutex := lock.(*sync.Mutex)

		mutex.Lock()
		defer mutex.Unlock()

		log.Printf("[TaskQueue] Acquired workspace lock for plan+apply task %d", task.ID)

		// 重新检查任务状态（可能在等待锁期间被其他goroutine处理了）
		var currentTask models.WorkspaceTask
		if err := m.db.First(&currentTask, task.ID).Error; err != nil {
			log.Printf("[TaskQueue] Task %d not found after acquiring lock: %v", task.ID, err)
			return nil
		}

		if currentTask.Status != models.TaskStatusPending && currentTask.Status != models.TaskStatusApplyPending {
			log.Printf("[TaskQueue] Task %d status changed to %s after acquiring lock, skipping", task.ID, currentTask.Status)
			return nil
		}

		// 更新task为最新状态
		task = &currentTask
	} else {
		log.Printf("[TaskQueue] Plan task %d does not require workspace lock (can execute concurrently)", task.ID)
	}

	// 3. 获取workspace信息以确定执行模式
	var workspace models.Workspace
	if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		log.Printf("[TaskQueue] Error getting workspace %s: %v", workspaceID, err)
		return err
	}

	// 4. 检查是否为K8s执行模式
	if workspace.ExecutionMode == models.ExecutionModeK8s {
		log.Printf("[TaskQueue] Workspace %s is in K8s mode, pushing task to K8s deployment agent", workspaceID)
		// K8s模式使用Deployment + auto-scaler
		// Agent通过C&C channel接收任务,和Agent模式一样
		return m.pushTaskToAgent(task, &workspace)
	}

	// 5. 检查是否为Agent执行模式
	if workspace.ExecutionMode == models.ExecutionModeAgent {
		log.Printf("[TaskQueue] Workspace %s is in Agent mode, pushing task to agent", workspaceID)
		return m.pushTaskToAgent(task, &workspace)
	}

	// 6. 本地模式 - 直接执行任务
	log.Printf("[TaskQueue] Starting task %d (type: %s, status: %s) for workspace %s in Local mode",
		task.ID, task.TaskType, task.Status, workspaceID)
	go m.executeTask(task)

	return nil
}

// createK8sJobForTask 为任务创建K8s Job（带指数退避重试）
func (m *TaskQueueManager) createK8sJobForTask(task *models.WorkspaceTask, workspace *models.Workspace) error {
	// 1. 检查K8s Job Service是否可用
	if m.k8sJobService == nil {
		log.Printf("[TaskQueue] K8s Job Service not available for task %d, will retry", task.ID)
		m.scheduleRetry(task.WorkspaceID, 5*time.Second)
		return nil
	}

	// 2. 获取Agent Pool信息
	if workspace.CurrentPoolID == nil {
		log.Printf("[TaskQueue] Workspace %s has no pool assigned, task %d will retry", workspace.WorkspaceID, task.ID)
		m.scheduleRetry(task.WorkspaceID, 10*time.Second)
		return nil
	}

	var pool models.AgentPool
	if err := m.db.Where("pool_id = ?", *workspace.CurrentPoolID).First(&pool).Error; err != nil {
		log.Printf("[TaskQueue] Error getting pool %s for task %d, will retry: %v", *workspace.CurrentPoolID, task.ID, err)
		m.scheduleRetry(task.WorkspaceID, 10*time.Second)
		return nil
	}

	// 3. 创建K8s Job
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := m.k8sJobService.CreateJobForTask(ctx, task, &pool); err != nil {
		log.Printf("[TaskQueue] Failed to create K8s Job for task %d: %v", task.ID, err)

		// 增加重试计数
		task.RetryCount++
		m.db.Save(task)

		// 检查是否是freeze window错误
		errMsg := err.Error()
		if len(errMsg) >= 13 && (errMsg[:13] == "pool is in fr" || errMsg[:13] == "Pool is in fr") {
			log.Printf("[TaskQueue] Task %d blocked by freeze window, will retry in 60s (retry count: %d)", task.ID, task.RetryCount)
			m.scheduleRetry(task.WorkspaceID, 60*time.Second)
			return nil
		}

		// 其他K8s相关错误使用指数退避重试
		retryDelay := m.calculateRetryDelay(task)
		log.Printf("[TaskQueue] K8s Job creation failed for task %d, will retry in %v (retry count: %d)", task.ID, retryDelay, task.RetryCount)
		m.scheduleRetry(task.WorkspaceID, retryDelay)
		return nil
	}

	// 成功创建Job，重置重试计数
	if task.RetryCount > 0 {
		task.RetryCount = 0
		m.db.Save(task)
	}

	log.Printf("[TaskQueue] Successfully created K8s Job for task %d", task.ID)
	return nil
}

// calculateRetryDelay 计算指数退避延迟时间
func (m *TaskQueueManager) calculateRetryDelay(task *models.WorkspaceTask) time.Duration {
	// 获取任务的重试次数（使用retry_count字段）
	retryCount := task.RetryCount

	// 指数退避：5s, 10s, 20s, 40s, 60s (最大)
	delays := []time.Duration{
		5 * time.Second,
		10 * time.Second,
		20 * time.Second,
		40 * time.Second,
		60 * time.Second,
	}

	if retryCount >= len(delays) {
		return delays[len(delays)-1] // 最大60秒
	}

	return delays[retryCount]
}

// scheduleRetry 调度重试
func (m *TaskQueueManager) scheduleRetry(workspaceID string, delay time.Duration) {
	go func() {
		time.Sleep(delay)
		log.Printf("[TaskQueue] Retrying task execution for workspace %s after %v", workspaceID, delay)
		m.TryExecuteNextTask(workspaceID)
	}()
}

// pushTaskToAgent pushes a task to an available agent via C&C channel
// For K8s mode with slot management enabled, this will allocate a slot before sending the task
// If no agents are available, it will trigger immediate scale-up for K8s pools
func (m *TaskQueueManager) pushTaskToAgent(task *models.WorkspaceTask, workspace *models.Workspace) error {
	// 0. CRITICAL SECURITY CHECK: Reject apply_pending tasks that weren't explicitly confirmed
	// apply_pending tasks MUST only be executed through ConfirmApply API
	// This is a defensive measure to prevent unauthorized apply executions
	if task.Status == models.TaskStatusApplyPending && task.ApplyConfirmedBy == nil {
		log.Printf("[TaskQueue] ❌ SECURITY: Rejecting unconfirmed apply_pending task %d - requires explicit user confirmation via ConfirmApply API", task.ID)
		return fmt.Errorf("apply_pending tasks require explicit user confirmation via ConfirmApply")
	}

	// Log confirmed apply_pending tasks for audit trail
	if task.Status == models.TaskStatusApplyPending && task.ApplyConfirmedBy != nil {
		log.Printf("[TaskQueue] ✓ Executing confirmed apply_pending task %d (confirmed by: %s at %v)",
			task.ID, *task.ApplyConfirmedBy, task.ApplyConfirmedAt)
	}

	// 1. Check if Agent C&C handler is available
	if m.agentCCHandler == nil {
		log.Printf("[TaskQueue] ❌ CRITICAL: Agent C&C handler is nil for task %d", task.ID)
		log.Printf("[TaskQueue] This means SetAgentCCHandler() was not called or failed")
		log.Printf("[TaskQueue] Please check server initialization logs for 'Agent C&C handler configured'")
		m.scheduleRetry(task.WorkspaceID, 5*time.Second)
		return nil
	}

	log.Printf("[TaskQueue] ✓ Agent C&C handler is available")

	// 2. Get workspace's current pool
	if workspace.CurrentPoolID == nil {
		log.Printf("[TaskQueue] Workspace %s has no pool assigned, task %d will retry", workspace.WorkspaceID, task.ID)
		m.scheduleRetry(task.WorkspaceID, 10*time.Second)
		return nil
	}

	// 2.5. For K8s mode with slot management, try to allocate a slot first
	var selectedPodName string
	var selectedSlotID int
	var selectedAgentID string

	if workspace.ExecutionMode == models.ExecutionModeK8s && m.k8sDeploymentSvc != nil && m.k8sDeploymentSvc.podManager != nil {
		log.Printf("[TaskQueue] K8s mode with slot management enabled for task %d", task.ID)

		// Special handling for apply_pending tasks: reuse the reserved slot
		if task.Status == models.TaskStatusApplyPending {
			log.Printf("[TaskQueue] Task %d is apply_pending, looking for reserved slot", task.ID)

			// Find the reserved slot for this task
			pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(task.ID)
			if err == nil {
				// Found the reserved slot, reuse it for apply
				selectedPodName = pod.PodName
				selectedSlotID = slotID
				selectedAgentID = pod.AgentID

				log.Printf("[TaskQueue] Found reserved slot %d on pod %s for apply_pending task %d (will reuse for apply)",
					slotID, pod.PodName, task.ID)
			} else {
				// No reserved slot found - Pod was deleted
				// But plan data is still in database, so we can allocate a new slot and continue with apply
				log.Printf("[TaskQueue] Apply_pending task %d has no reserved slot (Pod was deleted): %v", task.ID, err)
				log.Printf("[TaskQueue] Will allocate a new slot for task %d to continue with apply", task.ID)

				// Don't return error - continue to allocate a new slot below
				// Clear the selected variables so we allocate a new slot
				selectedPodName = ""
				selectedSlotID = -1
				selectedAgentID = ""
			}
		}

		// If no slot selected yet (either normal pending task or apply_pending with deleted Pod)
		if selectedPodName == "" {
			// Find a free slot
			pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodWithFreeSlot(
				*workspace.CurrentPoolID,
				string(task.TaskType),
			)

			if err != nil {
				log.Printf("[TaskQueue] No free slot available for task %d: %v, will retry", task.ID, err)
				m.scheduleRetry(task.WorkspaceID, 10*time.Second)
				return nil
			}

			// Allocate the slot
			if err := m.k8sDeploymentSvc.podManager.AssignTaskToSlot(pod.PodName, slotID, task.ID, string(task.TaskType)); err != nil {
				log.Printf("[TaskQueue] Failed to assign task %d to slot %d on pod %s: %v", task.ID, slotID, pod.PodName, err)
				m.scheduleRetry(task.WorkspaceID, 5*time.Second)
				return nil
			}

			selectedPodName = pod.PodName
			selectedSlotID = slotID
			selectedAgentID = pod.AgentID

			log.Printf("[TaskQueue] Allocated slot %d on pod %s for task %d", slotID, pod.PodName, task.ID)
		}
	}

	// 3. Get actually connected agents from AgentCCHandler
	connectedAgentIDs := m.agentCCHandler.GetConnectedAgents()
	if len(connectedAgentIDs) == 0 {
		// If we allocated a slot, release it before retrying
		if selectedPodName != "" {
			m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
			log.Printf("[TaskQueue] Released slot %d on pod %s (no connected agents)", selectedSlotID, selectedPodName)
		}

		log.Printf("[TaskQueue] No connected agents found, task %d will retry", task.ID)

		// For K8s mode: trigger immediate scale-up check if no agents are connected
		// This ensures pods are created when there are pending tasks but no agents
		if workspace.ExecutionMode == models.ExecutionModeK8s && m.k8sDeploymentSvc != nil {
			go m.triggerK8sScaleUpForPool(*workspace.CurrentPoolID)
		}

		m.scheduleRetry(task.WorkspaceID, 15*time.Second)
		return nil
	}

	log.Printf("[TaskQueue] Found %d connected agents: %v", len(connectedAgentIDs), connectedAgentIDs)

	// 4. Filter agents by pool and find available one
	var selectedAgent *models.Agent

	// If we already selected an agent via slot allocation, use that agent
	if selectedAgentID != "" {
		for _, agentID := range connectedAgentIDs {
			if agentID == selectedAgentID {
				var agent models.Agent
				if err := m.db.Where("agent_id = ?", agentID).First(&agent).Error; err == nil {
					selectedAgent = &agent
					log.Printf("[TaskQueue] Using pre-selected agent %s from slot allocation", agentID)
					break
				}
			}
		}

		// If the pre-selected agent is not connected, release the slot and find another
		if selectedAgent == nil {
			log.Printf("[TaskQueue] Pre-selected agent %s is not connected, releasing slot and finding another", selectedAgentID)
			m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
			selectedPodName = ""
			selectedSlotID = -1
			selectedAgentID = ""
		}
	}

	// If no agent selected yet, find one from connected agents
	if selectedAgent == nil {
		for _, agentID := range connectedAgentIDs {
			// Get agent from database to check pool
			var agent models.Agent
			if err := m.db.Where("agent_id = ?", agentID).First(&agent).Error; err != nil {
				log.Printf("[TaskQueue] Warning: Connected agent %s not found in database", agentID)
				continue
			}

			// Check if agent belongs to the target pool
			if agent.PoolID == nil {
				log.Printf("[TaskQueue] Agent %s has no pool assigned, skipping", agentID)
				continue
			}

			if *agent.PoolID != *workspace.CurrentPoolID {
				log.Printf("[TaskQueue] Agent %s belongs to pool %s, not target pool %s, skipping",
					agentID, *agent.PoolID, *workspace.CurrentPoolID)
				continue
			}

			log.Printf("[TaskQueue] Agent %s belongs to target pool %s, checking availability", agentID, *workspace.CurrentPoolID)

			// Check if agent can accept this task type
			if m.agentCCHandler.IsAgentAvailable(agentID, task.TaskType) {
				selectedAgent = &agent
				log.Printf("[TaskQueue] Selected agent %s from pool %s", agentID, *workspace.CurrentPoolID)
				break
			} else {
				log.Printf("[TaskQueue] Agent %s is not available for task type %s", agentID, task.TaskType)
			}
		}
	}

	if selectedAgent == nil {
		// Release slot if allocated
		if selectedPodName != "" {
			m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
			log.Printf("[TaskQueue] Released slot %d on pod %s (no available agents)", selectedSlotID, selectedPodName)
		}

		log.Printf("[TaskQueue] No available agents in pool %s for task %d (type: %s), will retry",
			*workspace.CurrentPoolID, task.ID, task.TaskType)
		log.Printf("[TaskQueue] Connected agents: %v, target pool: %s", connectedAgentIDs, *workspace.CurrentPoolID)
		m.scheduleRetry(task.WorkspaceID, 10*time.Second)
		return nil
	}

	// 5. Determine action based on task status
	action := "plan"
	if task.Status == models.TaskStatusApplyPending {
		action = "apply"
	}

	// 6. Update task status to running and assign to agent BEFORE sending to agent
	// This ensures agent_id is available when agent calls GetTaskData
	task.Status = models.TaskStatusRunning
	task.StartedAt = timePtr(time.Now())
	task.AgentID = &selectedAgent.AgentID // 设置 agent_id (MUST be before SendTaskToAgent)
	if action == "apply" {
		task.Stage = "applying"
		// Set PlanTaskID to point to itself for plan_and_apply tasks
		task.PlanTaskID = &task.ID
	} else {
		task.Stage = "planning"
	}

	// Save to database BEFORE sending task to agent
	// This is critical: agent will call GetTaskData immediately after receiving the task
	if err := m.db.Save(task).Error; err != nil {
		// Release slot if allocated
		if selectedPodName != "" {
			m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
			log.Printf("[TaskQueue] Released slot %d on pod %s (failed to save task)", selectedSlotID, selectedPodName)
		}

		log.Printf("[TaskQueue] Failed to save task %d before sending to agent: %v", task.ID, err)

		// Increment retry count
		task.RetryCount++
		m.db.Save(task)

		// Retry with exponential backoff
		retryDelay := m.calculateRetryDelay(task)
		log.Printf("[TaskQueue] Will retry task %d in %v (retry count: %d)", task.ID, retryDelay, task.RetryCount)
		m.scheduleRetry(task.WorkspaceID, retryDelay)
		return nil
	}

	log.Printf("[TaskQueue] Task %d status updated to running and agent_id set to %s (saved to DB)", task.ID, selectedAgent.AgentID)

	// 发送任务开始执行通知
	go m.sendTaskStartNotification(task, action)

	// 7. Send task to agent via C&C channel (AFTER saving agent_id to DB)
	if err := m.agentCCHandler.SendTaskToAgent(selectedAgent.AgentID, task.ID, task.WorkspaceID, action); err != nil {
		// Release slot if allocated
		if selectedPodName != "" {
			m.k8sDeploymentSvc.podManager.ReleaseSlot(selectedPodName, selectedSlotID)
			log.Printf("[TaskQueue] Released slot %d on pod %s (failed to send task)", selectedSlotID, selectedPodName)
		}

		log.Printf("[TaskQueue] Failed to send task %d to agent %s: %v", task.ID, selectedAgent.AgentID, err)

		// Rollback task status since we failed to send
		task.Status = models.TaskStatusPending
		task.StartedAt = nil
		task.AgentID = nil
		task.Stage = ""
		if action == "apply" {
			task.Status = models.TaskStatusApplyPending
		}
		m.db.Save(task)

		// Increment retry count
		task.RetryCount++
		m.db.Save(task)

		// Retry with exponential backoff
		retryDelay := m.calculateRetryDelay(task)
		log.Printf("[TaskQueue] Will retry task %d in %v (retry count: %d)", task.ID, retryDelay, task.RetryCount)
		m.scheduleRetry(task.WorkspaceID, retryDelay)
		return nil
	}

	// Reset retry count on success
	if task.RetryCount > 0 {
		task.RetryCount = 0
		m.db.Save(task)
	}

	log.Printf("[TaskQueue] Successfully pushed task %d to agent %s (action: %s)", task.ID, selectedAgent.AgentID, action)

	// 8. For K8s mode with slot management, log slot allocation
	if selectedPodName != "" {
		log.Printf("[TaskQueue] Task %d allocated to pod %s slot %d", task.ID, selectedPodName, selectedSlotID)
	}

	return nil
}

// executeTask 执行任务
func (m *TaskQueueManager) executeTask(task *models.WorkspaceTask) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	var err error

	// 检查任务状态，决定执行plan还是apply
	if task.Status == models.TaskStatusApplyPending {
		// 执行Apply阶段
		log.Printf("[TaskQueue] Executing apply for task %d (workspace %s)", task.ID, task.WorkspaceID)

		// 更新状态为running
		task.Status = models.TaskStatusRunning
		task.Stage = "applying"
		m.db.Save(task)

		// 设置PlanTaskID指向自己（plan_and_apply任务的plan数据在自己身上）
		task.PlanTaskID = &task.ID
		m.db.Save(task)

		// 执行Apply
		err = m.executor.ExecuteApply(ctx, task)

		if err != nil {
			// ExecuteApply已经通过saveTaskFailure保存了详细错误信息
			// 这里只需要记录日志，不要覆盖ErrorMessage
			log.Printf("[TaskQueue] Task %d apply failed: %v", task.ID, err)
		} else {
			log.Printf("[TaskQueue] Task %d apply completed successfully", task.ID)
		}
	} else {
		// 执行Plan阶段
		log.Printf("[TaskQueue] Executing plan for task %d (workspace %s)", task.ID, task.WorkspaceID)

		// 更新任务状态为running
		task.Status = models.TaskStatusRunning
		task.StartedAt = timePtr(time.Now())
		m.db.Save(task)

		// 执行Plan
		err = m.executor.ExecutePlan(ctx, task)

		if err != nil {
			// ExecutePlan已经通过saveTaskFailure保存了详细错误信息
			// 这里只需要记录日志，不要覆盖ErrorMessage
			log.Printf("[TaskQueue] Task %d plan failed: %v", task.ID, err)
		} else {
			log.Printf("[TaskQueue] Task %d plan completed with status: %s", task.ID, task.Status)
		}
	}

	// 任务完成后，尝试执行下一个任务
	// 注意：只有到达最终状态才会触发
	// planned_and_finished 也是最终状态（Plan完成但无需Apply）
	if task.Status == models.TaskStatusSuccess ||
		task.Status == models.TaskStatusApplied ||
		task.Status == models.TaskStatusPlannedAndFinished ||
		task.Status == models.TaskStatusFailed ||
		task.Status == models.TaskStatusCancelled {
		log.Printf("[TaskQueue] Task %d reached final status %s, triggering next task", task.ID, task.Status)

		// 如果任务成功完成（applied），执行 Run Triggers
		if task.Status == models.TaskStatusApplied {
			go m.executeRunTriggers(task)
		}

		// 如果是 drift_check 任务，处理 drift 检测结果
		if task.TaskType == models.TaskTypeDriftCheck {
			go m.processDriftCheckResult(task)
		}

		// Apply 完成后（无论成功还是失败）同步 CMDB
		// 这里统一处理，确保 Local、Agent、K8s Agent 三种模式都能正确同步
		if task.TaskType == models.TaskTypePlanAndApply &&
			(task.Status == models.TaskStatusApplied || task.Status == models.TaskStatusFailed) {
			go m.syncCMDBAfterApply(task)
		}

		go m.TryExecuteNextTask(task.WorkspaceID)
	} else {
		log.Printf("[TaskQueue] Task %d in non-final status %s, not triggering next task", task.ID, task.Status)
	}
}

// RecoverPendingTasks 系统启动时恢复pending任务
// 注意：只恢复真正pending的任务，不包括apply_pending状态的任务
// apply_pending任务需要等待用户确认，不应该在服务器重启时自动执行
// 【重要】Run Triggers 创建的任务不会被恢复，因为执行历史 trigger 可能带来意外风险
func (m *TaskQueueManager) RecoverPendingTasks() error {
	// 1. 清理孤儿任务（running状态但后端已重启）
	log.Println("[TaskQueue] Cleaning up orphan tasks...")
	if err := m.CleanupOrphanTasks(); err != nil {
		log.Printf("[TaskQueue] Warning: Failed to cleanup orphan tasks: %v", err)
	}

	// 2. 取消所有由 Run Triggers 创建的 pending 任务
	// 这些任务的 description 包含 "Triggered by workspace"
	// 执行历史 trigger 可能带来意外风险，所以不恢复这些任务
	var triggerTasks []models.WorkspaceTask
	m.db.Where("status = ? AND description LIKE ?", models.TaskStatusPending, "Triggered by workspace%").
		Find(&triggerTasks)

	if len(triggerTasks) > 0 {
		log.Printf("[TaskQueue] Found %d Run Trigger tasks pending, cancelling them (not safe to recover)", len(triggerTasks))
		for _, task := range triggerTasks {
			log.Printf("[TaskQueue] Cancelling Run Trigger task %d (workspace %s): %s", task.ID, task.WorkspaceID, task.Description)
			m.db.Model(&task).Updates(map[string]interface{}{
				"status":        models.TaskStatusCancelled,
				"error_message": "Cancelled on server restart - Run Trigger tasks are not recovered for safety",
			})
		}
	}

	// 3. 获取所有有pending任务的workspace（排除apply_pending状态和Run Trigger任务）
	// apply_pending任务在等待用户确认，不应该自动恢复执行
	var workspaceIDs []string
	m.db.Model(&models.WorkspaceTask{}).
		Where("status = ? AND (description IS NULL OR description NOT LIKE ?)", models.TaskStatusPending, "Triggered by workspace%").
		Distinct("workspace_id").
		Pluck("workspace_id", &workspaceIDs)

	log.Printf("[TaskQueue] Recovering pending tasks for %d workspaces (excluding apply_pending and Run Trigger tasks)", len(workspaceIDs))

	// 4. 为每个workspace尝试执行下一个任务
	for _, wsID := range workspaceIDs {
		log.Printf("[TaskQueue] Attempting to recover tasks for workspace %s", wsID)
		go m.TryExecuteNextTask(wsID)
	}

	// 4. 记录apply_pending任务的数量（这些任务不会自动恢复）
	var applyPendingCount int64
	m.db.Model(&models.WorkspaceTask{}).
		Where("status = ?", models.TaskStatusApplyPending).
		Count(&applyPendingCount)

	if applyPendingCount > 0 {
		log.Printf("[TaskQueue] Found %d apply_pending tasks waiting for user confirmation (will not auto-execute)", applyPendingCount)
	}

	return nil
}

// StartPendingTasksMonitor 启动pending任务监控器,定期检查并重试pending任务
// 这确保所有pending任务都能得到执行机会,即使之前的尝试失败了
func (m *TaskQueueManager) StartPendingTasksMonitor(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	log.Printf("[TaskQueue] Starting pending tasks monitor with interval: %v", interval)

	for {
		select {
		case <-ctx.Done():
			log.Printf("[TaskQueue] Pending tasks monitor stopped")
			return
		case <-ticker.C:
			m.checkAndRetryPendingTasks()
		}
	}
}

// checkAndRetryPendingTasks 检查并重试所有pending任务
// 【修复】不再排除 Run Triggers 创建的任务，因为 Agent 模式下 ExecuteTriggersCreateOnly 不会调用 TryExecuteNextTask
// Run Trigger 创建的任务需要通过这个定期检查机制来执行
func (m *TaskQueueManager) checkAndRetryPendingTasks() {
	// 获取所有有pending任务的workspace（包括 Run Trigger 任务）
	var workspaceIDs []string
	m.db.Model(&models.WorkspaceTask{}).
		Where("status = ?", models.TaskStatusPending).
		Distinct("workspace_id").
		Pluck("workspace_id", &workspaceIDs)

	if len(workspaceIDs) == 0 {
		return // 没有pending任务,跳过
	}

	log.Printf("[TaskQueue] Checking %d workspaces with pending tasks", len(workspaceIDs))

	// 为每个workspace尝试执行下一个任务
	for _, wsID := range workspaceIDs {
		go m.TryExecuteNextTask(wsID)
	}
}

// CleanupOrphanTasks 清理孤儿任务（后端重启时running状态的任务）
// 注意：apply_pending状态的任务不应该被标记为失败，因为它们只是在等待用户确认，并未实际执行
func (m *TaskQueueManager) CleanupOrphanTasks() error {
	// 查找所有running状态的任务
	// 注意：不包括apply_pending状态，因为这些任务只是在等待用户确认，服务器重启不影响它们
	var orphanTasks []models.WorkspaceTask
	err := m.db.Where("status = ?", models.TaskStatusRunning).Find(&orphanTasks).Error
	if err != nil {
		return fmt.Errorf("failed to query orphan tasks: %w", err)
	}

	if len(orphanTasks) == 0 {
		log.Println("[TaskQueue] No orphan tasks found")
		return nil
	}

	log.Printf("[TaskQueue] Found %d orphan tasks to check", len(orphanTasks))

	// 标记为failed（但排除apply_pending状态的任务）
	failedCount := 0
	skippedCount := 0
	for _, task := range orphanTasks {
		// 如果任务的stage是apply_pending，说明它只是在等待用户确认，不应该标记为失败
		// 这种情况下，任务应该保持在pending状态，等待用户确认后再执行
		if task.Stage == "apply_pending" {
			log.Printf("[TaskQueue] Skipping task %d (workspace %s, type %s, stage %s) - waiting for user confirmation",
				task.ID, task.WorkspaceID, task.TaskType, task.Stage)

			// 将状态重置为apply_pending（如果之前被错误地设置为running）
			task.Status = models.TaskStatusApplyPending
			if err := m.db.Save(&task).Error; err != nil {
				log.Printf("[TaskQueue] Failed to reset task %d status: %v", task.ID, err)
			}
			skippedCount++
			continue
		}

		// 其他running状态的任务确实是被中断的，标记为failed
		task.Status = models.TaskStatusFailed
		task.ErrorMessage = "Task interrupted by server restart"
		task.CompletedAt = timePtr(time.Now())

		if err := m.db.Save(&task).Error; err != nil {
			log.Printf("[TaskQueue] Failed to update orphan task %d: %v", task.ID, err)
			continue
		}

		log.Printf("[TaskQueue] Marked orphan task %d (workspace %s, type %s, stage %s) as failed",
			task.ID, task.WorkspaceID, task.TaskType, task.Stage)
		failedCount++
	}

	log.Printf("[TaskQueue] Cleanup complete: %d tasks marked as failed, %d tasks skipped (waiting for confirmation)",
		failedCount, skippedCount)

	return nil
}

// ============================================================================
// Phase 2: Pod槽位管理
// ============================================================================

// ReleaseTaskSlot releases the slot allocated to a task (called when task completes)
// This should be called by the agent when a task finishes execution
func (m *TaskQueueManager) ReleaseTaskSlot(taskID uint) error {
	// Check if K8s Deployment Service is available
	if m.k8sDeploymentSvc == nil || m.k8sDeploymentSvc.podManager == nil {
		// Slot management not enabled, nothing to do
		return nil
	}

	// Find the Pod and slot for this task
	pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(taskID)
	if err != nil {
		// Task not found in any slot - this is OK for non-K8s tasks or if already released
		log.Printf("[TaskQueue] Task %d not found in any slot (may be non-K8s task or already released)", taskID)
		return nil
	}

	// Release the slot
	if err := m.k8sDeploymentSvc.podManager.ReleaseSlot(pod.PodName, slotID); err != nil {
		return fmt.Errorf("failed to release slot: %w", err)
	}

	log.Printf("[TaskQueue] Released slot %d on pod %s for completed task %d", slotID, pod.PodName, taskID)
	return nil
}

// ReserveSlotForApplyPending reserves Slot 0 for an apply_pending task
// This should be called when a plan_and_apply task completes its plan phase
// The reserved slot ensures the Pod won't be deleted during scale-down
func (m *TaskQueueManager) ReserveSlotForApplyPending(taskID uint) error {
	// Check if K8s Deployment Service is available
	if m.k8sDeploymentSvc == nil || m.k8sDeploymentSvc.podManager == nil {
		// Slot management not enabled, nothing to do
		return nil
	}

	// Find the Pod and slot for this task
	pod, slotID, err := m.k8sDeploymentSvc.podManager.FindPodByTaskID(taskID)
	if err != nil {
		log.Printf("[TaskQueue] Warning: Task %d not found in any slot, cannot reserve", taskID)
		return nil
	}

	// Reserve the slot (must be Slot 0 for plan_and_apply tasks)
	if slotID != 0 {
		log.Printf("[TaskQueue] Warning: Task %d is on slot %d, but only Slot 0 can be reserved", taskID, slotID)
		return nil
	}

	if err := m.k8sDeploymentSvc.podManager.ReserveSlot(pod.PodName, slotID, taskID); err != nil {
		return fmt.Errorf("failed to reserve slot: %w", err)
	}

	log.Printf("[TaskQueue] Reserved slot %d on pod %s for apply_pending task %d", slotID, pod.PodName, taskID)
	return nil
}

// triggerK8sScaleUpForPool triggers an immediate scale-up check for a K8s pool
// This is called when there are pending tasks but no connected agents
// It ensures that pods are created to handle the pending tasks
func (m *TaskQueueManager) triggerK8sScaleUpForPool(poolID string) {
	if m.k8sDeploymentSvc == nil {
		return
	}

	log.Printf("[TaskQueue] Triggering immediate scale-up check for pool %s (no connected agents)", poolID)

	// Get the pool
	var pool models.AgentPool
	if err := m.db.Where("pool_id = ?", poolID).First(&pool).Error; err != nil {
		log.Printf("[TaskQueue] Failed to get pool %s for scale-up: %v", poolID, err)
		return
	}

	// Check if pool is K8s type
	if pool.PoolType != models.AgentPoolTypeK8s {
		log.Printf("[TaskQueue] Pool %s is not K8s type, skipping scale-up", poolID)
		return
	}

	// Trigger auto-scale with a short timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, ensure pods exist for the pool (this will create min_replicas pods if none exist)
	if err := m.k8sDeploymentSvc.EnsurePodsForPool(ctx, &pool); err != nil {
		log.Printf("[TaskQueue] Failed to ensure pods for pool %s: %v", poolID, err)
	}

	// Then trigger auto-scale to potentially create more pods based on pending tasks
	newCount, scaled, err := m.k8sDeploymentSvc.AutoScalePods(ctx, &pool)
	if err != nil {
		log.Printf("[TaskQueue] Failed to auto-scale pool %s: %v", poolID, err)
		return
	}

	if scaled {
		log.Printf("[TaskQueue] Successfully triggered scale-up for pool %s, new pod count: %d", poolID, newCount)
	} else {
		log.Printf("[TaskQueue] Scale-up check completed for pool %s, no scaling needed (current pods: %d)", poolID, newCount)
	}
}

// sendTaskStartNotification 发送任务开始执行通知
func (m *TaskQueueManager) sendTaskStartNotification(task *models.WorkspaceTask, action string) {
	// 确定通知事件类型
	var event models.NotificationEvent
	if action == "apply" {
		event = models.NotificationEventTaskApplying
	} else {
		event = models.NotificationEventTaskPlanning
	}

	// 创建 NotificationSender
	platformConfigService := NewPlatformConfigService(m.db)
	baseURL := platformConfigService.GetBaseURL()
	notificationSender := NewNotificationSender(m.db, baseURL)

	// 发送通知
	ctx := context.Background()
	if err := notificationSender.TriggerNotifications(
		ctx,
		task.WorkspaceID,
		event,
		task,
	); err != nil {
		log.Printf("[Notification] Failed to send %s notification for task %d: %v", event, task.ID, err)
	} else {
		log.Printf("[Notification] Successfully sent %s notification for task %d", event, task.ID)
	}
}

// executeRunTriggers 执行 Run Triggers（任务成功完成后触发下游 workspace）
// 这个方法在 Server 端执行，确保 Local、Agent、K8s Agent 三种模式都能正确触发
func (m *TaskQueueManager) executeRunTriggers(task *models.WorkspaceTask) {
	log.Printf("[RunTrigger] Checking run triggers for task %d (workspace %s)", task.ID, task.WorkspaceID)

	// 创建 RunTriggerService
	runTriggerService := NewRunTriggerService(m.db)

	// 执行触发
	ctx := context.Background()
	if err := runTriggerService.ExecuteTriggers(ctx, task, m); err != nil {
		log.Printf("[RunTrigger] Failed to execute run triggers for task %d: %v", task.ID, err)
	} else {
		log.Printf("[RunTrigger] Successfully executed run triggers for task %d", task.ID)
	}
}

// processDriftCheckResult 处理 drift check 任务完成后的结果
func (m *TaskQueueManager) processDriftCheckResult(task *models.WorkspaceTask) {
	log.Printf("[DriftCheck] Processing drift check result for task %d (workspace %s)", task.ID, task.WorkspaceID)

	// 创建 DriftCheckService
	driftService := NewDriftCheckService(m.db)

	// 处理结果
	if err := driftService.ProcessDriftCheckResult(task); err != nil {
		log.Printf("[DriftCheck] Failed to process drift check result for task %d: %v", task.ID, err)
	} else {
		log.Printf("[DriftCheck] Successfully processed drift check result for task %d", task.ID)
	}
}

// syncCMDBAfterApply Apply 完成后同步 CMDB
// 这个方法在 Server 端执行，确保 Local、Agent、K8s Agent 三种模式都能正确同步
// 无论 Apply 成功还是失败都会触发同步，因为部分资源可能已经创建或修改
func (m *TaskQueueManager) syncCMDBAfterApply(task *models.WorkspaceTask) {
	log.Printf("[CMDB] Apply completed for task %d (status: %s), syncing CMDB for workspace %s",
		task.ID, task.Status, task.WorkspaceID)

	// 创建 CMDBService
	cmdbService := NewCMDBService(m.db)

	// 同步 CMDB
	if err := cmdbService.SyncWorkspaceResources(task.WorkspaceID); err != nil {
		log.Printf("[CMDB] Failed to sync workspace %s after apply: %v", task.WorkspaceID, err)
	} else {
		log.Printf("[CMDB] Successfully synced workspace %s after apply (task %d, status: %s)",
			task.WorkspaceID, task.ID, task.Status)
	}
}

// ExecuteConfirmedApply executes a confirmed apply_pending task
// This method should ONLY be called from ConfirmApply endpoint after user confirmation
// It bypasses GetNextExecutableTask and directly executes the confirmed task
func (m *TaskQueueManager) ExecuteConfirmedApply(workspaceID string, taskID uint) error {
	log.Printf("[TaskQueue] ExecuteConfirmedApply called for task %d in workspace %s", taskID, workspaceID)

	// Get the task
	var task models.WorkspaceTask
	if err := m.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// Verify task is apply_pending and has been confirmed
	if task.Status != models.TaskStatusApplyPending {
		return fmt.Errorf("task %d is not in apply_pending status (current: %s)", taskID, task.Status)
	}

	if task.ApplyConfirmedBy == nil {
		return fmt.Errorf("task %d has not been confirmed by user", taskID)
	}

	log.Printf("[TaskQueue] Task %d confirmed by %s at %v, proceeding with apply execution",
		taskID, *task.ApplyConfirmedBy, task.ApplyConfirmedAt)

	// Get workspace
	var workspace models.Workspace
	if err := m.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	// Execute based on execution mode
	if workspace.ExecutionMode == models.ExecutionModeK8s || workspace.ExecutionMode == models.ExecutionModeAgent {
		return m.pushTaskToAgent(&task, &workspace)
	}

	// Local mode
	go m.executeTask(&task)
	return nil
}
