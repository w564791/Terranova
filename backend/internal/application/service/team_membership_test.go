package service

import (
	"context"
	"testing"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/infrastructure/persistence"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTeamMembershipServiceDB(t *testing.T, withOrganizationMembership bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Organization{}, &entity.Team{}, &entity.TeamMember{}); err != nil {
		t.Fatal(err)
	}
	if withOrganizationMembership {
		if err := db.AutoMigrate(&entity.UserOrganization{}); err != nil {
			t.Fatal(err)
		}
		if err := db.Exec(`CREATE UNIQUE INDEX user_organizations_user_org_key ON user_organizations (user_id, org_id)`).Error; err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Create(&entity.Organization{ID: 1, Name: "org-1", IsActive: true}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&entity.Team{ID: "team-1", OrgID: 1, Name: "team-1", DisplayName: "Team 1"}).Error; err != nil {
		t.Fatal(err)
	}
	return db
}

func newTeamMembershipService(db *gorm.DB) TeamService {
	return NewTeamService(
		persistence.NewTeamRepository(db),
		persistence.NewOrganizationRepository(db),
		nil,
	)
}

func TestAddTeamMemberEnsuresOrganizationMembership(t *testing.T) {
	db := setupTeamMembershipServiceDB(t, true)
	svc := newTeamMembershipService(db)

	if err := svc.AddTeamMember(context.Background(), &AddTeamMemberRequest{
		TeamID:  "team-1",
		UserID:  "user-1",
		Role:    entity.TeamRoleMember,
		AddedBy: "admin-1",
	}); err != nil {
		t.Fatalf("add team member: %v", err)
	}

	var memberCount, membershipCount int64
	if err := db.Model(&entity.TeamMember{}).
		Where("team_id = ? AND user_id = ?", "team-1", "user-1").
		Count(&memberCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", "user-1", 1).
		Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if memberCount != 1 || membershipCount != 1 {
		t.Fatalf("want one team and organization membership, got team=%d org=%d", memberCount, membershipCount)
	}

	if err := svc.AddTeamMember(context.Background(), &AddTeamMemberRequest{
		TeamID:  "team-1",
		UserID:  "user-1",
		Role:    entity.TeamRoleMember,
		AddedBy: "admin-1",
	}); err == nil {
		t.Fatal("duplicate team member must be rejected")
	}
	if err := db.Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", "user-1", 1).
		Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 {
		t.Fatalf("duplicate request must not create another organization membership, got %d", membershipCount)
	}
}

func TestAddTeamMemberRollsBackWhenOrganizationMembershipCannotBeWritten(t *testing.T) {
	// Deliberately omit user_organizations. The repository must roll back the
	// team_members insert when the companion membership write fails.
	db := setupTeamMembershipServiceDB(t, false)
	svc := newTeamMembershipService(db)

	if err := svc.AddTeamMember(context.Background(), &AddTeamMemberRequest{
		TeamID:  "team-1",
		UserID:  "user-1",
		Role:    entity.TeamRoleMember,
		AddedBy: "admin-1",
	}); err == nil {
		t.Fatal("expected membership-write failure")
	}

	var memberCount int64
	if err := db.Model(&entity.TeamMember{}).
		Where("team_id = ? AND user_id = ?", "team-1", "user-1").
		Count(&memberCount).Error; err != nil {
		t.Fatal(err)
	}
	if memberCount != 0 {
		t.Fatalf("team member must roll back with membership failure, got %d rows", memberCount)
	}
}
