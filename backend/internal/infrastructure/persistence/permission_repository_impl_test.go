package persistence

import (
	"context"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

func TestQueryOrgPermissions_ApplicationPrincipalMustBelongToGrantOrg(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Application{}, &entity.PermissionDefinition{}, &entity.OrgPermission{}); err != nil {
		t.Fatal(err)
	}

	if err := db.Create(&entity.PermissionDefinition{
		ID:           "org-permission-test",
		Name:         "org-permission-test",
		DisplayName:  "Organization permission test",
		ResourceType: valueobject.ResourceTypeIAMApplications,
		ScopeLevel:   valueobject.ScopeTypeOrganization,
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create([]*entity.Application{
		{ID: 101, OrgID: 1, Name: "org-one", AppKey: "app-org-one", IsActive: true},
		{ID: 202, OrgID: 2, Name: "org-two", AppKey: "app-org-two", IsActive: true},
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create([]*entity.OrgPermission{
		// Both canonical and numeric legacy IDs are valid for the application
		// in org 1.
		{OrgID: 1, PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-org-one", PermissionID: "org-permission-test", PermissionLevel: valueobject.PermissionLevelRead},
		{OrgID: 1, PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "101", PermissionID: "org-permission-test", PermissionLevel: valueobject.PermissionLevelRead},
		// These rows reference an application in org 2 while claiming an org 1
		// grant. They must never be returned as authority for either alias.
		{OrgID: 1, PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-org-two", PermissionID: "org-permission-test", PermissionLevel: valueobject.PermissionLevelRead},
		{OrgID: 1, PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "202", PermissionID: "org-permission-test", PermissionLevel: valueobject.PermissionLevelRead},
	}).Error; err != nil {
		t.Fatal(err)
	}

	repo := &PermissionRepositoryImpl{db: db}
	permissions, err := repo.QueryOrgPermissions(
		context.Background(),
		1,
		valueobject.PrincipalTypeApplication,
		[]string{"app-org-one", "101", "app-org-two", "202"},
		valueobject.ResourceTypeIAMApplications,
	)
	if err != nil {
		t.Fatal(err)
	}

	got := make(map[string]struct{}, len(permissions))
	for _, permission := range permissions {
		got[permission.PrincipalID] = struct{}{}
	}
	for _, principalID := range []string{"app-org-one", "101"} {
		if _, ok := got[principalID]; !ok {
			t.Fatalf("expected valid application grant for %q, got %#v", principalID, got)
		}
	}
	for _, principalID := range []string{"app-org-two", "202"} {
		if _, ok := got[principalID]; ok {
			t.Fatalf("cross-org application grant %q must be excluded, got %#v", principalID, got)
		}
	}
}
