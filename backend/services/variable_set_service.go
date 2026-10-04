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

// Create 创建变量集
func (s *VariableSetService) Create(name, description, scope string, createdBy *string) (*models.VariableSet, error) {
	// 验证 scope
	if scope != "global" && scope != "specific" {
		return nil, fmt.Errorf("invalid scope: %s, must be 'global' or 'specific'", scope)
	}

	// 检查名称唯一性（非删除）
	var existing models.VariableSet
	err := s.db.Where("name = ? AND is_deleted = ?", name, false).First(&existing).Error
	if err == nil {
		return nil, fmt.Errorf("variable set with name '%s' already exists", name)
	} else if err != gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("failed to check name uniqueness: %w", err)
	}

	varset := models.VariableSet{
		Name:        name,
		Description: description,
		Scope:       scope,
		CreatedBy:   createdBy,
	}

	if err := s.db.Create(&varset).Error; err != nil {
		return nil, fmt.Errorf("failed to create variable set: %w", err)
	}

	return &varset, nil
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
func (s *VariableSetService) Update(varsetID string, name, description *string) (*models.VariableSet, error) {
	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
	}

	// 如果更新名称，检查唯一性
	if name != nil {
		if *name != varset.Name {
			var existing models.VariableSet
			err := s.db.Where("name = ? AND is_deleted = ? AND varset_id != ?", *name, false, varsetID).First(&existing).Error
			if err == nil {
				return nil, fmt.Errorf("variable set with name '%s' already exists", *name)
			} else if err != gorm.ErrRecordNotFound {
				return nil, fmt.Errorf("failed to check name uniqueness: %w", err)
			}
		}
		updates["name"] = *name
	}

	if description != nil {
		updates["description"] = *description
	}

	if err := s.db.Model(varset).Updates(updates).Error; err != nil {
		return nil, fmt.Errorf("failed to update variable set: %w", err)
	}

	return varset, nil
}

// UpdateScope 更新变量集 scope
func (s *VariableSetService) UpdateScope(varsetID, newScope string) (*models.VariableSet, error) {
	// 验证 scope
	if newScope != "global" && newScope != "specific" {
		return nil, fmt.Errorf("invalid scope: %s, must be 'global' or 'specific'", newScope)
	}

	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}

	// 如果切换到 global，删除所有分配
	if newScope == "global" && varset.Scope != "global" {
		err := s.db.Transaction(func(tx *gorm.DB) error {
			// 删除所有分配
			if err := tx.Where("varset_id = ?", varsetID).Delete(&models.VarsetAssignment{}).Error; err != nil {
				return fmt.Errorf("failed to delete assignments: %w", err)
			}
			// 更新 scope
			if err := tx.Model(varset).Updates(map[string]interface{}{
				"scope":      newScope,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return fmt.Errorf("failed to update scope: %w", err)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		if err := s.db.Model(varset).Updates(map[string]interface{}{
			"scope":      newScope,
			"updated_at": time.Now(),
		}).Error; err != nil {
			return nil, fmt.Errorf("failed to update scope: %w", err)
		}
	}

	varset.Scope = newScope
	return varset, nil
}

// Delete 软删除变量集
func (s *VariableSetService) Delete(varsetID string) error {
	result := s.db.Model(&models.VariableSet{}).
		Where("varset_id = ? AND is_deleted = ?", varsetID, false).
		Updates(map[string]interface{}{
			"is_deleted": true,
			"updated_at": time.Now(),
		})

	if result.Error != nil {
		return fmt.Errorf("failed to delete variable set: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("variable set not found")
	}
	return nil
}

// CreateAssignment 创建变量集分配
func (s *VariableSetService) CreateAssignment(varsetID, scopeType string, projectID *int, workspaceID *string, attachedBy *string) (*models.VarsetAssignment, error) {
	// 验证变量集存在且是 specific scope
	varset, err := s.GetByVarsetID(varsetID)
	if err != nil {
		return nil, err
	}
	if varset.Scope != "specific" {
		return nil, fmt.Errorf("cannot create assignment for variable set with scope '%s', must be 'specific'", varset.Scope)
	}

	// 验证 scopeType
	if scopeType != "project" && scopeType != "workspace" {
		return nil, fmt.Errorf("invalid scope_type: %s, must be 'project' or 'workspace'", scopeType)
	}

	// 验证对应字段
	if scopeType == "project" && projectID == nil {
		return nil, fmt.Errorf("project_id is required for scope_type 'project'")
	}
	if scopeType == "workspace" && workspaceID == nil {
		return nil, fmt.Errorf("workspace_id is required for scope_type 'workspace'")
	}

	assignment := models.VarsetAssignment{
		VarsetID:    varsetID,
		ScopeType:   scopeType,
		ProjectID:   projectID,
		WorkspaceID: workspaceID,
		AttachedBy:  attachedBy,
	}

	if err := s.db.Create(&assignment).Error; err != nil {
		return nil, fmt.Errorf("failed to create assignment: %w", err)
	}

	return &assignment, nil
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
