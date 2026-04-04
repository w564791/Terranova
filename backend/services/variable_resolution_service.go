package services

import (
	"fmt"
	"iac-platform/internal/models"

	"gorm.io/gorm"
)

// EffectiveVariable 合并后的变量（Display 模式）
type EffectiveVariable struct {
	VariableID   string              `json:"variable_id"`
	Key          string              `json:"key"`
	Value        string              `json:"value"`
	Version      int                 `json:"version"`
	VariableType models.VariableType `json:"variable_type"`
	ValueFormat  models.ValueFormat  `json:"value_format"`
	Sensitive    bool                `json:"sensitive"`
	Description  string              `json:"description"`
	SourceType   string              `json:"source_type"`
	SourceID     string              `json:"source_id"`
	SourceName   string              `json:"source_name"`
	ScopeLevel   string              `json:"scope_level"`
	IsOverridden bool                `json:"is_overridden"`
	OverriddenBy *OverrideInfo       `json:"overridden_by,omitempty"`
}

// OverrideInfo 覆盖信息
type OverrideInfo struct {
	VariableID string `json:"variable_id"`
	SourceType string `json:"source_type"`
	SourceID   string `json:"source_id"`
}

// VariableResolutionService 变量解析服务
type VariableResolutionService struct {
	db *gorm.DB
}

// NewVariableResolutionService 创建变量解析服务实例
func NewVariableResolutionService(db *gorm.DB) *VariableResolutionService {
	return &VariableResolutionService{db: db}
}

// variableCandidate 内部收集用的候选变量
type variableCandidate struct {
	VariableID   string
	Key          string
	Value        string
	Version      int
	VariableType models.VariableType
	ValueFormat  models.ValueFormat
	Sensitive    bool
	Description  string
	SourceType   string // "workspace" | "varset"
	SourceID     string
	SourceName   string
	ScopeLevel   string // "global" | "project" | "workspace-specific" | "workspace"
}

// compositeKey returns the merge key for a variable candidate.
// Uses key only (not type) — same key always conflicts regardless of variable_type.
func (c *variableCandidate) compositeKey() string {
	return c.Key
}

// ResolveDisplay returns the full list of effective variables with override markers for the frontend.
func (s *VariableResolutionService) ResolveDisplay(workspaceID string) ([]EffectiveVariable, error) {
	candidates, err := s.collectAllCandidates(workspaceID)
	if err != nil {
		return nil, err
	}

	// Track all candidates per composite key, last one wins
	// Use ordered slice to preserve insertion order across keys
	type keyEntry struct {
		indices []int // indices into candidates slice
	}
	keyMap := make(map[string]*keyEntry)
	var keyOrder []string

	for i, c := range candidates {
		ck := c.compositeKey()
		if entry, ok := keyMap[ck]; ok {
			entry.indices = append(entry.indices, i)
		} else {
			keyMap[ck] = &keyEntry{indices: []int{i}}
			keyOrder = append(keyOrder, ck)
		}
	}

	// Build result: for each composite key, the last candidate wins
	var result []EffectiveVariable
	for _, ck := range keyOrder {
		entry := keyMap[ck]
		winnerIdx := entry.indices[len(entry.indices)-1]
		winner := candidates[winnerIdx]

		// Add all candidates for this key
		for _, idx := range entry.indices {
			c := candidates[idx]
			ev := EffectiveVariable{
				VariableID:   c.VariableID,
				Key:          c.Key,
				Version:      c.Version,
				VariableType: c.VariableType,
				ValueFormat:  c.ValueFormat,
				Sensitive:    c.Sensitive,
				Description:  c.Description,
				SourceType:   c.SourceType,
				SourceID:     c.SourceID,
				SourceName:   c.SourceName,
				ScopeLevel:   c.ScopeLevel,
			}

			if idx == winnerIdx {
				// Winner: show value unless sensitive
				ev.IsOverridden = false
				if c.Sensitive {
					ev.Value = ""
				} else {
					ev.Value = c.Value
				}
			} else {
				// Overridden: hide value, mark override info
				ev.IsOverridden = true
				ev.Value = ""
				ev.OverriddenBy = &OverrideInfo{
					VariableID: winner.VariableID,
					SourceType: winner.SourceType,
					SourceID:   winner.SourceID,
				}
			}

			result = append(result, ev)
		}
	}

	return result, nil
}

// ResolveExecution returns final effective variables as []WorkspaceVariable with FULL values (including sensitive).
// Used by execution path (LocalDataAccessor) — NEVER mask values here.
func (s *VariableResolutionService) ResolveExecution(workspaceID string) ([]models.WorkspaceVariable, error) {
	candidates, err := s.collectAllCandidates(workspaceID)
	if err != nil {
		return nil, err
	}

	// Last candidate per key wins
	winners := make(map[string]*variableCandidate)
	for i := range candidates {
		winners[candidates[i].Key] = &candidates[i]
	}

	var result []models.WorkspaceVariable
	for _, c := range winners {
		result = append(result, models.WorkspaceVariable{
			VariableID:   c.VariableID,
			WorkspaceID:  workspaceID,
			Key:          c.Key,
			Value:        c.Value, // Full value, never masked
			VariableType: c.VariableType,
			ValueFormat:  c.ValueFormat,
			Sensitive:    c.Sensitive,
			Description:  c.Description,
			Version:      c.Version,
		})
	}
	return result, nil
}

// ResolveFlat returns only the final effective variables as key->value map, filtered by variable type.
func (s *VariableResolutionService) ResolveFlat(workspaceID string, varType models.VariableType) (map[string]string, error) {
	candidates, err := s.collectAllCandidates(workspaceID)
	if err != nil {
		return nil, err
	}

	result := make(map[string]string)
	for _, c := range candidates {
		if c.VariableType == varType {
			// Later candidates overwrite earlier ones (higher priority wins)
			result[c.Key] = c.Value
		}
	}

	return result, nil
}

// collectAllCandidates collects variables from all layers in priority order (low to high).
func (s *VariableResolutionService) collectAllCandidates(workspaceID string) ([]variableCandidate, error) {
	var candidates []variableCandidate

	// Step 1: Global variable sets
	globalCandidates, err := s.collectGlobalVarsets()
	if err != nil {
		return nil, fmt.Errorf("collecting global varsets: %w", err)
	}
	candidates = append(candidates, globalCandidates...)

	// Step 2: Get workspace's project ID
	var relation struct {
		ProjectID uint
	}
	hasProject := false
	err = s.db.Table("workspace_project_relations").
		Select("project_id").
		Where("workspace_id = ?", workspaceID).
		Scan(&relation).Error
	if err == nil && relation.ProjectID > 0 {
		hasProject = true
	}

	// Step 3: Project-level assignments
	if hasProject {
		projectCandidates, err := s.collectProjectVarsets(relation.ProjectID)
		if err != nil {
			return nil, fmt.Errorf("collecting project varsets: %w", err)
		}
		candidates = append(candidates, projectCandidates...)
	}

	// Step 4: Workspace-level assignments
	wsCandidates, err := s.collectWorkspaceVarsets(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("collecting workspace varsets: %w", err)
	}
	candidates = append(candidates, wsCandidates...)

	// Step 5: Workspace's own variables
	ownCandidates, err := s.collectWorkspaceOwnVariables(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("collecting workspace variables: %w", err)
	}
	candidates = append(candidates, ownCandidates...)

	return candidates, nil
}

// collectGlobalVarsets loads global variable sets and their active variables.
func (s *VariableResolutionService) collectGlobalVarsets() ([]variableCandidate, error) {
	var varsets []models.VariableSet
	if err := s.db.Where("scope = ? AND is_deleted = ?", "global", false).
		Order("created_at ASC").
		Find(&varsets).Error; err != nil {
		return nil, err
	}

	// Collect varset IDs and batch load variables
	varsetIDs := make([]string, len(varsets))
	for i, vs := range varsets {
		varsetIDs[i] = vs.VarsetID
	}
	varsMap, err := s.batchLoadVarsetVariables(varsetIDs)
	if err != nil {
		return nil, err
	}

	var candidates []variableCandidate
	for _, vs := range varsets {
		for _, v := range varsMap[vs.VarsetID] {
			candidates = append(candidates, variableCandidate{
				VariableID:   v.VariableID,
				Key:          v.Key,
				Value:        v.Value,
				Version:      v.Version,
				VariableType: v.VariableType,
				ValueFormat:  v.ValueFormat,
				Sensitive:    v.Sensitive,
				Description:  v.Description,
				SourceType:   "varset",
				SourceID:     vs.VarsetID,
				SourceName:   vs.Name,
				ScopeLevel:   "global",
			})
		}
	}
	return candidates, nil
}

// collectProjectVarsets loads project-level varset assignments and their variables.
func (s *VariableResolutionService) collectProjectVarsets(projectID uint) ([]variableCandidate, error) {
	var assignments []models.VarsetAssignment
	if err := s.db.Joins("JOIN variable_sets ON variable_sets.varset_id = varset_assignments.varset_id AND variable_sets.is_deleted = ?", false).
		Where("varset_assignments.scope_type = ? AND varset_assignments.project_id = ?", "project", projectID).
		Order("varset_assignments.attached_at ASC").
		Find(&assignments).Error; err != nil {
		return nil, err
	}

	// Collect varset IDs and batch load names + variables
	varsetIDs := make([]string, len(assignments))
	for i, a := range assignments {
		varsetIDs[i] = a.VarsetID
	}
	namesMap, err := s.batchLoadVarsetNames(varsetIDs)
	if err != nil {
		return nil, err
	}
	varsMap, err := s.batchLoadVarsetVariables(varsetIDs)
	if err != nil {
		return nil, err
	}

	var candidates []variableCandidate
	for _, a := range assignments {
		for _, v := range varsMap[a.VarsetID] {
			candidates = append(candidates, variableCandidate{
				VariableID:   v.VariableID,
				Key:          v.Key,
				Value:        v.Value,
				Version:      v.Version,
				VariableType: v.VariableType,
				ValueFormat:  v.ValueFormat,
				Sensitive:    v.Sensitive,
				Description:  v.Description,
				SourceType:   "varset",
				SourceID:     a.VarsetID,
				SourceName:   namesMap[a.VarsetID],
				ScopeLevel:   "project",
			})
		}
	}
	return candidates, nil
}

// collectWorkspaceVarsets loads workspace-level varset assignments and their variables.
func (s *VariableResolutionService) collectWorkspaceVarsets(workspaceID string) ([]variableCandidate, error) {
	var assignments []models.VarsetAssignment
	if err := s.db.Joins("JOIN variable_sets ON variable_sets.varset_id = varset_assignments.varset_id AND variable_sets.is_deleted = ?", false).
		Where("varset_assignments.scope_type = ? AND varset_assignments.workspace_id = ?", "workspace", workspaceID).
		Order("varset_assignments.attached_at ASC").
		Find(&assignments).Error; err != nil {
		return nil, err
	}

	// Collect varset IDs and batch load names + variables
	varsetIDs := make([]string, len(assignments))
	for i, a := range assignments {
		varsetIDs[i] = a.VarsetID
	}
	namesMap, err := s.batchLoadVarsetNames(varsetIDs)
	if err != nil {
		return nil, err
	}
	varsMap, err := s.batchLoadVarsetVariables(varsetIDs)
	if err != nil {
		return nil, err
	}

	var candidates []variableCandidate
	for _, a := range assignments {
		for _, v := range varsMap[a.VarsetID] {
			candidates = append(candidates, variableCandidate{
				VariableID:   v.VariableID,
				Key:          v.Key,
				Value:        v.Value,
				Version:      v.Version,
				VariableType: v.VariableType,
				ValueFormat:  v.ValueFormat,
				Sensitive:    v.Sensitive,
				Description:  v.Description,
				SourceType:   "varset",
				SourceID:     a.VarsetID,
				SourceName:   namesMap[a.VarsetID],
				ScopeLevel:   "workspace-specific",
			})
		}
	}
	return candidates, nil
}

// collectWorkspaceOwnVariables loads the workspace's own variables (latest version per variable_id).
func (s *VariableResolutionService) collectWorkspaceOwnVariables(workspaceID string) ([]variableCandidate, error) {
	// Replicate the subquery pattern from WorkspaceVariableService.ListVariables
	subQuery := s.db.Table("workspace_variables").
		Select("variable_id, MAX(version) as max_version").
		Where("workspace_id = ?", workspaceID).
		Group("variable_id")

	var variables []models.WorkspaceVariable
	if err := s.db.Table("workspace_variables").
		Joins("INNER JOIN (?) AS latest ON workspace_variables.variable_id = latest.variable_id AND workspace_variables.version = latest.max_version", subQuery).
		Where("workspace_variables.workspace_id = ? AND workspace_variables.is_deleted = ?", workspaceID, false).
		Find(&variables).Error; err != nil {
		return nil, err
	}

	var candidates []variableCandidate
	for _, v := range variables {
		candidates = append(candidates, variableCandidate{
			VariableID:   v.VariableID,
			Key:          v.Key,
			Value:        v.Value,
			Version:      v.Version,
			VariableType: v.VariableType,
			ValueFormat:  v.ValueFormat,
			Sensitive:    v.Sensitive,
			Description:  v.Description,
			SourceType:   "workspace",
			SourceID:     workspaceID,
			SourceName:   "Workspace",
			ScopeLevel:   "workspace",
		})
	}
	return candidates, nil
}

// batchLoadVarsetVariables loads latest version of variables for multiple varsets in a single query.
func (s *VariableResolutionService) batchLoadVarsetVariables(varsetIDs []string) (map[string][]models.VarsetVariable, error) {
	result := make(map[string][]models.VarsetVariable)
	if len(varsetIDs) == 0 {
		return result, nil
	}

	// Get latest version per variable_id across the requested varsets
	subQuery := s.db.Table("varset_variables").
		Select("variable_id, MAX(version) as max_version").
		Where("varset_id IN ? AND is_deleted = false", varsetIDs).
		Group("variable_id")

	var allVars []models.VarsetVariable
	if err := s.db.Table("varset_variables").
		Joins("INNER JOIN (?) AS latest ON varset_variables.variable_id = latest.variable_id AND varset_variables.version = latest.max_version", subQuery).
		Where("varset_variables.varset_id IN ? AND varset_variables.is_deleted = false", varsetIDs).
		Find(&allVars).Error; err != nil {
		return nil, fmt.Errorf("batch loading varset variables: %w", err)
	}
	for _, v := range allVars {
		result[v.VarsetID] = append(result[v.VarsetID], v)
	}
	return result, nil
}

// batchLoadVarsetNames loads names for multiple varsets in a single query.
func (s *VariableResolutionService) batchLoadVarsetNames(varsetIDs []string) (map[string]string, error) {
	result := make(map[string]string)
	if len(varsetIDs) == 0 {
		return result, nil
	}
	var varsets []models.VariableSet
	if err := s.db.Select("varset_id, name").
		Where("varset_id IN ? AND is_deleted = false", varsetIDs).
		Find(&varsets).Error; err != nil {
		return nil, fmt.Errorf("batch loading varset names: %w", err)
	}
	for _, vs := range varsets {
		result[vs.VarsetID] = vs.Name
	}
	return result, nil
}
