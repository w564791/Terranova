package services

import (
	"fmt"
	"iac-platform/internal/infrastructure"
	"iac-platform/internal/models"

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

	var result []models.WorkspaceVariable
	for _, ref := range refs {
		switch ref.SourceType {
		case "workspace":
			var v models.WorkspaceVariable
			if err := s.db.Where("variable_id = ? AND version = ?", ref.VariableID, ref.Version).
				First(&v).Error; err == nil {
				result = append(result, v)
			}
		case "varset":
			var v models.VarsetVariable
			if err := s.db.Where("variable_id = ? AND version = ?", ref.VariableID, ref.Version).
				First(&v).Error; err == nil {
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
			}
		}
	}

	return result, nil
}
