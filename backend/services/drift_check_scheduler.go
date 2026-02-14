package services

import (
	"context"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"sync"
	"time"

	"gorm.io/gorm"
)

// DriftCheckScheduler Drift 检测调度器
// 定时检查需要执行 drift 检测的 workspace，创建后台任务
type DriftCheckScheduler struct {
	db               *gorm.DB
	driftService     *DriftCheckService
	taskQueueManager *TaskQueueManager
	ticker           *time.Ticker
	stopChan         chan struct{}
	isRunning        bool
	mutex            sync.Mutex
}

// NewDriftCheckScheduler 创建 Drift 检测调度器
func NewDriftCheckScheduler(db *gorm.DB, taskQueueManager *TaskQueueManager) *DriftCheckScheduler {
	return &DriftCheckScheduler{
		db:               db,
		driftService:     NewDriftCheckService(db),
		taskQueueManager: taskQueueManager,
		stopChan:         make(chan struct{}),
	}
}

// Start 启动调度器
func (s *DriftCheckScheduler) Start(ctx context.Context, interval time.Duration) {
	s.mutex.Lock()
	if s.isRunning {
		s.mutex.Unlock()
		log.Printf("[DriftScheduler] Already running")
		return
	}
	s.isRunning = true
	s.mutex.Unlock()

	s.ticker = time.NewTicker(interval)
	log.Printf("[DriftScheduler] Started with interval %v", interval)

	go func() {
		// 不在启动时立即执行检查，等待第一个 tick
		// 这样可以避免服务重启时立即为所有 workspace 创建任务
		log.Printf("[DriftScheduler] Waiting for first tick...")

		for {
			select {
			case <-ctx.Done():
				log.Println("[DriftScheduler] Stopped: context cancelled")
				return
			case <-s.ticker.C:
				s.checkWorkspaces()
			case <-s.stopChan:
				log.Printf("[DriftScheduler] Stopped")
				return
			}
		}
	}()
}

// Stop 停止调度器
func (s *DriftCheckScheduler) Stop() {
	s.mutex.Lock()
	defer s.mutex.Unlock()

	if !s.isRunning {
		return
	}

	if s.ticker != nil {
		s.ticker.Stop()
	}
	close(s.stopChan)
	s.isRunning = false
}

// checkWorkspaces 检查所有需要执行 drift 检测的 workspace
func (s *DriftCheckScheduler) checkWorkspaces() {
	log.Printf("[DriftScheduler] Checking workspaces for drift detection...")

	// 获取所有启用了 drift 检测的 workspace
	workspaces, err := s.driftService.GetWorkspacesNeedingCheck()
	if err != nil {
		log.Printf("[DriftScheduler] Failed to get workspaces: %v", err)
		return
	}

	log.Printf("[DriftScheduler] Found %d workspaces with drift check enabled", len(workspaces))

	for _, ws := range workspaces {
		// 检查是否已经存在 drift（如果已有 drift，不需要再检测）
		if s.hasDrift(ws.WorkspaceID) {
			log.Printf("[DriftScheduler] Workspace %s: already has drift, skipping (fix drift first)", ws.WorkspaceID)
			continue
		}

		// 检查是否在允许的时间窗口内
		if !s.driftService.IsInTimeWindow(&ws) {
			log.Printf("[DriftScheduler] Workspace %s: outside time window, skipping", ws.WorkspaceID)
			continue
		}

		// 检查是否应该运行（基于 drift_check_interval 配置）
		shouldRun, reason := s.driftService.ShouldRunDriftCheck(&ws)
		if !shouldRun {
			log.Printf("[DriftScheduler] Workspace %s: %s, skipping", ws.WorkspaceID, reason)
			continue
		}
		log.Printf("[DriftScheduler] Workspace %s: %s", ws.WorkspaceID, reason)

		// 检查 Agent 是否可用
		if !s.isAgentAvailable(&ws) {
			log.Printf("[DriftScheduler] Workspace %s: agent not available, skipping", ws.WorkspaceID)
			continue
		}

		// 检查是否有正在运行的任务
		if s.hasRunningTask(ws.WorkspaceID) {
			log.Printf("[DriftScheduler] Workspace %s: has running task, skipping", ws.WorkspaceID)
			continue
		}

		// 创建 drift check 任务
		if err := s.createDriftCheckTask(&ws); err != nil {
			log.Printf("[DriftScheduler] Workspace %s: failed to create task: %v", ws.WorkspaceID, err)
			continue
		}

		log.Printf("[DriftScheduler] Workspace %s: drift check task created", ws.WorkspaceID)
	}
}

// isAgentAvailable 检查 Agent 是否可用
func (s *DriftCheckScheduler) isAgentAvailable(ws *models.Workspace) bool {
	// Local 模式始终可用
	if ws.ExecutionMode == models.ExecutionModeLocal {
		return true
	}

	// Agent 模式：检查 pool 中是否有在线 agent
	if ws.ExecutionMode == models.ExecutionModeAgent {
		if ws.CurrentPoolID == nil || *ws.CurrentPoolID == "" {
			return false
		}

		var count int64
		err := s.db.Model(&models.Agent{}).
			Where("pool_id = ? AND status = ?", *ws.CurrentPoolID, "online").
			Count(&count).Error
		if err != nil {
			log.Printf("[DriftScheduler] Failed to check agent availability: %v", err)
			return false
		}
		return count > 0
	}

	// K8s 模式：假设始终可用（K8s 会自动创建 pod）
	if ws.ExecutionMode == models.ExecutionModeK8s {
		return true
	}

	return false
}

// hasRunningTask 检查是否有正在运行的任务
func (s *DriftCheckScheduler) hasRunningTask(workspaceID string) bool {
	var count int64
	err := s.db.Model(&models.WorkspaceTask{}).
		Where("workspace_id = ? AND status IN ?", workspaceID,
			[]string{"pending", "running", "waiting"}).
		Count(&count).Error
	if err != nil {
		log.Printf("[DriftScheduler] Failed to check running tasks: %v", err)
		return true // 保守起见，假设有任务在运行
	}
	return count > 0
}

// hasDrift 检查 workspace 是否已经存在 drift
// 如果已经存在 drift，不需要再运行检测，应该先修复
func (s *DriftCheckScheduler) hasDrift(workspaceID string) bool {
	var result models.WorkspaceDriftResult
	err := s.db.Where("workspace_id = ? AND has_drift = ?", workspaceID, true).First(&result).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false
		}
		log.Printf("[DriftScheduler] Failed to check drift status: %v", err)
		return false
	}
	return true
}

// createDriftCheckTask 创建 drift check 任务
func (s *DriftCheckScheduler) createDriftCheckTask(ws *models.Workspace) error {
	// 创建后台任务
	task := &models.WorkspaceTask{
		WorkspaceID:   ws.WorkspaceID,
		TaskType:      models.TaskTypeDriftCheck,
		Status:        models.TaskStatusPending,
		ExecutionMode: ws.ExecutionMode,
		IsBackground:  true, // 标记为后台任务
		Description:   "Automated drift detection",
	}

	// 保存任务
	if err := s.db.Create(task).Error; err != nil {
		s.driftService.UpdateDriftStatus(ws.WorkspaceID, models.DriftCheckStatusFailed, err.Error())
		return fmt.Errorf("failed to create task: %w", err)
	}

	// 更新 drift 状态为 running，并关联任务 ID
	if err := s.driftService.UpdateDriftStatusWithTaskID(ws.WorkspaceID, &task.ID, models.DriftCheckStatusRunning, ""); err != nil {
		log.Printf("[DriftScheduler] Failed to update drift status: %v", err)
	}

	log.Printf("[DriftScheduler] Created drift check task %d for workspace %s", task.ID, ws.WorkspaceID)

	// 触发任务队列执行
	if s.taskQueueManager != nil {
		go s.taskQueueManager.TryExecuteNextTask(ws.WorkspaceID)
	}

	return nil
}

// TriggerManualCheck 手动触发 drift 检测（不受每日限制）
func (s *DriftCheckScheduler) TriggerManualCheck(ctx context.Context, workspaceID string) error {
	// 获取 workspace
	var ws models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&ws).Error; err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	// 检查 Agent 是否可用
	if !s.isAgentAvailable(&ws) {
		return fmt.Errorf("agent not available for workspace %s", workspaceID)
	}

	// 检查是否有正在运行的任务
	if s.hasRunningTask(workspaceID) {
		return fmt.Errorf("workspace %s has running tasks", workspaceID)
	}

	// 创建任务
	return s.createDriftCheckTask(&ws)
}
