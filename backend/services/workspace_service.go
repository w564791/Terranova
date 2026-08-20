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

// SearchWorkspaces 搜索工作空间（无 org 过滤；兼容旧调用，多租户请用 SearchWorkspacesInOrg）
// projectID: 0 表示不过滤项目，>0 表示过滤指定项目，-1 表示只返回未分配项目的工作空间
func (ws *WorkspaceService) SearchWorkspaces(search string, page, size int, projectID int) ([]models.Workspace, int64, error) {
	return ws.SearchWorkspacesInOrg(search, page, size, 0, projectID)
}

// SearchWorkspacesInOrg 按组织过滤工作空间列表（经 workspace_project_relations → projects.org_id）
// orgID==0 时不过滤组织（仅单测/内部兼容）；多租户 API 必须传 orgID>0。
// projectID: 0=组织内全部，>0=指定项目（须属于该 org），-1=组织内未分配（多租户下通常为空）
func (ws *WorkspaceService) SearchWorkspacesInOrg(search string, page, size int, orgID uint, projectID int) ([]models.Workspace, int64, error) {
	return ws.SearchWorkspacesInOrgAndWorkspaceIDs(search, page, size, orgID, projectID, nil)
}

// SearchWorkspacesInOrgAndWorkspaceIDs is the tenant-safe workspace query used
// by the HTTP list endpoint. allowedWorkspaceIDs has deliberate tri-state
// semantics: nil means the caller holds an organization-wide grant; a non-nil
// empty slice means no workspace may be returned; a populated slice is an IAM
// allow-list of semantic workspace IDs. Keeping this distinction in the SQL
// layer prevents a scoped Role from accidentally falling back to all workspaces.
func (ws *WorkspaceService) SearchWorkspacesInOrgAndWorkspaceIDs(search string, page, size int, orgID uint, projectID int, allowedWorkspaceIDs []string) ([]models.Workspace, int64, error) {
	var workspaces []models.Workspace
	var total int64

	query := ws.db.Model(&models.Workspace{})

	if orgID > 0 {
		// 仅返回已绑定到该组织项目下的 workspace
		query = query.Where(`workspace_id IN (
			SELECT wpr.workspace_id FROM workspace_project_relations wpr
			JOIN projects p ON p.id = wpr.project_id
			WHERE p.org_id = ?
		)`, orgID)
	}

	if allowedWorkspaceIDs != nil {
		if len(allowedWorkspaceIDs) == 0 {
			return []models.Workspace{}, 0, nil
		}
		query = query.Where("workspace_id IN ?", allowedWorkspaceIDs)
	}

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
		if orgID > 0 {
			// 项目必须属于鉴权 org
			query = query.Where(`workspace_id IN (
				SELECT wpr.workspace_id FROM workspace_project_relations wpr
				JOIN projects p ON p.id = wpr.project_id
				WHERE wpr.project_id = ? AND p.org_id = ?
			)`, projectID, orgID)
		} else {
			query = query.Where("workspace_id IN (SELECT workspace_id FROM workspace_project_relations WHERE project_id = ?)", projectID)
		}
	} else if projectID == -1 {
		// 未分配项目：多租户下不可见（无 org 绑定）；仅 orgID==0 时返回全局未分配
		if orgID > 0 {
			// 组织作用域内不暴露未绑定 workspace
			query = query.Where("1 = 0")
		} else {
			query = query.Where("workspace_id NOT IN (SELECT workspace_id FROM workspace_project_relations)")
		}
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

// ResolveWorkspaceOrgID 经 project 关系解析 workspace 所属 org；无绑定返回 0
func (ws *WorkspaceService) ResolveWorkspaceOrgID(workspaceID string) (uint, error) {
	wsObj, err := ws.GetWorkspaceByID(workspaceID)
	if err != nil {
		return 0, err
	}
	var orgID uint
	err = ws.db.Raw(`
SELECT p.org_id FROM workspace_project_relations wpr
JOIN projects p ON p.id = wpr.project_id
WHERE wpr.workspace_id = ? LIMIT 1`, wsObj.WorkspaceID).Scan(&orgID).Error
	if err != nil {
		return 0, err
	}
	return orgID, nil
}

// EnsureWorkspaceInOrg 校验 workspace 属于 org；否则返回 gorm.ErrRecordNotFound（对外 404）
func (ws *WorkspaceService) EnsureWorkspaceInOrg(workspaceID string, orgID uint) error {
	if orgID == 0 {
		return fmt.Errorf("org_id required")
	}
	resolved, err := ws.ResolveWorkspaceOrgID(workspaceID)
	if err != nil {
		return err
	}
	if resolved == 0 || resolved != orgID {
		return gorm.ErrRecordNotFound
	}
	return nil
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
	return ws.CreateWorkspaceInOrg(workspace, 0, 0)
}

// CreateWorkspaceInOrg 创建 workspace 并绑定到组织项目（同一事务）。
// orgID>0 时必须绑定 project；projectID==0 时使用该组织 default 项目（必要时创建）。
func (ws *WorkspaceService) CreateWorkspaceInOrg(workspace *models.Workspace, orgID uint, projectID uint) error {
	return ws.db.Transaction(func(tx *gorm.DB) error {
		// 名称唯一：有 org 时在 org 内唯一；否则全局（兼容）
		var existingCount int64
		nameQ := tx.Model(&models.Workspace{}).Where("name = ?", workspace.Name)
		if orgID > 0 {
			nameQ = nameQ.Where(`workspace_id IN (
				SELECT wpr.workspace_id FROM workspace_project_relations wpr
				JOIN projects p ON p.id = wpr.project_id WHERE p.org_id = ?
			)`, orgID)
		}
		if err := nameQ.Count(&existingCount).Error; err != nil {
			return fmt.Errorf("failed to check workspace name: %w", err)
		}
		if existingCount > 0 {
			return fmt.Errorf("workspace name '%s' already exists", workspace.Name)
		}

		workspaceID, err := infrastructure.GenerateWorkspaceID()
		if err != nil {
			return fmt.Errorf("failed to generate workspace ID: %w", err)
		}
		workspace.WorkspaceID = workspaceID

		if err := tx.Create(workspace).Error; err != nil {
			return err
		}

		if orgID == 0 {
			return nil
		}

		// 解析/创建目标项目
		bindProjectID := projectID
		if bindProjectID > 0 {
			var pOrg uint
			if err := tx.Table("projects").Select("org_id").Where("id = ?", bindProjectID).Scan(&pOrg).Error; err != nil || pOrg == 0 {
				return fmt.Errorf("project %d not found", bindProjectID)
			}
			if pOrg != orgID {
				return fmt.Errorf("project %d does not belong to organization %d", bindProjectID, orgID)
			}
		} else {
			// default 项目
			var defID uint
			err := tx.Table("projects").Select("id").
				Where("org_id = ? AND is_default = ?", orgID, true).
				Limit(1).Scan(&defID).Error
			if err != nil || defID == 0 {
				// 创建 default 项目
				now := time.Now()
				row := map[string]interface{}{
					"org_id":      orgID,
					"name":        "default",
					"description": "Default project",
					"is_default":  true,
					"created_at":  now,
					"updated_at":  now,
				}
				if err := tx.Table("projects").Create(row).Error; err != nil {
					return fmt.Errorf("failed to create default project: %w", err)
				}
				if err := tx.Table("projects").Select("id").
					Where("org_id = ? AND is_default = ?", orgID, true).
					Limit(1).Scan(&defID).Error; err != nil || defID == 0 {
					return fmt.Errorf("failed to resolve default project for org %d", orgID)
				}
			}
			bindProjectID = defID
		}

		// 一对一绑定（唯一约束由迁移保证；此处先删后插兼容旧数据）
		if err := tx.Exec(`DELETE FROM workspace_project_relations WHERE workspace_id = ?`, workspace.WorkspaceID).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			`INSERT INTO workspace_project_relations (workspace_id, project_id, created_at) VALUES (?, ?, ?)`,
			workspace.WorkspaceID, bindProjectID, time.Now(),
		).Error; err != nil {
			return fmt.Errorf("failed to bind workspace to project: %w", err)
		}
		return nil
	})
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
	LockID                 *string               `json:"lock_id"`
	LockInfo               models.JSONB          `json:"lock_info"`
	ProviderConfig         models.JSONB          `json:"provider_config"`
	ProviderInstances      models.JSONB          `json:"provider_instances"`
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
	// Manifest 软链接 (PR1.5 后端字段)
	ManifestDeploymentID *string `json:"manifest_deployment_id,omitempty"`
	ManifestActiveTag    *string `json:"manifest_active_tag,omitempty"`
	ManifestSubpath      *string `json:"manifest_subpath,omitempty"`
}

// WorkspaceWithStatus 包含状态信息的工作空间（不包含tf_state等大字段）
type WorkspaceWithStatus struct {
	WorkspaceListItem
	LatestRunStatus    string     `json:"latest_run_status,omitempty"`
	LatestRunID        uint       `json:"latest_run_id,omitempty"`
	LatestRunTaskType  string     `json:"latest_run_task_type,omitempty"`
	LatestRunCreatedAt *time.Time `json:"latest_run_created_at,omitempty"`
	LatestApplyTime    *time.Time `json:"latest_apply_time,omitempty"`
}

// toWorkspaceListItem 将Workspace转换为WorkspaceListItem（排除tf_state等大字段）
func toWorkspaceListItem(w models.Workspace) WorkspaceListItem {
	return WorkspaceListItem{
		ID:               w.ID,
		WorkspaceID:      w.WorkspaceID,
		Name:             w.Name,
		Description:      w.Description,
		CreatedBy:        w.CreatedBy,
		CreatedAt:        w.CreatedAt,
		UpdatedAt:        w.UpdatedAt,
		ExecutionMode:    w.ExecutionMode,
		AgentID:          w.AgentID,
		AutoApply:        w.AutoApply,
		PlanOnly:         w.PlanOnly,
		TerraformVersion: w.TerraformVersion,
		Workdir:          w.Workdir,
		StateBackend:     w.StateBackend,
		// 列表接口数据最小化：不返回可能含密钥的原始配置（详情接口另取）
		StateConfig:            nil,
		LockID:                 w.LockID,
		LockInfo:               nil,
		ProviderConfig:         nil,
		ProviderInstances:      nil,
		InitConfig:             nil,
		RetryEnabled:           w.RetryEnabled,
		MaxRetries:             w.MaxRetries,
		NotifySettings:         nil,
		LogConfig:              nil,
		State:                  w.State,
		Tags:                   w.Tags,
		SystemVariables:        nil,
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
		ManifestDeploymentID:   w.ManifestDeploymentID,
		ManifestActiveTag:      w.ManifestActiveTag,
		ManifestSubpath:        w.ManifestSubpath,
	}
}

// SearchWorkspacesWithStatus 搜索工作空间并包含最新任务状态（无 org；兼容）
func (ws *WorkspaceService) SearchWorkspacesWithStatus(search string, page, size int, projectID int) ([]WorkspaceWithStatus, int64, error) {
	return ws.SearchWorkspacesWithStatusInOrg(search, page, size, 0, projectID)
}

// SearchWorkspacesWithStatusInOrg 组织范围内列表 + 最新任务状态
func (ws *WorkspaceService) SearchWorkspacesWithStatusInOrg(search string, page, size int, orgID uint, projectID int) ([]WorkspaceWithStatus, int64, error) {
	return ws.SearchWorkspacesWithStatusInOrgAndWorkspaceIDs(search, page, size, orgID, projectID, nil)
}

// SearchWorkspacesWithStatusInOrgAndWorkspaceIDs applies the same explicit IAM
// allow-list as SearchWorkspacesInOrgAndWorkspaceIDs before calculating the
// total, pagination, and latest task metadata.
func (ws *WorkspaceService) SearchWorkspacesWithStatusInOrgAndWorkspaceIDs(search string, page, size int, orgID uint, projectID int, allowedWorkspaceIDs []string) ([]WorkspaceWithStatus, int64, error) {
	workspaces, total, err := ws.SearchWorkspacesInOrgAndWorkspaceIDs(search, page, size, orgID, projectID, allowedWorkspaceIDs)
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
		CreatedAt   time.Time  `gorm:"column:created_at"`
	}

	var latestTasks []LatestTaskInfo

	// 使用 DISTINCT ON 获取每个工作空间的最新任务（PostgreSQL 特性）
	// 排除 drift_check 任务，因为它是后台静默运行的，不应影响 workspace 状态显示
	// 优先级：1. Needs Attention(会卡住后续任务) 2. 按最近创建时间
	subQuery := `
		SELECT DISTINCT ON (workspace_id)
			workspace_id, id, status, task_type, completed_at, created_at
		FROM workspace_tasks
		WHERE workspace_id IN (?)
			AND task_type != 'drift_check'
		ORDER BY workspace_id,
			CASE
				WHEN status IN ('apply_pending', 'decision_required') THEN 0
				ELSE 1
			END,
			created_at DESC
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
			result[i].LatestRunTaskType = task.TaskType
			result[i].LatestRunCreatedAt = &task.CreatedAt
		}
		if applyTime, ok := applyTimeMap[w.WorkspaceID]; ok {
			result[i].LatestApplyTime = applyTime
		}
	}

	return result, total, nil
}
