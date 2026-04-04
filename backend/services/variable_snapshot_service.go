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
func (s *VariableSnapshotService) CreateSnapshot(workspaceID string, createdBy *string) (*string, int, error) {
	resolver := NewVariableResolutionService(s.db)
	display, err := resolver.ResolveDisplay(workspaceID)
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

// DeleteSnapshot deletes all rows for a given vsnap_id.
func (s *VariableSnapshotService) DeleteSnapshot(vsnapID string) error {
	result := s.db.Where("vsnap_id = ?", vsnapID).Delete(&models.VariableSnapshot{})
	if result.Error != nil {
		return fmt.Errorf("failed to delete snapshot: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("snapshot not found: %s", vsnapID)
	}
	return nil
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
