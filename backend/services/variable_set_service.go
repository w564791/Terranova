package services

import (
	"fmt"
	"iac-platform/internal/models"
	"time"

	"gorm.io/gorm"
)

// VariableSetService 变量集服务
type VariableSetService struct {
	db *gorm.DB
}

// NewVariableSetService 创建变量集服务实例
func NewVariableSetService(db *gorm.DB) *VariableSetService {
	return &VariableSetService{db: db}
}

// Create 创建变量集.
// specific: orgID required (from auth org; body org_id ignored by caller).
// global: orgID must be nil.
func (s *VariableSetService) Create(name, description, scope string, createdBy *string, orgID *uint) (*models.VariableSet, error) {
	if scope != "global" && scope != "specific" {
		return nil, fmt.Errorf("invalid scope: %s, must be 'global' or 'specific'", scope)
	}
	if scope == "specific" {
		if orgID == nil || *orgID == 0 {
			return nil, fmt.Errorf("org_id required for specific variable set")
		}
	} else {
		orgID = nil
	}

	if err := s.ensureNameUnique(name, orgID, ""); err != nil {
		return nil, err
	}

	varset := models.VariableSet{
		Name:        name,
		Description: description,
		Scope:       scope,
		OrgID:       orgID,
		CreatedBy:   createdBy,
	}

	if err := s.db.Create(&varset).Error; err != nil {
		return nil, fmt.Errorf("failed to create variable set: %w", err)
	}

	return &varset, nil
}

// ensureNameUnique enforces per-org name uniqueness for specific varsets and
// a separate global/orphan namespace (org_id IS NULL).
func (s *VariableSetService) ensureNameUnique(name string, orgID *uint, excludeVarsetID string) error {
	q := s.db.Where("name = ? AND is_deleted = ?", name, false)
	if orgID == nil {
		q = q.Where("org_id IS NULL")
	} else {
		q = q.Where("org_id = ?", *orgID)
	}
	if excludeVarsetID != "" {
		q = q.Where("varset_id != ?", excludeVarsetID)
	}
	var existing models.VariableSet
	err := q.First(&existing).Error
	if err == nil {
		return fmt.Errorf("variable set with name '%s' already exists", name)
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check name uniqueness: %w", err)
	}
	return nil
}

// List 获取变量集列表
func (s *VariableSetService) List(scope string) ([]models.VariableSet, error) {
	query := s.db.Where("is_deleted = ?", false)

	if scope != "" {
		query = query.Where("scope = ?", scope)
	}

	var varsets []models.VariableSet
	if err := query.Order("created_at DESC").Find(&varsets).Error; err != nil {
		return nil, fmt.Errorf("failed to list variable sets: %w", err)
	}

	return varsets, nil
}

// ListForOrg 返回调用组织可见的变量集(GET /variable-sets 无 workspace_id 时)。
// 优先按 org_id 列;仅 org_id IS NULL 的兼容行才回退到分配/创建者推导
// (见 visibleInOrgCond)。
func (s *VariableSetService) ListForOrg(scope string, orgID uint, callerID string) ([]models.VariableSet, error) {
	if orgID == 0 {
		return nil, fmt.Errorf("org_id required")
	}
	query := s.db.Where("is_deleted = ?", false).Where(s.visibleInOrgCond(orgID, callerID))
	if scope != "" {
		query = query.Where("scope = ?", scope)
	}
	var varsets []models.VariableSet
	if err := query.Order("created_at DESC").Find(&varsets).Error; err != nil {
		return nil, fmt.Errorf("failed to list variable sets: %w", err)
	}
	return varsets, nil
}

// orgAssignmentCond 匹配"落在 orgID 内"的分配行:分配到本组织 workspace
// (workspace -> workspace_project_relations -> projects.org_id)或本组织 project。
// ListForOrg / VarsetVisibleInOrg / VarsetWritableInOrg 共用这一条分配查询。
func (s *VariableSetService) orgAssignmentCond(orgID uint) *gorm.DB {
	orgProjects := s.db.Table("projects").Select("id").Where("org_id = ?", orgID)
	orgWorkspaces := s.db.Table("workspace_project_relations AS wpr").Select("wpr.workspace_id").
		Joins("JOIN projects AS p ON p.id = wpr.project_id").Where("p.org_id = ?", orgID)
	return s.db.Where("scope_type = ? AND workspace_id IN (?)", "workspace", orgWorkspaces).
		Or("scope_type = ? AND project_id IN (?)", "project", orgProjects)
}

// visibleInOrgCond 变量集在 orgID 内可见的条件。
// Prefer org_id column; assignment/creator joins only for org_id IS NULL
// compat/orphan rows. New rows with org_id set never infer org via join.
func (s *VariableSetService) visibleInOrgCond(orgID uint, callerID string) *gorm.DB {
	assigned := s.db.Model(&models.VarsetAssignment{}).Select("varset_id").Where(s.orgAssignmentCond(orgID))
	anyAssignment := s.db.Model(&models.VarsetAssignment{}).Select("varset_id")
	compatParts := s.db.Where("varset_id IN (?)", assigned)
	if callerID != "" {
		compatParts = compatParts.Or(
			"scope <> ? AND created_by = ? AND varset_id NOT IN (?)",
			"global", callerID, anyAssignment,
		)
	}
	compat := s.db.Where("org_id IS NULL").Where(compatParts)
	return s.db.Where("scope = ?", "global").
		Or("org_id = ?", orgID).
		Or(compat)
}

// VarsetVisibleInOrg 单个变量集是否对 orgID 内的 callerID 可见(by-ID 路由的读校验)。
// 规则与 ListForOrg 完全一致。已删除或不存在 => false。
func (s *VariableSetService) VarsetVisibleInOrg(varsetID string, orgID uint, callerID string) (bool, error) {
	if orgID == 0 || varsetID == "" {
		return false, nil
	}
	var n int64
	if err := s.db.Model(&models.VariableSet{}).
		Where("varset_id = ? AND is_deleted = ?", varsetID, false).
		Where(s.visibleInOrgCond(orgID, callerID)).
		Count(&n).Error; err != nil {
		return false, fmt.Errorf("check variable set visibility: %w", err)
	}
	return n > 0, nil
}

// VarsetWritableInOrg 单个变量集是否可被 orgID 内的 callerID 修改(by-ID 写路由)。
// 调用前应先确认可见(不可见由调用方返回 404)。
//   - global → 仅平台超管;
//   - org_id = orgID → 可写;
//   - specific + org_id IS NULL (legacy orphan) → 仅创建者可写,且本次写入须
//     把 org_id 设为调用者 auth org(heal-on-write, orphan=true);其他人 404。
func (s *VariableSetService) VarsetWritableInOrg(varsetID string, orgID uint, callerID string, isSystemAdmin bool) (writable bool, orphan bool, err error) {
	if orgID == 0 || varsetID == "" {
		return false, false, nil
	}
	var vs models.VariableSet
	if err := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false).First(&vs).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return false, false, nil
		}
		return false, false, fmt.Errorf("load variable set: %w", err)
	}
	if vs.Scope == "global" {
		return isSystemAdmin, false, nil
	}
	if vs.OrgID != nil {
		return *vs.OrgID == orgID, false, nil
	}
	return callerID != "" && vs.CreatedBy != nil && *vs.CreatedBy == callerID, true, nil
}

// HealOrphan binds a legacy orphan (specific, org_id NULL) to orgID in one
// conditional UPDATE, only when callerID is its creator. Used by write paths
// that do not otherwise update the variable_sets row (variables, unmount).
// No-op when the row already has an org_id.
func (s *VariableSetService) HealOrphan(varsetID string, orgID uint, callerID string) error {
	if orgID == 0 || callerID == "" {
		return fmt.Errorf("variable set not found")
	}
	var vs models.VariableSet
	if err := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false).First(&vs).Error; err != nil {
		return fmt.Errorf("variable set not found")
	}
	if vs.OrgID != nil {
		return nil
	}
	if err := s.ensureNameUnique(vs.Name, &orgID, varsetID); err != nil {
		return err
	}
	res := s.db.Model(&models.VariableSet{}).
		Where("varset_id = ? AND scope = ? AND org_id IS NULL AND created_by = ?", varsetID, "specific", callerID).
		Updates(map[string]interface{}{"org_id": orgID, "updated_at": time.Now()})
	if res.Error != nil {
		return fmt.Errorf("heal variable set org_id: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("variable set not found")
	}
	return nil
}

// GetAssignment 读取属于 varsetID 的分配(删除分配前校验其目标)。
func (s *VariableSetService) GetAssignment(varsetID string, assignmentID uint) (*models.VarsetAssignment, error) {
	var a models.VarsetAssignment
	if err := s.db.Where("id = ? AND varset_id = ?", assignmentID, varsetID).First(&a).Error; err != nil {
		return nil, fmt.Errorf("assignment not found: %w", err)
	}
	return &a, nil
}

// ListMountableForWorkspace 返回指定 workspace 可挂载的变量集(manifest 部署选择 varset 用)。
//
// "可挂载" 沿用变量解析链(VariableResolutionService)的作用域规则:
//   - scope = global 的变量集;
//   - 分配到该 workspace 的变量集(varset_assignments.scope_type = workspace);
//   - 分配到该 workspace 所属项目的变量集(varset_assignments.scope_type = project)。
//
// 未分配给该 workspace / 项目的 specific 变量集不可挂载。
func (s *VariableSetService) ListMountableForWorkspace(workspaceID string) ([]models.VariableSet, error) {
	var projectIDs []int
	if err := s.db.Table("workspace_project_relations").
		Where("workspace_id = ?", workspaceID).
		Pluck("project_id", &projectIDs).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve workspace project: %w", err)
	}

	assigned := s.db.Model(&models.VarsetAssignment{}).Select("varset_id").
		Where("scope_type = ? AND workspace_id = ?", "workspace", workspaceID)
	if len(projectIDs) > 0 {
		assigned = assigned.Or("scope_type = ? AND project_id IN ?", "project", projectIDs)
	}

	var varsets []models.VariableSet
	if err := s.db.Where("is_deleted = ?", false).
		Where(s.db.Where("scope = ?", "global").Or("varset_id IN (?)", assigned)).
		Order("created_at DESC").
		Find(&varsets).Error; err != nil {
		return nil, fmt.Errorf("failed to list mountable variable sets: %w", err)
	}
	return varsets, nil
}

// UnmountableVarsetIDs 返回 varsetIDs 中不能挂载到 workspace 的那些(去重,保持入参顺序)。
func (s *VariableSetService) UnmountableVarsetIDs(workspaceID string, varsetIDs []string) ([]string, error) {
	if len(varsetIDs) == 0 {
		return nil, nil
	}
	mountable, err := s.ListMountableForWorkspace(workspaceID)
	if err != nil {
		return nil, err
	}
	ok := make(map[string]bool, len(mountable))
	for _, vs := range mountable {
		ok[vs.VarsetID] = true
	}
	var bad []string
	seen := map[string]bool{}
	for _, id := range varsetIDs {
		if !ok[id] && !seen[id] {
			bad = append(bad, id)
			seen[id] = true
		}
	}
	return bad, nil
}

// GetByVarsetID 通过 varset_id 获取变量集
func (s *VariableSetService) GetByVarsetID(varsetID string) (*models.VariableSet, error) {
	var varset models.VariableSet
	if err := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false).First(&varset).Error; err != nil {
		return nil, fmt.Errorf("variable set not found: %w", err)
	}
	return &varset, nil
}

// Update 更新变量集
func (s *VariableSetService) Update(varsetID string, name, description *string, healOrg *uint) (*models.VariableSet, error) {
	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	targetOrg := varset.OrgID
	if varset.OrgID == nil && varset.Scope == "specific" && healOrg != nil {
		// Heal orphan in the same UPDATE as the edit.
		targetOrg = healOrg
		updates["org_id"] = *healOrg
	}
	newName := varset.Name
	if name != nil {
		newName = *name
		updates["name"] = *name
	}
	if newName != varset.Name || targetOrg != varset.OrgID {
		if err := s.ensureNameUnique(newName, targetOrg, varsetID); err != nil {
			return nil, err
		}
	}

	if description != nil {
		updates["description"] = *description
	}

	if err := s.db.Model(varset).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update variable set: %w", err)
	}
	if v, ok := updates["org_id"]; ok {
		o := v.(uint)
		varset.OrgID = &o
	}

	return varset, nil
}

// UpdateScope 更新变量集 scope.
// global: clear mounts + org_id NULL. specific: set org_id from authOrgID when
// currently NULL (e.g. flipping from global).
func (s *VariableSetService) UpdateScope(varsetID, newScope string, authOrgID uint) (*models.VariableSet, error) {
	if newScope != "global" && newScope != "specific" {
		return nil, fmt.Errorf("invalid scope: %s, must be 'global' or 'specific'", newScope)
	}

	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}

	if newScope == "global" {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Where("varset_id = ?", varsetID).Delete(&models.VarsetAssignment{}).Error; err != nil {
				return fmt.Errorf("failed to delete assignments: %w", err)
			}
			if err := tx.Model(varset).Updates(map[string]interface{}{
				"scope":      newScope,
				"org_id":     nil,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("failed to update scope: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		varset.Scope = newScope
		varset.OrgID = nil
		return varset, nil
	}

	updates := map[string]interface{}{
		"scope":      newScope,
		"updated_at": time.Now(),
	}
	if varset.OrgID == nil {
		if authOrgID == 0 {
			return nil, fmt.Errorf("org_id required when switching to specific")
		}
		updates["org_id"] = authOrgID
	}
	if err := s.db.Model(varset).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update scope: %w", err)
	}
	varset.Scope = newScope
	if varset.OrgID == nil && authOrgID != 0 {
		varset.OrgID = &authOrgID
	}
	return varset, nil
}

// Delete 软删除变量集
func (s *VariableSetService) Delete(varsetID string, healOrg *uint) error {
	updates := map[string]interface{}{
		"is_deleted": true,
		"updated_at": time.Now(),
	}
	q := s.db.Model(&models.VariableSet{}).Where("varset_id = ? AND is_deleted = ?", varsetID, false)
	if healOrg != nil {
		// Orphan: bind org in the same UPDATE (deleted rows are out of the
		// unique indexes, so no name check is needed).
		var vs models.VariableSet
		if err := s.db.Where("varset_id = ? AND is_deleted = ?", varsetID, false).First(&vs).Error; err == nil &&
			vs.Scope == "specific" && vs.OrgID == nil {
			updates["org_id"] = *healOrg
		}
	}
	result := q.Updates(updates)

	if result.Error != nil {
		return fmt.Errorf("failed to delete variable set: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("variable set not found")
	}
	return nil
}

// CreateAssignment 创建变量集分配.
// Target org must equal varset.org_id (except global, which cannot be mounted).
// Compat rows with org_id IS NULL get org_id set to the target org on this mount.
func (s *VariableSetService) CreateAssignment(varsetID, scopeType string, projectID *int, workspaceID *string, attachedBy *string, targetOrgID uint) (*models.VarsetAssignment, error) {
	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}
	if varset.Scope != "specific" {
		return nil, fmt.Errorf("cannot create assignment for variable set with scope '%s', must be 'specific'", varset.Scope)
	}
	if scopeType != "project" && scopeType != "workspace" {
		return nil, fmt.Errorf("invalid scope_type: %s, must be 'project' or 'workspace'", scopeType)
	}
	if scopeType == "project" && projectID == nil {
		return nil, fmt.Errorf("project_id is required for scope_type 'project'")
	}
	if scopeType == "workspace" && workspaceID == nil {
		return nil, fmt.Errorf("workspace_id is required for scope_type 'workspace'")
	}
	if targetOrgID == 0 {
		return nil, fmt.Errorf("target org_id required")
	}
	if varset.OrgID != nil && *varset.OrgID != targetOrgID {
		return nil, fmt.Errorf("assignment target not found")
	}

	assignment := models.VarsetAssignment{
		VarsetID:    varsetID,
		ScopeType:   scopeType,
		ProjectID:   projectID,
		WorkspaceID: workspaceID,
		AttachedBy:  attachedBy,
	}

	err = s.db.Transaction(func(tx *gorm.DB) error {
		if varset.OrgID == nil {
			// First mount claims the orphan for the target org.
			if err := s.ensureNameUniqueTx(tx, varset.Name, &targetOrgID, varsetID); err != nil {
				return err
			}
			if err := tx.Model(&models.VariableSet{}).
				Where("varset_id = ? AND org_id IS NULL", varsetID).
				Updates(map[string]interface{}{"org_id": targetOrgID, "updated_at": time.Now()}).Error; err != nil {
				return fmt.Errorf("failed to bind varset org_id: %w", err)
			}
		}
		if err := tx.Create(&assignment).Error; err != nil {
			return fmt.Errorf("failed to create assignment: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &assignment, nil
}

func (s *VariableSetService) ensureNameUniqueTx(tx *gorm.DB, name string, orgID *uint, excludeVarsetID string) error {
	q := tx.Where("name = ? AND is_deleted = ?", name, false)
	if orgID == nil {
		q = q.Where("org_id IS NULL")
	} else {
		q = q.Where("org_id = ?", *orgID)
	}
	if excludeVarsetID != "" {
		q = q.Where("varset_id != ?", excludeVarsetID)
	}
	var existing models.VariableSet
	err := q.First(&existing).Error
	if err == nil {
		return fmt.Errorf("variable set with name '%s' already exists", name)
	}
	if err != gorm.ErrRecordNotFound {
		return fmt.Errorf("failed to check name uniqueness: %w", err)
	}
	return nil
}

// ListAssignments 获取变量集的分配列表
func (s *VariableSetService) ListAssignments(varsetID string) ([]models.VarsetAssignment, error) {
	var assignments []models.VarsetAssignment
	if err := s.db.Where("varset_id = ?", varsetID).
		Order("attached_at ASC").
		Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("failed to list assignments: %w", err)
	}
	return assignments, nil
}

// DeleteAssignment 删除分配（硬删除，验证所属 varset）
func (s *VariableSetService) DeleteAssignment(varsetID string, assignmentID uint) error {
	result := s.db.Where("id = ? AND varset_id = ?", assignmentID, varsetID).Delete(&models.VarsetAssignment{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete assignment: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("assignment not found")
	}
	return nil
}

// GetVariableCounts 批量获取变量数量
func (s *VariableSetService) GetVariableCounts(varsetIDs []string) (map[string]int64, error) {
	type countResult struct {
		VarsetID string
		Count    int64
	}
	counts := make(map[string]int64)
	if len(varsetIDs) == 0 {
		return counts, nil
	}
	var results []countResult
	if err := s.db.Model(&models.VarsetVariable{}).
		Select("varset_id, COUNT(*) as count").
		Where("varset_id IN ? AND is_deleted = false", varsetIDs).
		Group("varset_id").
		Find(&results).Error; err != nil {
		return nil, err
	}
	for _, r := range results {
		counts[r.VarsetID] = r.Count
	}
	return counts, nil
}

// GetAssignmentCounts 批量获取分配数量
func (s *VariableSetService) GetAssignmentCounts(varsetIDs []string) (map[string]int64, error) {
	type countResult struct {
		VarsetID string
		Count    int64
	}
	counts := make(map[string]int64)
	if len(varsetIDs) == 0 {
		return counts, nil
	}
	var results []countResult
	if err := s.db.Model(&models.VarsetAssignment{}).
		Select("varset_id, COUNT(*) as count").
		Where("varset_id IN ?", varsetIDs).
		Group("varset_id").
		Find(&results).Error; err != nil {
		return nil, err
	}
	for _, r := range results {
		counts[r.VarsetID] = r.Count
	}
	return counts, nil
}

// GetVariableCount 获取变量集中活跃变量数量
func (s *VariableSetService) GetVariableCount(varsetID string) (int64, error) {
	var count int64
	if err := s.db.Model(&models.VarsetVariable{}).
		Where("varset_id = ? AND is_deleted = ?", varsetID, false).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count variables: %w", err)
	}
	return count, nil
}

// GetAssignmentCount 获取变量集分配数量
func (s *VariableSetService) GetAssignmentCount(varsetID string) (int64, error) {
	var count int64
	if err := s.db.Model(&models.VarsetAssignment{}).
		Where("varset_id = ?", varsetID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count assignments: %w", err)
	}
	return count, nil
}
