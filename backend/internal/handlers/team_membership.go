package handlers

import (
	"context"
	"fmt"
	"time"

	"iac-platform/internal/domain/entity"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ensureTeamMembersOrganizationMemberships repairs the tenant-context rows
// for an already-populated team. It deliberately only inserts missing rows;
// revoking one team membership must not remove a user_organizations row that
// may also be justified by another team or a direct role.
//
// The caller supplies its transaction so a newly assigned team role can never
// commit without the memberships required to reach that role through the
// active-organization bootstrap.
func ensureTeamMembersOrganizationMemberships(
	ctx context.Context,
	tx *gorm.DB,
	teamID string,
	orgID uint,
) error {
	if tx == nil || teamID == "" || orgID == 0 {
		return fmt.Errorf("team membership repair requires team and organization")
	}

	var userIDs []string
	if err := tx.WithContext(ctx).
		Table("team_members").
		Where("team_id = ?", teamID).
		Distinct("user_id").
		Pluck("user_id", &userIDs).Error; err != nil {
		return fmt.Errorf("list team members: %w", err)
	}
	if len(userIDs) == 0 {
		return nil
	}

	now := time.Now()
	memberships := make([]entity.UserOrganization, 0, len(userIDs))
	for _, userID := range userIDs {
		if userID == "" {
			continue
		}
		memberships = append(memberships, entity.UserOrganization{
			UserID:   userID,
			OrgID:    orgID,
			JoinedAt: now,
		})
	}
	if len(memberships) == 0 {
		return nil
	}

	if err := tx.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "org_id"}},
			DoNothing: true,
		}).
		Create(&memberships).Error; err != nil {
		return fmt.Errorf("ensure team organization memberships: %w", err)
	}
	return nil
}
