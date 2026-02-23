package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/internal/observability/metrics"
	"log"
	"regexp"
	"strings"
	"time"

	"gorm.io/gorm"
)

// 全局 TaskQueueManager 引用（用于手动触发 drift check）
var globalTaskQueueManager *TaskQueueManager

// SetGlobalTaskQueueManager 设置全局 TaskQueueManager
func SetGlobalTaskQueueManager(qm *TaskQueueManager) {
	globalTaskQueueManager = qm
}

// DriftCheckService Drift 检测服务
type DriftCheckService struct {
	db *gorm.DB
}

// NewDriftCheckService 创建 Drift 检测服务
func NewDriftCheckService(db *gorm.DB) *DriftCheckService {
	return &DriftCheckService{db: db}
}

// GetWorkspacesNeedingCheck 获取需要执行 drift 检测的 workspace 列表
func (s *DriftCheckService) GetWorkspacesNeedingCheck() ([]models.Workspace, error) {
	var workspaces []models.Workspace

	// 查询启用了 drift 检测的 workspace
	err := s.db.Where("drift_check_enabled = ?", true).Find(&workspaces).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get workspaces: %w", err)
	}

	return workspaces, nil
}

// IsInTimeWindow 检查当前时间是否在允许的检测时间窗口内
func (s *DriftCheckService) IsInTimeWindow(ws *models.Workspace) bool {
	now := time.Now()
	currentTime := now.Format("15:04:05")

	startTime := ws.DriftCheckStartTime
	endTime := ws.DriftCheckEndTime

	// 如果没有配置时间窗口，默认允许
	if startTime == "" || endTime == "" {
		return true
	}

	// 比较时间字符串
	return currentTime >= startTime && currentTime <= endTime
}

// HasRunToday 检查今天是否已经执行过 drift 检测
// Deprecated: 使用 ShouldRunDriftCheck 代替，它支持 drift_check_interval 配置
func (s *DriftCheckService) HasRunToday(workspaceID string) (bool, error) {
	var result models.WorkspaceDriftResult
	today := time.Now().Format("2006-01-02")

	err := s.db.Where("workspace_id = ? AND last_check_date = ?", workspaceID, today).First(&result).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// ShouldRunDriftCheck 检查是否应该执行 drift 检测
// 使用 workspace 的 drift_check_interval 配置（分钟）来判断
// 同时考虑 continue_on_failure 和 continue_on_success 设置
func (s *DriftCheckService) ShouldRunDriftCheck(ws *models.Workspace) (bool, string) {
	// 获取上次检测结果
	var result models.WorkspaceDriftResult
	err := s.db.Where("workspace_id = ?", ws.WorkspaceID).First(&result).Error

	if err != nil {
		if err == gorm.ErrRecordNotFound {
			// 从未运行过，应该运行
			return true, "never run before"
		}
		log.Printf("[DriftService] Failed to get drift result for %s: %v", ws.WorkspaceID, err)
		return false, fmt.Sprintf("failed to get drift result: %v", err)
	}

	// 如果没有上次检测时间，应该运行
	if result.LastCheckAt == nil {
		return true, "no last check time"
	}

	// 检查上次检测状态和继续设置
	// 如果上次检测失败且 continue_on_failure 为 false，不继续
	if result.CheckStatus == models.DriftCheckStatusFailed && !result.ContinueOnFailure {
		return false, "last check failed and continue_on_failure is disabled"
	}

	// 如果上次检测成功且 continue_on_success 为 false，不继续
	if result.CheckStatus == models.DriftCheckStatusSuccess && !result.ContinueOnSuccess {
		return false, "last check succeeded and continue_on_success is disabled"
	}

	// 获取配置的间隔（分钟），默认为 1440 分钟（24小时）
	intervalMinutes := ws.DriftCheckInterval
	if intervalMinutes <= 0 {
		intervalMinutes = 1440 // 默认每天一次
	}

	// 计算下次运行时间
	interval := time.Duration(intervalMinutes) * time.Minute
	nextRunTime := result.LastCheckAt.Add(interval)
	now := time.Now()

	if now.After(nextRunTime) {
		timeSinceLastCheck := now.Sub(*result.LastCheckAt)
		return true, fmt.Sprintf("interval elapsed (last check: %v ago, interval: %d min, status: %s)",
			timeSinceLastCheck.Round(time.Minute), intervalMinutes, result.CheckStatus)
	}

	timeUntilNextRun := nextRunTime.Sub(now)
	return false, fmt.Sprintf("next run in %v (interval: %d min)",
		timeUntilNextRun.Round(time.Minute), intervalMinutes)
}

// GetDriftResult 获取 workspace 的 drift 检测结果
func (s *DriftCheckService) GetDriftResult(workspaceID string) (*models.WorkspaceDriftResult, error) {
	var result models.WorkspaceDriftResult

	err := s.db.Where("workspace_id = ?", workspaceID).First(&result).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, err
	}

	return &result, nil
}

// UpdateDriftStatus 更新 drift 检测状态
func (s *DriftCheckService) UpdateDriftStatus(workspaceID string, status models.DriftCheckStatus, errorMsg string) error {
	return s.UpdateDriftStatusWithTaskID(workspaceID, nil, status, errorMsg)
}

// UpdateDriftStatusWithTaskID 更新 drift 检测状态（带任务 ID）
func (s *DriftCheckService) UpdateDriftStatusWithTaskID(workspaceID string, taskID *uint, status models.DriftCheckStatus, errorMsg string) error {
	now := time.Now()
	today := time.Now().Truncate(24 * time.Hour)

	updates := map[string]interface{}{
		"check_status":  status,
		"error_message": errorMsg,
		"updated_at":    now,
	}

	// 如果提供了 taskID，更新 current_task_id
	if taskID != nil {
		updates["current_task_id"] = *taskID
	}

	// 如果状态是成功或失败，清除 current_task_id 并更新 last_check_at/last_check_date
	// 这样即使失败了，今天也不会再重试
	if status == models.DriftCheckStatusSuccess || status == models.DriftCheckStatusFailed {
		updates["current_task_id"] = nil
		updates["last_check_at"] = now
		updates["last_check_date"] = today
	}

	// 使用 upsert 操作
	result := models.WorkspaceDriftResult{
		WorkspaceID:  workspaceID,
		CheckStatus:  status,
		ErrorMessage: errorMsg,
		UpdatedAt:    now,
	}

	return s.db.Where("workspace_id = ?", workspaceID).
		Assign(updates).
		FirstOrCreate(&result).Error
}

// SaveDriftResult 保存 drift 检测结果
func (s *DriftCheckService) SaveDriftResult(workspaceID string, driftDetails *models.DriftDetailsJSON, hasDrift bool, driftCount, totalResources int) error {
	now := time.Now()
	today := time.Now().Truncate(24 * time.Hour)

	// 使用 upsert 操作
	result := models.WorkspaceDriftResult{
		WorkspaceID:    workspaceID,
		HasDrift:       hasDrift,
		DriftCount:     driftCount,
		TotalResources: totalResources,
		DriftDetails:   driftDetails,
		CheckStatus:    models.DriftCheckStatusSuccess,
		LastCheckAt:    &now,
		LastCheckDate:  &today,
		UpdatedAt:      now,
	}

	err := s.db.Where("workspace_id = ?", workspaceID).
		Assign(map[string]interface{}{
			"has_drift":       hasDrift,
			"drift_count":     driftCount,
			"total_resources": totalResources,
			"drift_details":   driftDetails,
			"check_status":    models.DriftCheckStatusSuccess,
			"error_message":   "",
			"last_check_at":   now,
			"last_check_date": today,
			"updated_at":      now,
		}).
		FirstOrCreate(&result).Error

	if err != nil {
		return fmt.Errorf("failed to save drift result: %w", err)
	}

	// 更新 workspace 的统计字段
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(map[string]interface{}{
			"drift_count":      driftCount,
			"last_drift_check": now,
		}).Error
}

// GetResourceDriftStatuses 获取 workspace 下所有资源的 drift 状态
func (s *DriftCheckService) GetResourceDriftStatuses(workspaceID string) ([]models.ResourceDriftStatus, error) {
	// 获取所有活跃资源
	var resources []models.WorkspaceResource
	err := s.db.Where("workspace_id = ? AND is_active = ?", workspaceID, true).Find(&resources).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get resources: %w", err)
	}

	// 获取 drift 检测结果
	driftResult, err := s.GetDriftResult(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get drift result: %w", err)
	}

	// 构建资源 drift 状态映射
	driftMap := make(map[uint]models.DriftResource)
	if driftResult != nil && driftResult.DriftDetails != nil {
		for _, r := range driftResult.DriftDetails.Resources {
			driftMap[r.ResourceID] = r
		}
	}

	// 构建响应
	statuses := make([]models.ResourceDriftStatus, 0, len(resources))
	for _, resource := range resources {
		status := models.ResourceDriftStatus{
			ResourceID:    resource.ID,
			ResourceName:  resource.ResourceName,
			LastAppliedAt: resource.LastAppliedAt,
		}

		// 检查 drift_details 中是否有该资源的信息
		if driftInfo, ok := driftMap[resource.ID]; ok {
			if driftInfo.HasDrift && len(driftInfo.DriftedChildren) > 0 {
				// 检查子资源的 action 类型
				// create 类型表示资源未应用（unapplied），不是 drift
				// update/delete 类型才是真正的 drift
				hasRealDrift := false
				realDriftCount := 0
				for _, child := range driftInfo.DriftedChildren {
					if child.Action == "update" || child.Action == "delete" || child.Action == "replace" {
						hasRealDrift = true
						realDriftCount++
					}
				}

				if hasRealDrift {
					// 有真正的 drift（update/delete/replace）
					status.Status = "drifted"
					status.HasDrift = true
					status.DriftedChildrenCount = realDriftCount
				} else {
					// 只有 create 类型，标记为 unapplied
					status.Status = "unapplied"
					status.HasDrift = false
					status.DriftedChildrenCount = 0
				}
			} else {
				// 资源在 drift_details 中存在但没有变更，说明云端状态与代码一致
				status.Status = "synced"
				status.HasDrift = false
				status.DriftedChildrenCount = 0
			}
		} else if resource.LastAppliedAt == nil {
			// 资源不在 drift_details 中且从未 apply 过
			status.Status = "unapplied"
			status.HasDrift = false
		} else {
			// 资源不在 drift_details 中但已 apply 过（可能是 drift 检测还没运行）
			status.Status = "synced"
			status.HasDrift = false
		}

		statuses = append(statuses, status)
	}

	return statuses, nil
}

// GetDriftConfig 获取 workspace 的 drift 配置
func (s *DriftCheckService) GetDriftConfig(workspaceID string) (*models.DriftConfigResponse, error) {
	// 使用原生 SQL 查询，确保正确读取 time 类型字段
	var result struct {
		DriftCheckEnabled   bool   `gorm:"column:drift_check_enabled"`
		DriftCheckStartTime string `gorm:"column:drift_check_start_time"`
		DriftCheckEndTime   string `gorm:"column:drift_check_end_time"`
		DriftCheckInterval  int    `gorm:"column:drift_check_interval"`
	}

	err := s.db.Raw(`
		SELECT 
			drift_check_enabled,
			TO_CHAR(drift_check_start_time, 'HH24:MI') as drift_check_start_time,
			TO_CHAR(drift_check_end_time, 'HH24:MI') as drift_check_end_time,
			drift_check_interval
		FROM workspaces 
		WHERE workspace_id = ?
	`, workspaceID).Scan(&result).Error

	if err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// 获取继续检测设置
	var driftResult models.WorkspaceDriftResult
	continueOnFailure := false
	continueOnSuccess := false

	if err := s.db.Where("workspace_id = ?", workspaceID).First(&driftResult).Error; err == nil {
		continueOnFailure = driftResult.ContinueOnFailure
		continueOnSuccess = driftResult.ContinueOnSuccess
	}

	log.Printf("[DriftConfig] GetDriftConfig for %s: enabled=%v, start=%s, end=%s, interval=%d, continueOnFailure=%v, continueOnSuccess=%v",
		workspaceID, result.DriftCheckEnabled, result.DriftCheckStartTime, result.DriftCheckEndTime, result.DriftCheckInterval,
		continueOnFailure, continueOnSuccess)

	return &models.DriftConfigResponse{
		DriftCheckEnabled:   result.DriftCheckEnabled,
		DriftCheckStartTime: result.DriftCheckStartTime,
		DriftCheckEndTime:   result.DriftCheckEndTime,
		DriftCheckInterval:  result.DriftCheckInterval,
		ContinueOnFailure:   continueOnFailure,
		ContinueOnSuccess:   continueOnSuccess,
	}, nil
}

// UpdateDriftConfig 更新 workspace 的 drift 配置（部分更新）
func (s *DriftCheckService) UpdateDriftConfig(workspaceID string, req *models.DriftConfigRequest) error {
	updates := make(map[string]interface{})

	if req.DriftCheckEnabled != nil {
		updates["drift_check_enabled"] = *req.DriftCheckEnabled
	}
	if req.DriftCheckStartTime != nil {
		updates["drift_check_start_time"] = *req.DriftCheckStartTime
	}
	if req.DriftCheckEndTime != nil {
		updates["drift_check_end_time"] = *req.DriftCheckEndTime
	}
	if req.DriftCheckInterval != nil {
		updates["drift_check_interval"] = *req.DriftCheckInterval
	}

	if len(updates) == 0 {
		return nil
	}

	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates).Error
}

// UpdateDriftConfigFull 更新 workspace 的 drift 配置（完整更新）
func (s *DriftCheckService) UpdateDriftConfigFull(workspaceID string, req *models.DriftConfigUpdateRequest) error {
	log.Printf("[DriftConfig] UpdateDriftConfigFull for %s: enabled=%v, start=%s, end=%s, interval=%d, continueOnFailure=%v, continueOnSuccess=%v",
		workspaceID, req.DriftCheckEnabled, req.DriftCheckStartTime, req.DriftCheckEndTime, req.DriftCheckInterval,
		req.ContinueOnFailure, req.ContinueOnSuccess)

	// 更新 workspace 表中的基本配置
	updates := map[string]interface{}{
		"drift_check_enabled":    req.DriftCheckEnabled,
		"drift_check_start_time": req.DriftCheckStartTime,
		"drift_check_end_time":   req.DriftCheckEndTime,
		"drift_check_interval":   req.DriftCheckInterval,
	}

	result := s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Updates(updates)

	if result.Error != nil {
		log.Printf("[DriftConfig] UpdateDriftConfigFull error: %v", result.Error)
		return result.Error
	}

	// 更新 workspace_drift_results 表中的继续检测设置
	if err := s.UpdateContinueSettings(workspaceID, req.ContinueOnFailure, req.ContinueOnSuccess); err != nil {
		log.Printf("[DriftConfig] UpdateContinueSettings error: %v", err)
		return err
	}

	log.Printf("[DriftConfig] UpdateDriftConfigFull success, rows affected: %d", result.RowsAffected)
	return nil
}

// UpdateContinueSettings 更新继续检测设置
func (s *DriftCheckService) UpdateContinueSettings(workspaceID string, continueOnFailure, continueOnSuccess bool) error {
	now := time.Now()

	// 使用 upsert 操作
	result := models.WorkspaceDriftResult{
		WorkspaceID:       workspaceID,
		ContinueOnFailure: continueOnFailure,
		ContinueOnSuccess: continueOnSuccess,
		UpdatedAt:         now,
	}

	err := s.db.Where("workspace_id = ?", workspaceID).
		Assign(map[string]interface{}{
			"continue_on_failure": continueOnFailure,
			"continue_on_success": continueOnSuccess,
			"updated_at":          now,
		}).
		FirstOrCreate(&result).Error

	if err != nil {
		return fmt.Errorf("failed to update continue settings: %w", err)
	}

	log.Printf("[DriftConfig] UpdateContinueSettings for %s: continueOnFailure=%v, continueOnSuccess=%v",
		workspaceID, continueOnFailure, continueOnSuccess)

	return nil
}

// GetContinueSettings 获取继续检测设置
func (s *DriftCheckService) GetContinueSettings(workspaceID string) (continueOnFailure, continueOnSuccess bool) {
	var result models.WorkspaceDriftResult
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&result).Error; err != nil {
		return false, false
	}

	return result.ContinueOnFailure, result.ContinueOnSuccess
}

// ClearDriftOnFullApply 全量 apply 后清除 drift 状态
func (s *DriftCheckService) ClearDriftOnFullApply(workspaceID string) error {
	now := time.Now()

	// 更新所有资源的 last_applied_at
	err := s.db.Model(&models.WorkspaceResource{}).
		Where("workspace_id = ? AND is_active = ?", workspaceID, true).
		Update("last_applied_at", now).Error
	if err != nil {
		return fmt.Errorf("failed to update resources: %w", err)
	}

	// 清除 drift 结果
	return s.db.Model(&models.WorkspaceDriftResult{}).
		Where("workspace_id = ?", workspaceID).
		Updates(map[string]interface{}{
			"has_drift":     false,
			"drift_count":   0,
			"drift_details": nil,
			"updated_at":    now,
		}).Error
}

// ParsePlanOutputForDrift 解析 plan 输出，提取 drift 信息
func (s *DriftCheckService) ParsePlanOutputForDrift(workspaceID string, planJSON map[string]interface{}) (*models.DriftDetailsJSON, error) {
	// 获取 workspace 的资源列表
	var resources []models.WorkspaceResource
	err := s.db.Where("workspace_id = ? AND is_active = ?", workspaceID, true).Find(&resources).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get resources: %w", err)
	}

	// 构建资源名称到 ID 的映射
	// 模块名称格式: {resource_type}_{resource_name}
	resourceMap := make(map[string]models.WorkspaceResource)
	for _, r := range resources {
		// 使用完整的模块名称格式
		moduleName := fmt.Sprintf("%s_%s", r.ResourceType, r.ResourceName)
		resourceMap[moduleName] = r
		log.Printf("[DriftCheck] Registered resource: %s (id=%d)", moduleName, r.ID)
	}

	// 解析 plan_json 中的 resource_changes
	resourceChanges, ok := planJSON["resource_changes"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid plan_json: missing resource_changes")
	}

	// 按 module 分组 drift 信息
	moduleChanges := make(map[string][]models.DriftedChild)

	for _, rc := range resourceChanges {
		changeMap, ok := rc.(map[string]interface{})
		if !ok {
			continue
		}

		// 获取 change 详情
		change, ok := changeMap["change"].(map[string]interface{})
		if !ok {
			continue
		}

		// 获取 actions
		actions, ok := change["actions"].([]interface{})
		if !ok || len(actions) == 0 {
			continue
		}

		// 跳过 no-op
		if len(actions) == 1 {
			if actionStr, ok := actions[0].(string); ok && actionStr == "no-op" {
				continue
			}
		}

		// 获取资源地址
		address, _ := changeMap["address"].(string)
		resourceType, _ := changeMap["type"].(string)
		resourceName, _ := changeMap["name"].(string)

		// 提取 module 名称
		moduleName := extractModuleName(address)
		if moduleName == "" {
			continue
		}

		// 确定 action
		action := determineActionFromActions(actions)

		// 提取变更详情
		changes := extractChanges(change)

		// 添加到 module 的变更列表
		moduleChanges[moduleName] = append(moduleChanges[moduleName], models.DriftedChild{
			Address: address,
			Type:    resourceType,
			Name:    resourceName,
			Action:  action,
			Changes: changes,
		})
	}

	// 构建 drift 详情
	driftResources := make([]models.DriftResource, 0)
	for moduleName, children := range moduleChanges {
		// 查找对应的 workspace resource
		resource, exists := resourceMap[moduleName]
		if !exists {
			log.Printf("[DriftCheck] Module %s not found in workspace resources", moduleName)
			continue
		}

		driftResources = append(driftResources, models.DriftResource{
			ResourceID:      resource.ID,
			ResourceName:    resource.ResourceName,
			ResourceType:    resource.ResourceType,
			HasDrift:        len(children) > 0,
			DriftedChildren: children,
		})
	}

	// 添加没有 drift 的资源
	for _, resource := range resources {
		found := false
		for _, dr := range driftResources {
			if dr.ResourceID == resource.ID {
				found = true
				break
			}
		}
		if !found {
			driftResources = append(driftResources, models.DriftResource{
				ResourceID:      resource.ID,
				ResourceName:    resource.ResourceName,
				ResourceType:    resource.ResourceType,
				HasDrift:        false,
				DriftedChildren: nil,
			})
		}
	}

	// 获取 terraform 版本
	tfVersion := ""
	if v, ok := planJSON["terraform_version"].(string); ok {
		tfVersion = v
	}

	details := &models.DriftDetailsJSON{
		CheckTime:         time.Now().Format(time.RFC3339),
		TerraformVersion:  tfVersion,
		PlanOutputSummary: generatePlanSummary(planJSON),
		Resources:         driftResources,
	}

	return (*models.DriftDetailsJSON)(details), nil
}

// ProcessDriftCheckResult 处理 drift check 任务完成后的结果
func (s *DriftCheckService) ProcessDriftCheckResult(task *models.WorkspaceTask) error {
	if task.TaskType != models.TaskTypeDriftCheck {
		return nil
	}

	log.Printf("[DriftCheck] Processing drift check result for task %d (workspace %s, status %s)",
		task.ID, task.WorkspaceID, task.Status)

	// 检查任务状态
	if task.Status == models.TaskStatusFailed || task.Status == models.TaskStatusCancelled {
		return s.UpdateDriftStatus(task.WorkspaceID, models.DriftCheckStatusFailed, task.ErrorMessage)
	}

	// 直接从 workspace_task_resource_changes 表读取资源变更
	var resourceChanges []models.WorkspaceTaskResourceChange
	err := s.db.Where("task_id = ?", task.ID).Find(&resourceChanges).Error
	if err != nil {
		log.Printf("[DriftCheck] Failed to get resource changes: %v", err)
		return s.UpdateDriftStatus(task.WorkspaceID, models.DriftCheckStatusFailed, err.Error())
	}

	log.Printf("[DriftCheck] Found %d resource changes for task %d", len(resourceChanges), task.ID)

	// 获取 workspace 的资源列表
	var resources []models.WorkspaceResource
	err = s.db.Where("workspace_id = ? AND is_active = ?", task.WorkspaceID, true).Find(&resources).Error
	if err != nil {
		return s.UpdateDriftStatus(task.WorkspaceID, models.DriftCheckStatusFailed, err.Error())
	}

	// 构建资源映射（模块名称 -> 资源）
	resourceMap := make(map[string]models.WorkspaceResource)
	for _, r := range resources {
		moduleName := fmt.Sprintf("%s_%s", r.ResourceType, r.ResourceName)
		resourceMap[moduleName] = r
	}

	// 按模块分组资源变更
	moduleChanges := make(map[string][]models.DriftedChild)
	for _, rc := range resourceChanges {
		// 跳过 no-op
		if rc.Action == "no-op" {
			continue
		}

		// 从 resource_address 提取模块名称
		moduleName := extractModuleName(rc.ResourceAddress)
		if moduleName == "" {
			log.Printf("[DriftCheck] Could not extract module name from: %s", rc.ResourceAddress)
			continue
		}

		// 提取变更详情
		changes := make(map[string]models.DriftChangeDetail)
		if rc.ChangesBefore != nil && rc.ChangesAfter != nil {
			// JSONB 已经是 map[string]interface{} 类型
			changes = extractChanges(map[string]interface{}{
				"before": map[string]interface{}(rc.ChangesBefore),
				"after":  map[string]interface{}(rc.ChangesAfter),
			})
		}

		moduleChanges[moduleName] = append(moduleChanges[moduleName], models.DriftedChild{
			Address: rc.ResourceAddress,
			Type:    rc.ResourceType,
			Name:    rc.ResourceName,
			Action:  rc.Action,
			Changes: changes,
		})
	}

	// 构建 drift 详情
	driftResources := make([]models.DriftResource, 0)
	for moduleName, children := range moduleChanges {
		resource, exists := resourceMap[moduleName]
		if !exists {
			log.Printf("[DriftCheck] Module %s not found in workspace resources", moduleName)
			continue
		}

		driftResources = append(driftResources, models.DriftResource{
			ResourceID:      resource.ID,
			ResourceName:    resource.ResourceName,
			ResourceType:    resource.ResourceType,
			HasDrift:        len(children) > 0,
			DriftedChildren: children,
		})
	}

	// 添加没有 drift 的资源
	for _, resource := range resources {
		found := false
		for _, dr := range driftResources {
			if dr.ResourceID == resource.ID {
				found = true
				break
			}
		}
		if !found {
			driftResources = append(driftResources, models.DriftResource{
				ResourceID:      resource.ID,
				ResourceName:    resource.ResourceName,
				ResourceType:    resource.ResourceType,
				HasDrift:        false,
				DriftedChildren: nil,
			})
		}
	}

	// 计算统计
	driftCount := 0
	for _, r := range driftResources {
		if r.HasDrift {
			driftCount++
		}
	}

	log.Printf("[DriftCheck] Drift result: %d resources with drift out of %d total",
		driftCount, len(driftResources))

	// 生成 plan 摘要
	planSummary := ""
	if task.PlanJSON != nil {
		planSummary = generatePlanSummary(task.PlanJSON)
	}

	details := &models.DriftDetailsJSON{
		CheckTime:         time.Now().Format(time.RFC3339),
		TerraformVersion:  "",
		PlanOutputSummary: planSummary,
		Resources:         driftResources,
	}

	// Record drift detection metric
	metrics.RecordDriftDetected(driftCount > 0)

	// 保存结果
	return s.SaveDriftResult(task.WorkspaceID, details, driftCount > 0, driftCount, len(resources))
}

// extractModuleName 从资源地址中提取 module 名称
func extractModuleName(address string) string {
	// 格式: module.xxx.resource_type.resource_name
	re := regexp.MustCompile(`^module\.([^.]+)\.`)
	matches := re.FindStringSubmatch(address)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

// determineActionFromActions 从 actions 数组确定操作类型
func determineActionFromActions(actions []interface{}) string {
	if len(actions) == 1 {
		if action, ok := actions[0].(string); ok {
			return action
		}
	}

	// ["delete", "create"] = replace
	if len(actions) == 2 {
		action0, ok0 := actions[0].(string)
		action1, ok1 := actions[1].(string)
		if ok0 && ok1 && action0 == "delete" && action1 == "create" {
			return "replace"
		}
	}

	return "unknown"
}

// extractChanges 提取变更详情
func extractChanges(change map[string]interface{}) map[string]models.DriftChangeDetail {
	changes := make(map[string]models.DriftChangeDetail)

	before, _ := change["before"].(map[string]interface{})
	after, _ := change["after"].(map[string]interface{})

	if before == nil && after == nil {
		return changes
	}

	// 比较 before 和 after，找出变更的字段
	allKeys := make(map[string]bool)
	for k := range before {
		allKeys[k] = true
	}
	for k := range after {
		allKeys[k] = true
	}

	for key := range allKeys {
		beforeVal := before[key]
		afterVal := after[key]

		// 比较值是否相同
		beforeJSON, _ := json.Marshal(beforeVal)
		afterJSON, _ := json.Marshal(afterVal)

		if string(beforeJSON) != string(afterJSON) {
			changes[key] = models.DriftChangeDetail{
				Before: beforeVal,
				After:  afterVal,
			}
		}
	}

	return changes
}

// TriggerManualDriftCheck 手动触发 drift 检测
func (s *DriftCheckService) TriggerManualDriftCheck(workspaceID string) error {
	// 获取 workspace
	var ws models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&ws).Error; err != nil {
		return fmt.Errorf("workspace not found: %w", err)
	}

	// 检查 Agent 是否可用
	if !s.isAgentAvailable(&ws) {
		return fmt.Errorf("agent not available for workspace %s", workspaceID)
	}

	// 检查是否已有 pending/running 的 drift_check 任务
	if s.hasPendingDriftCheckTask(workspaceID) {
		return fmt.Errorf("workspace %s already has a pending drift check task", workspaceID)
	}

	// 检查是否有正在运行的其他任务
	if s.hasRunningTask(workspaceID) {
		return fmt.Errorf("workspace %s has running tasks", workspaceID)
	}

	// 创建任务
	return s.createDriftCheckTask(&ws)
}

// isAgentAvailable 检查 Agent 是否可用
func (s *DriftCheckService) isAgentAvailable(ws *models.Workspace) bool {
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
			log.Printf("[DriftService] Failed to check agent availability: %v", err)
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

// hasRunningTask 检查是否有正在运行的任务（非 drift_check 类型）
func (s *DriftCheckService) hasRunningTask(workspaceID string) bool {
	var count int64
	err := s.db.Model(&models.WorkspaceTask{}).
		Where("workspace_id = ? AND task_type != ? AND status IN ?", workspaceID,
			models.TaskTypeDriftCheck,
			[]string{"pending", "running", "waiting"}).
		Count(&count).Error
	if err != nil {
		log.Printf("[DriftService] Failed to check running tasks: %v", err)
		return true // 保守起见，假设有任务在运行
	}
	return count > 0
}

// hasPendingDriftCheckTask 检查是否已有 pending 的 drift_check 任务
func (s *DriftCheckService) hasPendingDriftCheckTask(workspaceID string) bool {
	var count int64
	err := s.db.Model(&models.WorkspaceTask{}).
		Where("workspace_id = ? AND task_type = ? AND status IN ?", workspaceID,
			models.TaskTypeDriftCheck,
			[]string{"pending", "running"}).
		Count(&count).Error
	if err != nil {
		log.Printf("[DriftService] Failed to check pending drift_check tasks: %v", err)
		return true // 保守起见，假设有任务在运行
	}
	return count > 0
}

// createDriftCheckTask 创建 drift check 任务
func (s *DriftCheckService) createDriftCheckTask(ws *models.Workspace) error {
	// 创建后台任务
	task := &models.WorkspaceTask{
		WorkspaceID:   ws.WorkspaceID,
		TaskType:      models.TaskTypeDriftCheck,
		Status:        models.TaskStatusPending,
		ExecutionMode: ws.ExecutionMode,
		IsBackground:  true, // 标记为后台任务
		Description:   "Manual drift detection",
	}

	// 保存任务
	if err := s.db.Create(task).Error; err != nil {
		s.UpdateDriftStatus(ws.WorkspaceID, models.DriftCheckStatusFailed, err.Error())
		return fmt.Errorf("failed to create task: %w", err)
	}

	// 更新 drift 状态为 running，并关联任务 ID
	if err := s.UpdateDriftStatusWithTaskID(ws.WorkspaceID, &task.ID, models.DriftCheckStatusRunning, ""); err != nil {
		log.Printf("[DriftService] Failed to update drift status: %v", err)
	}

	log.Printf("[DriftService] Created drift check task %d for workspace %s", task.ID, ws.WorkspaceID)

	// 触发任务队列执行
	if globalTaskQueueManager != nil {
		go globalTaskQueueManager.TryExecuteNextTask(ws.WorkspaceID)
	}

	return nil
}

// ProcessDriftCheckResultByTaskID 通过任务 ID 处理 drift check 结果
// 用于 Agent 模式下任务完成/失败时的回调
func (s *DriftCheckService) ProcessDriftCheckResultByTaskID(taskID uint) error {
	// 获取任务
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return fmt.Errorf("task not found: %w", err)
	}

	// 检查是否是 drift_check 任务
	if task.TaskType != models.TaskTypeDriftCheck {
		return nil // 不是 drift_check 任务，忽略
	}

	log.Printf("[DriftService] Processing drift check result for task %d (workspace %s, status %s)",
		taskID, task.WorkspaceID, task.Status)

	// 处理结果
	return s.ProcessDriftCheckResult(&task)
}

// generatePlanSummary 生成 plan 摘要
func generatePlanSummary(planJSON map[string]interface{}) string {
	add, change, destroy := 0, 0, 0

	if resourceChanges, ok := planJSON["resource_changes"].([]interface{}); ok {
		for _, rc := range resourceChanges {
			if changeMap, ok := rc.(map[string]interface{}); ok {
				if changeDetail, ok := changeMap["change"].(map[string]interface{}); ok {
					if actions, ok := changeDetail["actions"].([]interface{}); ok {
						for _, action := range actions {
							switch action.(string) {
							case "create":
								add++
							case "update":
								change++
							case "delete":
								destroy++
							}
						}
					}
				}
			}
		}
	}

	parts := []string{}
	if add > 0 {
		parts = append(parts, fmt.Sprintf("%d to add", add))
	}
	if change > 0 {
		parts = append(parts, fmt.Sprintf("%d to change", change))
	}
	if destroy > 0 {
		parts = append(parts, fmt.Sprintf("%d to destroy", destroy))
	}

	if len(parts) == 0 {
		return "No changes."
	}

	return "Plan: " + strings.Join(parts, ", ") + "."
}
