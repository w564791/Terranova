package services

import (
	"fmt"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"
	"log"
	"time"

	"gorm.io/gorm"
)

type WorkspaceService struct {
	db *gorm.DB
}

func NewWorkspaceService(db *gorm.DB) *WorkspaceService {
	return &WorkspaceService{db: db}
}

func (ws *WorkspaceService) GetDB() *gorm.DB {
	return ws.db
}

func (ws *WorkspaceService) GetWorkspaces(page, size int) ([]models.Workspace, int64, error) {
	return ws.SearchWorkspaces("", page, size, 0)
}

// SearchWorkspaces 搜索工作空间
// projectID: 0 表示不过滤项目，>0 表示过滤指定项目，-1 表示只返回未分配项目的工作空间
func (ws *WorkspaceService) SearchWorkspaces(search string, page, size int, projectID int) ([]models.Workspace, int64, error) {
	var workspaces []models.Workspace
	var total int64

	query := ws.db.Model(&models.Workspace{})

	// 如果有搜索词，添加搜索条件
	if search != "" {
		searchPattern := "%" + search + "%"
		query = query.Where(
			"name ILIKE ? OR description ILIKE ? OR tags::text ILIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	// 如果指定了项目ID，添加项目过滤条件
	if projectID > 0 {
		// 查询属于指定项目的工作空间
		query = query.Where("workspace_id IN (SELECT workspace_id FROM workspace_project_relations WHERE project_id = ?)", projectID)
	} else if projectID == -1 {
		// 查询未分配项目的工作空间（归入 default）
		query = query.Where("workspace_id NOT IN (SELECT workspace_id FROM workspace_project_relations)")
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}

	offset := (page - 1) * size
	if err := query.Offset(offset).Limit(size).Order("updated_at DESC").Find(&workspaces).Error; err != nil {
		return nil, 0, err
	}

	return workspaces, total, nil
}

func (ws *WorkspaceService) GetWorkspaceByID(workspaceID string) (*models.Workspace, error) {
	var workspace models.Workspace

	// 判断是数字ID还是语义化ID
	var numID uint
	if _, err := fmt.Sscanf(workspaceID, "%d", &numID); err == nil && numID > 0 {
		// 是数字，直接用id字段查询
		if err := ws.db.Where("id = ?", numID).First(&workspace).Error; err != nil {
			return nil, err
		}
		return &workspace, nil
	}

	// 不是数字，作为语义化 ID 查询
	if err := ws.db.Where("workspace_id = ?", workspaceID).First(&workspace).Error; err != nil {
		return nil, err
	}
	return &workspace, nil
}

func (ws *WorkspaceService) CreateWorkspace(workspace *models.Workspace) error {
	// 检查名称是否已存在
	var existingCount int64
	if err := ws.db.Model(&models.Workspace{}).Where("name = ?", workspace.Name).Count(&existingCount).Error; err != nil {
		return fmt.Errorf("failed to check workspace name: %w", err)
	}
	if existingCount > 0 {
		return fmt.Errorf("workspace name '%s' already exists", workspace.Name)
	}

	// 生成workspace_id
	workspaceID, err := infrastructure.GenerateWorkspaceID()
	if err != nil {
		return fmt.Errorf("failed to generate workspace ID: %w", err)
	}
	workspace.WorkspaceID = workspaceID

	return ws.db.Create(workspace).Error
}

func (ws *WorkspaceService) UpdateWorkspace(workspaceID string, description, terraformVersion, executionMode string) error {
	updates := map[string]interface{}{}
	if description != "" {
		updates["description"] = description
	}
	if terraformVersion != "" {
		updates["terraform_version"] = terraformVersion
	}
	if executionMode != "" {
		updates["execution_mode"] = executionMode
	}

	return ws.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(updates).Error
}

func (ws *WorkspaceService) UpdateWorkspaceFields(workspaceID string, updates map[string]interface{}) error {
	// 添加日志
	log.Printf("UpdateWorkspaceFields: workspace_id=%s, updates=%+v", workspaceID, updates)

	// 如果更新包含name字段，检查名称是否已被其他workspace使用
	if newName, ok := updates["name"]; ok {
		if nameStr, ok := newName.(string); ok && nameStr != "" {
			var existingCount int64
			if err := ws.db.Model(&models.Workspace{}).
				Where("name = ? AND workspace_id != ?", nameStr, workspaceID).
				Count(&existingCount).Error; err != nil {
				return fmt.Errorf("failed to check workspace name: %w", err)
			}
			if existingCount > 0 {
				return fmt.Errorf("workspace name '%s' already exists", nameStr)
			}
		}
	}

	// 对于JSONB字段，需要使用Update而不是Updates
	// 或者使用Save方法
	result := ws.db.Model(&models.Workspace{}).Where("workspace_id = ?", workspaceID).Updates(updates)

	if result.Error != nil {
		log.Printf("UpdateWorkspaceFields failed: %v", result.Error)
		return result.Error
	}

	log.Printf("UpdateWorkspaceFields success: %d rows affected", result.RowsAffected)
	return nil
}

func (ws *WorkspaceService) DeleteWorkspace(workspaceID string) error {
	return ws.db.Where("workspace_id = ?", workspaceID).Delete(&models.Workspace{}).Error
}

// WorkspaceListItem 工作空间列表项（不包含tf_state等大字段）
type WorkspaceListItem struct {
	ID                     uint                  `json:"id"`
	WorkspaceID            string                `json:"workspace_id"`
	Name                   string                `json:"name"`
	Description            string                `json:"description"`
	CreatedBy              *string               `json:"created_by"`
	CreatedAt              time.Time             `json:"created_at"`
	UpdatedAt              time.Time             `json:"updated_at"`
	ExecutionMode          models.ExecutionMode  `json:"execution_mode"`
	AgentID                *uint                 `json:"agent_id"`
	AutoApply              bool                  `json:"auto_apply"`
	PlanOnly               bool                  `json:"plan_only"`
	TerraformVersion       string                `json:"terraform_version"`
	Workdir                string                `json:"workdir"`
	StateBackend           string                `json:"state_backend"`
	StateConfig            models.JSONB          `json:"state_config"`
	IsLocked               bool                  `json:"is_locked"`
	LockedBy               *string               `json:"locked_by"`
	LockedAt               *time.Time            `json:"locked_at"`
	LockReason             string                `json:"lock_reason"`
	ProviderConfig         models.JSONB          `json:"provider_config"`
	InitConfig             models.JSONB          `json:"init_config"`
	RetryEnabled           bool                  `json:"retry_enabled"`
	MaxRetries             int                   `json:"max_retries"`
	NotifySettings         models.JSONB          `json:"notify_settings"`
	LogConfig              models.JSONB          `json:"log_config"`
	State                  models.WorkspaceState `json:"state"`
	Tags                   models.JSONB          `json:"tags"`
	SystemVariables        models.JSONB          `json:"system_variables"`
	ResourceCount          int                   `json:"resource_count"`
	LastPlanAt             *time.Time            `json:"last_plan_at"`
	LastApplyAt            *time.Time            `json:"last_apply_at"`
	DriftCount             int                   `json:"drift_count"`
	LastDriftCheck         *time.Time            `json:"last_drift_check"`
	UIMode                 string                `json:"ui_mode"`
	ShowUnchangedResources bool                  `json:"show_unchanged_resources"`
	OutputsSharing         string                `json:"outputs_sharing"`
	AgentPoolID            *uint                 `json:"agent_pool_id"`
	CurrentPoolID          *string               `json:"current_pool_id"`
	K8sConfigID            *uint                 `json:"k8s_config_id"`
}

// WorkspaceWithStatus 包含状态信息的工作空间（不包含tf_state等大字段）
type WorkspaceWithStatus struct {
	WorkspaceListItem
	LatestRunStatus string     `json:"latest_run_status,omitempty"`
	LatestRunID     uint       `json:"latest_run_id,omitempty"`
	LatestApplyTime *time.Time `json:"latest_apply_time,omitempty"`
}

// toWorkspaceListItem 将Workspace转换为WorkspaceListItem（排除tf_state等大字段）
func toWorkspaceListItem(w models.Workspace) WorkspaceListItem {
	return WorkspaceListItem{
		ID:                     w.ID,
		WorkspaceID:            w.WorkspaceID,
		Name:                   w.Name,
		Description:            w.Description,
		CreatedBy:              w.CreatedBy,
		CreatedAt:              w.CreatedAt,
		UpdatedAt:              w.UpdatedAt,
		ExecutionMode:          w.ExecutionMode,
		AgentID:                w.AgentID,
		AutoApply:              w.AutoApply,
		PlanOnly:               w.PlanOnly,
		TerraformVersion:       w.TerraformVersion,
		Workdir:                w.Workdir,
		StateBackend:           w.StateBackend,
		StateConfig:            w.StateConfig,
		IsLocked:               w.IsLocked,
		LockedBy:               w.LockedBy,
		LockedAt:               w.LockedAt,
		LockReason:             w.LockReason,
		ProviderConfig:         w.ProviderConfig,
		InitConfig:             w.InitConfig,
		RetryEnabled:           w.RetryEnabled,
		MaxRetries:             w.MaxRetries,
		NotifySettings:         w.NotifySettings,
		LogConfig:              w.LogConfig,
		State:                  w.State,
		Tags:                   w.Tags,
		SystemVariables:        w.SystemVariables,
		ResourceCount:          w.ResourceCount,
		LastPlanAt:             w.LastPlanAt,
		LastApplyAt:            w.LastApplyAt,
		DriftCount:             w.DriftCount,
		LastDriftCheck:         w.LastDriftCheck,
		UIMode:                 w.UIMode,
		ShowUnchangedResources: w.ShowUnchangedResources,
		OutputsSharing:         w.OutputsSharing,
		AgentPoolID:            w.AgentPoolID,
		CurrentPoolID:          w.CurrentPoolID,
		K8sConfigID:            w.K8sConfigID,
	}
}

// SearchWorkspacesWithStatus 搜索工作空间并包含最新任务状态
func (ws *WorkspaceService) SearchWorkspacesWithStatus(search string, page, size int, projectID int) ([]WorkspaceWithStatus, int64, error) {
	workspaces, total, err := ws.SearchWorkspaces(search, page, size, projectID)
	if err != nil {
		return nil, 0, err
	}

	if len(workspaces) == 0 {
		return []WorkspaceWithStatus{}, total, nil
	}

	// 收集所有 workspace_id
	workspaceIDs := make([]string, len(workspaces))
	for i, w := range workspaces {
		workspaceIDs[i] = w.WorkspaceID
	}

	// 批量查询每个工作空间的最新任务状态
	// 使用子查询获取每个工作空间的最新任务
	type LatestTaskInfo struct {
		WorkspaceID string     `gorm:"column:workspace_id"`
		TaskID      uint       `gorm:"column:id"`
		Status      string     `gorm:"column:status"`
		TaskType    string     `gorm:"column:task_type"`
		CompletedAt *time.Time `gorm:"column:completed_at"`
	}

	var latestTasks []LatestTaskInfo

	// 使用 DISTINCT ON 获取每个工作空间的最新任务（PostgreSQL 特性）
	// 排除 drift_check 任务，因为它是后台静默运行的，不应影响 workspace 状态显示
	subQuery := `
		SELECT DISTINCT ON (workspace_id) 
			workspace_id, id, status, task_type, completed_at
		FROM workspace_tasks 
		WHERE workspace_id IN (?)
			AND task_type != 'drift_check'
		ORDER BY workspace_id, created_at DESC
	`
	ws.db.Raw(subQuery, workspaceIDs).Scan(&latestTasks)

	// 查询每个工作空间最近的 apply 任务时间
	type ApplyTimeInfo struct {
		WorkspaceID string     `gorm:"column:workspace_id"`
		CompletedAt *time.Time `gorm:"column:completed_at"`
	}
	var applyTimes []ApplyTimeInfo

	applySubQuery := `
		SELECT DISTINCT ON (workspace_id) 
			workspace_id, completed_at
		FROM workspace_tasks 
		WHERE workspace_id IN (?) 
			AND (task_type = 'apply' OR task_type = 'plan_and_apply')
			AND (status = 'applied' OR status = 'failed')
		ORDER BY workspace_id, completed_at DESC
	`
	ws.db.Raw(applySubQuery, workspaceIDs).Scan(&applyTimes)

	// 构建映射
	taskMap := make(map[string]LatestTaskInfo)
	for _, t := range latestTasks {
		taskMap[t.WorkspaceID] = t
	}

	applyTimeMap := make(map[string]*time.Time)
	for _, a := range applyTimes {
		applyTimeMap[a.WorkspaceID] = a.CompletedAt
	}

	// 组装结果
	result := make([]WorkspaceWithStatus, len(workspaces))
	for i, w := range workspaces {
		result[i] = WorkspaceWithStatus{
			WorkspaceListItem: toWorkspaceListItem(w),
		}
		if task, ok := taskMap[w.WorkspaceID]; ok {
			result[i].LatestRunStatus = task.Status
			result[i].LatestRunID = task.TaskID
		}
		if applyTime, ok := applyTimeMap[w.WorkspaceID]; ok {
			result[i].LatestApplyTime = applyTime
		}
	}

	return result, total, nil
}
