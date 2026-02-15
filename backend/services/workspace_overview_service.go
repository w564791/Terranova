package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// WorkspaceOverviewService Overview服务
type WorkspaceOverviewService struct {
	db *gorm.DB
}

// NewWorkspaceOverviewService 创建Overview服务
func NewWorkspaceOverviewService(db *gorm.DB) *WorkspaceOverviewService {
	return &WorkspaceOverviewService{db: db}
}

// ResourceSummary 资源摘要
type ResourceSummary struct {
	Type  string `json:"type"`  // 资源类型，如 aws_instance
	Count int    `json:"count"` // 数量
}

// LatestRunInfo 最近运行信息
type LatestRunInfo struct {
	ID             uint      `json:"id"`
	TaskType       string    `json:"task_type"` // 添加task_type字段
	Message        string    `json:"message"`
	CreatedBy      string    `json:"created_by"`
	Status         string    `json:"status"`
	PlanDuration   int       `json:"plan_duration"`  // 秒
	ApplyDuration  int       `json:"apply_duration"` // 秒
	ChangesAdd     int       `json:"changes_add"`
	ChangesChange  int       `json:"changes_change"`
	ChangesDestroy int       `json:"changes_destroy"`
	CreatedAt      time.Time `json:"created_at"`
}

// WorkspaceOverviewResponse Overview响应
type WorkspaceOverviewResponse struct {
	// 基本信息
	ID               uint    `json:"id"`           // 数字ID (内部使用)
	WorkspaceID      string  `json:"workspace_id"` // 语义化ID (对外使用)
	Name             string  `json:"name"`
	Description      string  `json:"description"`
	IsLocked         bool    `json:"is_locked"`
	LockedBy         *string `json:"locked_by"`
	LockedByUsername string  `json:"locked_by_username,omitempty"` // 锁定者用户名
	LockReason       string  `json:"lock_reason"`

	// 执行配置
	ExecutionMode    string `json:"execution_mode"`
	TerraformVersion string `json:"terraform_version"`
	WorkingDirectory string `json:"working_directory"`
	AutoApply        bool   `json:"auto_apply"`

	// 统计信息
	ResourceCount  int        `json:"resource_count"`
	LastPlanAt     *time.Time `json:"last_plan_at"`
	LastApplyAt    *time.Time `json:"last_apply_at"`
	DriftCount     int        `json:"drift_count"`
	LastDriftCheck *time.Time `json:"last_drift_check"`

	// 最近运行
	LatestRun *LatestRunInfo `json:"latest_run"`

	// 资源列表
	Resources []ResourceSummary `json:"resources"`

	// 时间戳
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// GetWorkspaceOverview 获取Workspace Overview
func (s *WorkspaceOverviewService) GetWorkspaceOverview(workspaceID string) (*WorkspaceOverviewResponse, error) {
	// 查询Workspace
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, fmt.Errorf("workspace not found: %w", err)
	}

	// 获取最新 state version 并实时计算资源数量
	var resourceCount int
	var stateVersion models.WorkspaceStateVersion
	err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error
	
	if err == nil && stateVersion.Content != nil {
		// 实时从 state JSON 的 resources 数组计算资源数量
		if resources, ok := stateVersion.Content["resources"].([]interface{}); ok {
			resourceCount = len(resources)
		}
	}

	// 构建基础响应
	response := &WorkspaceOverviewResponse{
		ID:               workspace.ID,          // 数字ID
		WorkspaceID:      workspace.WorkspaceID, // 语义化ID
		Name:             workspace.Name,
		Description:      workspace.Description,
		IsLocked:         workspace.IsLocked,
		LockedBy:         workspace.LockedBy,
		LockReason:       workspace.LockReason,
		ExecutionMode:    string(workspace.ExecutionMode),
		TerraformVersion: workspace.TerraformVersion,
		WorkingDirectory: workspace.Workdir,
		AutoApply:        workspace.AutoApply,
		ResourceCount:    resourceCount,
		LastPlanAt:       workspace.LastPlanAt,
		LastApplyAt:      workspace.LastApplyAt,
		DriftCount:       workspace.DriftCount,
		LastDriftCheck:   workspace.LastDriftCheck,
		CreatedAt:        workspace.CreatedAt,
		UpdatedAt:        workspace.UpdatedAt,
	}

	// 如果workspace被锁定，查询锁定者的用户名
	if workspace.IsLocked && workspace.LockedBy != nil {
		var username string
		err := s.db.Table("users").
			Select("username").
			Where("id = ?", *workspace.LockedBy).
			Scan(&username).Error

		if err == nil && username != "" {
			response.LockedByUsername = username
		}
	}

	// 获取最近运行 (使用内部数字ID)
	latestRun, err := s.getLatestRun(workspace.WorkspaceID)
	if err == nil {
		response.LatestRun = latestRun
	}

	// 获取资源列表 (使用内部数字ID)
	resources, err := s.getResourceSummary(workspace.WorkspaceID)
	if err == nil {
		response.Resources = resources
	}

	return response, nil
}

// getLatestRun 获取最近运行
// 优先级：1. Needs Attention任务 2. Running任务 3. 最新任务
func (s *WorkspaceOverviewService) getLatestRun(workspaceID string) (*LatestRunInfo, error) {
	var task models.WorkspaceTask

	// 1. 优先查找Needs Attention任务（apply_pending）
	// 使用 Take 而不是 First 来避免 record not found 日志
	err := s.db.Where("workspace_id = ? AND status = ?",
		workspaceID,
		models.TaskStatusApplyPending).
		Order("created_at DESC").
		Take(&task).Error

	if err == nil {
		// 找到Needs Attention任务
		goto buildResponse
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 2. 查找running任务
	// 使用 Take 而不是 First 来避免 record not found 日志
	err = s.db.Where("workspace_id = ? AND status = ?", workspaceID, models.TaskStatusRunning).
		Order("created_at DESC").
		Take(&task).Error

	if err == nil {
		// 找到running任务
		goto buildResponse
	}

	if err != gorm.ErrRecordNotFound {
		return nil, err
	}

	// 3. 获取最新任务
	// 使用 Take 而不是 First 来避免 record not found 日志
	err = s.db.Where("workspace_id = ?", workspaceID).
		Order("created_at DESC").
		Take(&task).Error

	if err != nil {
		return nil, err
	}

buildResponse:

	// 计算持续时间
	planDuration := 0
	applyDuration := 0
	if task.StartedAt != nil && task.CompletedAt != nil {
		duration := task.CompletedAt.Sub(*task.StartedAt).Seconds()
		if task.TaskType == models.TaskTypePlan {
			planDuration = int(duration)
		} else if task.TaskType == models.TaskTypeApply {
			applyDuration = int(duration)
		}
	}

	// 获取创建者信息（简化版，实际应该查询users表）
	createdBy := "unknown"
	if task.CreatedBy != nil {
		createdBy = fmt.Sprintf("user_%s", *task.CreatedBy)
	}

	return &LatestRunInfo{
		ID:             task.ID,
		TaskType:       string(task.TaskType), // 添加task_type
		Message:        fmt.Sprintf("%s task", task.TaskType),
		CreatedBy:      createdBy,
		Status:         string(task.Status),
		PlanDuration:   planDuration,
		ApplyDuration:  applyDuration,
		ChangesAdd:     task.ChangesAdd,
		ChangesChange:  task.ChangesChange,
		ChangesDestroy: task.ChangesDestroy,
		CreatedAt:      task.CreatedAt,
	}, nil
}

// getResourceSummary 获取资源摘要
func (s *WorkspaceOverviewService) getResourceSummary(workspaceID string) ([]ResourceSummary, error) {
	// 获取最新的State版本
	var stateVersion models.WorkspaceStateVersion
	err := s.db.Where("workspace_id = ?", workspaceID).
		Order("version DESC").
		First(&stateVersion).Error

	if err != nil {
		return []ResourceSummary{}, nil // 没有State时返回空列表
	}

	// 解析State内容
	resources, err := s.parseStateResources(stateVersion.Content)
	if err != nil {
		return []ResourceSummary{}, nil
	}

	return resources, nil
}

// parseStateResources 解析State中的资源
func (s *WorkspaceOverviewService) parseStateResources(stateContent models.JSONB) ([]ResourceSummary, error) {
	// Terraform State结构
	type TerraformState struct {
		Resources []struct {
			Type      string `json:"type"`
			Instances []struct {
				Attributes map[string]interface{} `json:"attributes"`
			} `json:"instances"`
		} `json:"resources"`
	}

	// 将JSONB转换为JSON字符串
	jsonBytes, err := json.Marshal(stateContent)
	if err != nil {
		return nil, err
	}

	// 解析State
	var state TerraformState
	if err := json.Unmarshal(jsonBytes, &state); err != nil {
		return nil, err
	}

	// 统计资源类型
	resourceMap := make(map[string]int)
	for _, resource := range state.Resources {
		resourceMap[resource.Type] += len(resource.Instances)
	}

	// 转换为ResourceSummary列表
	var resources []ResourceSummary
	for resourceType, count := range resourceMap {
		resources = append(resources, ResourceSummary{
			Type:  resourceType,
			Count: count,
		})
	}

	return resources, nil
}

// UpdateResourceCount 更新资源数量
func (s *WorkspaceOverviewService) UpdateResourceCount(workspaceID string) error {
	// 先查询workspace获取内部ID
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return err
	}

	// 获取资源摘要 (使用内部数字ID)
	resources, err := s.getResourceSummary(workspace.WorkspaceID)
	if err != nil {
		return err
	}

	// 计算总数
	totalCount := 0
	for _, resource := range resources {
		totalCount += resource.Count
	}

	// 更新Workspace
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("resource_count", totalCount).Error
}

// UpdateLastPlanAt 更新最后Plan时间
func (s *WorkspaceOverviewService) UpdateLastPlanAt(workspaceID string) error {
	now := time.Now()
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("last_plan_at", now).Error
}

// UpdateLastApplyAt 更新最后Apply时间
func (s *WorkspaceOverviewService) UpdateLastApplyAt(workspaceID string) error {
	now := time.Now()
	return s.db.Model(&models.Workspace{}).
		Where("workspace_id = ?", workspaceID).
		Update("last_apply_at", now).Error
}
