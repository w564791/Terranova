package service

import (
	"context"
	"testing"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/infrastructure/persistence"
	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupOrganizationLifecycleDB(t *testing.T, withAdminRole bool) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(
		&models.User{},
		&entity.Organization{},
		&entity.Team{},
		&entity.UserOrganization{},
		&entity.Role{},
		&entity.UserRole{},
	); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		"CREATE UNIQUE INDEX user_organizations_user_org_key ON user_organizations (user_id, org_id)",
		"CREATE UNIQUE INDEX iam_user_roles_identity_key ON iam_user_roles (user_id, role_id, scope_type, scope_id)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatal(err)
		}
	}

	users := []*models.User{
		{ID: "sys-active", Username: "sys-active", Email: "sys-active@example.test", PasswordHash: "x", IsActive: true, IsSystemAdmin: true},
		{ID: "sys-disabled", Username: "sys-disabled", Email: "sys-disabled@example.test", PasswordHash: "x", IsActive: false, IsSystemAdmin: true},
		{ID: "regular", Username: "regular", Email: "regular@example.test", PasswordHash: "x", IsActive: true, IsSystemAdmin: false},
	}
	for _, user := range users {
		if err := db.Create(user).Error; err != nil {
			t.Fatal(err)
		}
	}
	// GORM's default:true tag omits a false zero-value on INSERT, so force the
	// disabled fixture after creation to exercise the active-user predicate.
	if err := db.Model(&models.User{}).Where("user_id = ?", "sys-disabled").Update("is_active", false).Error; err != nil {
		t.Fatal(err)
	}
	if withAdminRole {
		if err := db.Create(&entity.Role{
			OrgID:       0,
			Name:        "admin",
			DisplayName: "Platform Administrator",
			IsSystem:    true,
			IsActive:    true,
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func newOrganizationLifecycleService(db *gorm.DB) OrganizationService {
	return NewOrganizationService(
		persistence.NewOrganizationRepository(db),
		persistence.NewTeamRepository(db),
		nil,
	)
}

func TestCreateOrganizationBootstrapsSystemAdminTenantAccess(t *testing.T) {
	db := setupOrganizationLifecycleDB(t, true)
	svc := newOrganizationLifecycleService(db)

	org, err := svc.CreateOrganization(context.Background(), &CreateOrganizationRequest{
		Name:        "new-org",
		DisplayName: "New Organization",
		CreatedBy:   "sys-active",
	})
	if err != nil {
		t.Fatalf("create organization: %v", err)
	}

	var teamCount int64
	if err := db.Model(&entity.Team{}).Where("org_id = ?", org.ID).Count(&teamCount).Error; err != nil {
		t.Fatal(err)
	}
	if teamCount != 2 {
		t.Fatalf("want two default teams, got %d", teamCount)
	}

	var membershipCount int64
	if err := db.Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", "sys-active", org.ID).
		Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 {
		t.Fatalf("active system administrator needs membership, got %d", membershipCount)
	}
	if err := db.Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", "sys-disabled", org.ID).
		Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 0 {
		t.Fatalf("inactive system administrator must not be bootstrapped, got %d", membershipCount)
	}

	var assignment entity.UserRole
	if err := db.Joins("JOIN iam_roles ON iam_roles.id = iam_user_roles.role_id").
		Where("iam_user_roles.user_id = ? AND iam_user_roles.scope_type = ? AND iam_user_roles.scope_id = ? AND iam_roles.name = ?", "sys-active", "ORGANIZATION", org.ID, "admin").
		First(&assignment).Error; err != nil {
		t.Fatalf("active system administrator needs explicit org admin role: %v", err)
	}
}

func TestCreateOrganizationRollsBackWhenAdminBootstrapIsUnavailable(t *testing.T) {
	db := setupOrganizationLifecycleDB(t, false)
	svc := newOrganizationLifecycleService(db)

	if _, err := svc.CreateOrganization(context.Background(), &CreateOrganizationRequest{
		Name: "cannot-bootstrap", CreatedBy: "sys-active",
	}); err == nil {
		t.Fatal("expected missing canonical admin role to abort organization creation")
	}

	var count int64
	if err := db.Model(&entity.Organization{}).Where("name = ?", "cannot-bootstrap").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("organization creation must roll back, got %d rows", count)
	}
}

func TestReactivatingOrganizationRestoresSystemAdminAccess(t *testing.T) {
	db := setupOrganizationLifecycleDB(t, true)
	if err := db.Create(&entity.Organization{ID: 41, Name: "previously-inactive", IsActive: false}).Error; err != nil {
		t.Fatal(err)
	}
	svc := newOrganizationLifecycleService(db)

	if err := svc.UpdateOrganization(context.Background(), &UpdateOrganizationRequest{
		ID:       41,
		IsActive: true,
	}); err != nil {
		t.Fatalf("reactivate organization: %v", err)
	}

	var membershipCount, assignmentCount int64
	if err := db.Model(&entity.UserOrganization{}).
		Where("user_id = ? AND org_id = ?", "sys-active", 41).
		Count(&membershipCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Table("iam_user_roles").
		Where("user_id = ? AND scope_type = ? AND scope_id = ?", "sys-active", "ORGANIZATION", 41).
		Count(&assignmentCount).Error; err != nil {
		t.Fatal(err)
	}
	if membershipCount != 1 || assignmentCount != 1 {
		t.Fatalf("reactivation must bootstrap membership and role, got membership=%d role=%d", membershipCount, assignmentCount)
	}
}
