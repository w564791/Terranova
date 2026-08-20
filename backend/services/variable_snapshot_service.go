package services

import (
	"fmt"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"
	"log"

	"gorm.io/gorm"
)

type VariableSnapshotService struct {
	db *gorm.DB
}

func NewVariableSnapshotService(db *gorm.DB) *VariableSnapshotService {
	return &VariableSnapshotService{db: db}
}

// CreateSnapshot creates a variable snapshot for a workspace.
// Uses ResolveDisplay to get effective variables with SourceType.
// Returns vsnap_id (nil if no variables) and item count.
//
// 若 workspace 当前有 active manifest deployment,deployment 选定的 varsets 会被折进
// 优先级链一并快照(它们是带 version 的 varset 变量,可被引用机制固化)。
// deployment 的 variable_overrides 不在这里 —— 它无 variable_id,由任务行的
// variable_overrides 列单独快照(见 workspace_task_controller)。
func (s *VariableSnapshotService) CreateSnapshot(workspaceID string, createdBy *string) (*string, int, error) {
	resolver := NewVariableResolutionService(s.db)

	// 折入 active deployment 的 varsets(overrides 单独处理,见上注释)
	extraVarsetIDs, _, exErr := resolver.GetActiveDeploymentExtras(workspaceID)
	if exErr != nil {
		// best-effort: 拿不到 deployment 信息不阻塞快照,退回无 extra
		log.Printf("[WARN] resolve active deployment extras for %s failed: %v", workspaceID, exErr)
		extraVarsetIDs = nil
	}

	display, err := resolver.resolveDisplayWithExtraVarsets(workspaceID, extraVarsetIDs)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to resolve variables: %w", err)
	}

	// Filter non-overridden (effective) variables
	var effective []EffectiveVariable
	for _, ev := range display {
		if !ev.IsOverridden {
			effective = append(effective, ev)
		}
	}

	// No variables → return nil
	if len(effective) == 0 {
		return nil, 0, nil
	}

	vsnapID, err := infrastructure.GenerateVsnapID()
	if err != nil {
		return nil, 0, fmt.Errorf("failed to generate vsnap_id: %w", err)
	}

	// Batch insert snapshot rows
	snapshots := make([]models.VariableSnapshot, 0, len(effective))
	for _, ev := range effective {
		snapshots = append(snapshots, models.VariableSnapshot{
			VsnapID:      vsnapID,
			WorkspaceID:  workspaceID,
			VariableID:   ev.VariableID,
			Version:      ev.Version,
			VariableType: string(ev.VariableType),
			SourceType:   ev.SourceType,
			CreatedBy:    createdBy,
		})
	}

	if err := s.db.Create(&snapshots).Error; err != nil {
		return nil, 0, fmt.Errorf("failed to create snapshot: %w", err)
	}

	return &vsnapID, len(snapshots), nil
}

// DeleteSnapshot deletes all rows for a given vsnap_id within a workspace (防跨 WS IDOR).
func (s *VariableSnapshotService) DeleteSnapshot(workspaceID, vsnapID string) error {
	if workspaceID == "" || vsnapID == "" {
		return fmt.Errorf("workspace_id and vsnap_id are required")
	}
	result := s.db.Where("vsnap_id = ? AND workspace_id = ?", vsnapID, workspaceID).
		Delete(&models.VariableSnapshot{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete snapshot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("snapshot not found: %s", vsnapID)
	}
	return nil
}

// SnapshotBelongsToWorkspace reports whether any row of vsnap_id is under workspace_id.
func (s *VariableSnapshotService) SnapshotBelongsToWorkspace(workspaceID, vsnapID string) (bool, error) {
	var count int64
	err := s.db.Model(&models.VariableSnapshot{}).
		Where("vsnap_id = ? AND workspace_id = ?", vsnapID, workspaceID).
		Count(&count).Error
	return count > 0, err
}

// LoadFromSnapshot loads full variable data by resolving snapshot references.
// Queries workspace_variables or varset_variables based on source_type.
// Returns complete variables with sensitive values decrypted (via GORM AfterFind hooks).
func (s *VariableSnapshotService) LoadFromSnapshot(vsnapID string) ([]models.WorkspaceVariable, error) {
	var refs []models.VariableSnapshot
	if err := s.db.Where("vsnap_id = ?", vsnapID).Find(&refs).Error; err != nil {
		return nil, fmt.Errorf("failed to load snapshot refs: %w", err)
	}

	// Group refs by source_type
	type varRef struct {
		VariableID string
		Version    int
	}
	var wsRefs, varsetRefs []varRef
	wsRefMap := make(map[string]bool)
	varsetRefMap := make(map[string]bool)

	for _, ref := range refs {
		r := varRef{VariableID: ref.VariableID, Version: ref.Version}
		switch ref.SourceType {
		case "workspace":
			wsRefs = append(wsRefs, r)
			wsRefMap[fmt.Sprintf("%s:%d", r.VariableID, r.Version)] = true
		case "varset":
			varsetRefs = append(varsetRefs, r)
			varsetRefMap[fmt.Sprintf("%s:%d", r.VariableID, r.Version)] = true
		}
	}

	var result []models.WorkspaceVariable

	// Batch load workspace variables
	if len(wsRefs) > 0 {
		var wsVars []models.WorkspaceVariable
		tx := s.db
		for i, r := range wsRefs {
			if i == 0 {
				tx = tx.Where("(variable_id = ? AND version = ?)", r.VariableID, r.Version)
			} else {
				tx = tx.Or("(variable_id = ? AND version = ?)", r.VariableID, r.Version)
			}
		}
		if err := tx.Find(&wsVars).Error; err != nil {
			log.Printf("[WARN] Failed to batch load workspace variables: %v", err)
		} else {
			found := make(map[string]bool)
			for _, v := range wsVars {
				result = append(result, v)
				found[fmt.Sprintf("%s:%d", v.VariableID, v.Version)] = true
			}
			for key := range wsRefMap {
				if !found[key] {
					log.Printf("[WARN] Snapshot ref not found in workspace_variables: %s", key)
				}
			}
		}
	}

	// Batch load varset variables
	if len(varsetRefs) > 0 {
		var vsetVars []models.VarsetVariable
		tx := s.db
		for i, r := range varsetRefs {
			if i == 0 {
				tx = tx.Where("(variable_id = ? AND version = ?)", r.VariableID, r.Version)
			} else {
				tx = tx.Or("(variable_id = ? AND version = ?)", r.VariableID, r.Version)
			}
		}
		if err := tx.Find(&vsetVars).Error; err != nil {
			log.Printf("[WARN] Failed to batch load varset variables: %v", err)
		} else {
			found := make(map[string]bool)
			for _, v := range vsetVars {
				result = append(result, models.WorkspaceVariable{
					VariableID:   v.VariableID,
					Key:          v.Key,
					Value:        v.Value,
					VariableType: v.VariableType,
					ValueFormat:  v.ValueFormat,
					Sensitive:    v.Sensitive,
					Description:  v.Description,
					Version:      v.Version,
				})
				found[fmt.Sprintf("%s:%d", v.VariableID, v.Version)] = true
			}
			for key := range varsetRefMap {
				if !found[key] {
					log.Printf("[WARN] Snapshot ref not found in varset_variables: %s", key)
				}
			}
		}
	}

	return result, nil
}
