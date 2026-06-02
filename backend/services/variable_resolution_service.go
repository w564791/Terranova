package services

import (
	"encoding/json"
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
	return s.resolveDisplayWithExtraVarsets(workspaceID, nil)
}

// resolveDisplayWithExtraVarsets 与 ResolveDisplay 相同,但额外把 manifest deployment 选定的
// varsets(extraVarsetIDs,按 priority ASC)折进优先级链(第 4 层之后、workspace own 之前)。
// 供任务变量快照创建复用:deployment varsets 本身是带 version 的 varset 变量,可被快照引用机制
// 原样固化。注意 variable_overrides 不在这里处理(它无 variable_id,无法做引用快照)。
func (s *VariableResolutionService) resolveDisplayWithExtraVarsets(
	workspaceID string, extraVarsetIDs []string,
) ([]EffectiveVariable, error) {
	candidates, err := s.collectAllCandidatesWithExtra(workspaceID, extraVarsetIDs)
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
	return s.ResolveExecutionWithExtra(workspaceID, nil, nil)
}

// ResolveExecutionWithExtra 在常规优先级链基础上,把 manifest deployment 选定的 varset 与
// variable_overrides 注入。优先级:
//
//	低 → global varset → project varset → workspace-attached varset
//	→ deployment varset (extraVarsetIDs,数组顺序即 priority 从低到高)
//	→ workspace own variable
//	→ deployment variable_overrides (overrides,最高)
//
// extraVarsetIDs:已按 priority ASC 排序好的 varset id 列表
// overrides: deployment 的 variable_overrides(扁平 key=value),不含敏感值
func (s *VariableResolutionService) ResolveExecutionWithExtra(
	workspaceID string,
	extraVarsetIDs []string,
	overrides map[string]string,
) ([]models.WorkspaceVariable, error) {
	candidates, err := s.collectAllCandidatesWithExtra(workspaceID, extraVarsetIDs)
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
			Value:        c.Value,
			VariableType: c.VariableType,
			ValueFormat:  c.ValueFormat,
			Sensitive:    c.Sensitive,
			Description:  c.Description,
			Version:      c.Version,
		})
	}

	// overrides 最后注入,直接覆盖同 key
	for k, v := range overrides {
		// overrides 不存敏感值(spec §11.3),按 terraform 类型归类用 Terraform
		// 已存在则覆盖,不存在则追加
		found := false
		for i := range result {
			if result[i].Key == k {
				result[i].Value = v
				result[i].Sensitive = false
				result[i].VariableID = "override-" + k
				found = true
				break
			}
		}
		if !found {
			result = append(result, models.WorkspaceVariable{
				VariableID:   "override-" + k,
				WorkspaceID:  workspaceID,
				Key:          k,
				Value:        v,
				VariableType: models.VariableTypeTerraform,
				Sensitive:    false,
			})
		}
	}

	return result, nil
}

// ResolveDisplayWithExtra 与 ResolveDisplay 类似,但接受 manifest deployment 注入的
// extraVarsetIDs 与 overrides。敏感值仍然 mask。
func (s *VariableResolutionService) ResolveDisplayWithExtra(
	workspaceID string,
	extraVarsetIDs []string,
	overrides map[string]string,
) (map[string]string, error) {
	full, err := s.ResolveExecutionWithExtra(workspaceID, extraVarsetIDs, overrides)
	if err != nil {
		return nil, err
	}
	out := make(map[string]string, len(full))
	for _, v := range full {
		if v.Sensitive {
			out[v.Key] = ""
			continue
		}
		out[v.Key] = v.Value
	}
	return out, nil
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
	return s.collectAllCandidatesWithExtra(workspaceID, nil)
}

// collectAllCandidatesWithExtra 在原优先级链中,在 workspace-attached 与 workspace own 之间
// 插入 manifest deployment 选定的 varset (extraVarsetIDs)。
func (s *VariableResolutionService) collectAllCandidatesWithExtra(
	workspaceID string, extraVarsetIDs []string,
) ([]variableCandidate, error) {
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

	// Step 4.5 (新): manifest deployment 选定的 varsets
	if len(extraVarsetIDs) > 0 {
		extraCandidates, err := s.collectExtraVarsets(extraVarsetIDs)
		if err != nil {
			return nil, fmt.Errorf("collecting deployment varsets: %w", err)
		}
		candidates = append(candidates, extraCandidates...)
	}

	// Step 5: Workspace's own variables
	ownCandidates, err := s.collectWorkspaceOwnVariables(workspaceID)
	if err != nil {
		return nil, fmt.Errorf("collecting workspace variables: %w", err)
	}
	candidates = append(candidates, ownCandidates...)

	return candidates, nil
}

// collectExtraVarsets 加载指定 varset 列表的变量,按入参顺序压栈
// (调用方按 priority ASC 排序传入,数字大者最后压栈,后续覆盖语义自然成立)
func (s *VariableResolutionService) collectExtraVarsets(varsetIDs []string) ([]variableCandidate, error) {
	if len(varsetIDs) == 0 {
		return nil, nil
	}
	var varsets []models.VariableSet
	if err := s.db.Where("varset_id IN ? AND is_deleted = ?", varsetIDs, false).
		Find(&varsets).Error; err != nil {
		return nil, err
	}
	// 转 map 便于按入参顺序遍历
	vsByID := make(map[string]models.VariableSet, len(varsets))
	for _, vs := range varsets {
		vsByID[vs.VarsetID] = vs
	}
	varsMap, err := s.batchLoadVarsetVariables(varsetIDs)
	if err != nil {
		return nil, err
	}
	var out []variableCandidate
	for _, id := range varsetIDs {
		vs, ok := vsByID[id]
		if !ok {
			continue
		}
		for _, v := range varsMap[id] {
			out = append(out, variableCandidate{
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
				ScopeLevel:   "manifest_deployment",
			})
		}
	}
	return out, nil
}

// GetActiveDeploymentExtras 返回 workspace 当前 active manifest deployment 注入到优先级链的
// 额外变量来源:按 priority ASC 排序的 varset id 列表 + variable_overrides(扁平 key=value)。
//
// 无 active deployment 时返回 (nil, nil, nil)。供任务变量快照创建与执行路径复用,
// 确保 deployment 选定的 varsets / overrides 真正参与 plan/apply,而不只是 install 对话框预览。
func (s *VariableResolutionService) GetActiveDeploymentExtras(
	workspaceID string,
) (extraVarsetIDs []string, overrides map[string]string, err error) {
	var dep models.ManifestDeployment
	res := s.db.Where("workspace_id = ? AND status = ?", workspaceID, models.DeploymentStatusActive).
		Order("deployed_at DESC").
		Limit(1).
		Find(&dep)
	if res.Error != nil {
		return nil, nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, nil, nil
	}

	// varsets 按 priority ASC(数字大者优先级高 → 后压栈 → 覆盖语义成立)
	var links []models.ManifestDeploymentVarset
	if err := s.db.Where("deployment_id = ?", dep.ID).
		Order("priority ASC").
		Find(&links).Error; err != nil {
		return nil, nil, err
	}
	for _, l := range links {
		extraVarsetIDs = append(extraVarsetIDs, l.VarsetID)
	}

	// variable_overrides: JSONB 解出扁平 key=string
	if len(dep.VariableOverrides) > 0 {
		var raw map[string]interface{}
		if jsonErr := json.Unmarshal(dep.VariableOverrides, &raw); jsonErr == nil {
			overrides = make(map[string]string, len(raw))
			for k, v := range raw {
				if sv, ok := v.(string); ok {
					overrides[k] = sv
				} else {
					overrides[k] = fmt.Sprintf("%v", v)
				}
			}
		}
	}

	return extraVarsetIDs, overrides, nil
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
