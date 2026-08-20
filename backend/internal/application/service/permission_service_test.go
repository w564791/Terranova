package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// grantStub extends stubPermRepo with capture of last grant
type grantStub struct {
	stubPermRepo
	lastOrg *entity.OrgPermission
	lastWs  *entity.WorkspacePermission
}

func (g *grantStub) GrantOrgPermission(ctx context.Context, p *entity.OrgPermission) error {
	g.lastOrg = p
	return nil
}
func (g *grantStub) GrantWorkspacePermission(ctx context.Context, p *entity.WorkspacePermission) error {
	g.lastWs = p
	return nil
}

func TestNewPermissionServiceAndChecker(t *testing.T) {
	repo := &grantStub{}
	chk := NewPermissionChecker(repo, nil, nil, nil, &stubAuditRepo{})
	if chk == nil {
		t.Fatal("checker")
	}
	svc := NewPermissionService(repo, &stubAuditRepo{}, chk, nil)
	if svc == nil {
		t.Fatal("service")
	}
}

func TestValidateGrantRequest(t *testing.T) {
	s := &PermissionServiceImpl{}
	err := s.validateGrantRequest(&GrantPermissionRequest{})
	if err == nil {
		t.Fatal("empty request should fail")
	}
	err = s.validateGrantRequest(&GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "p1", PermissionLevel: valueobject.PermissionLevelRead,
		GrantedBy: "admin",
	})
	if err != nil {
		t.Fatalf("valid request: %v", err)
	}

	// field-level failures
	base := GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "p1", PermissionLevel: valueobject.PermissionLevelRead,
		GrantedBy: "admin",
	}
	cases := []struct {
		name string
		mut  func(*GrantPermissionRequest)
	}{
		{"scope_id0", func(r *GrantPermissionRequest) { r.ScopeID = 0 }},
		{"empty principal", func(r *GrantPermissionRequest) { r.PrincipalID = "" }},
		{"empty permission", func(r *GrantPermissionRequest) { r.PermissionID = "" }},
		{"empty granted_by", func(r *GrantPermissionRequest) { r.GrantedBy = "" }},
		{"invalid level", func(r *GrantPermissionRequest) { r.PermissionLevel = valueobject.PermissionLevel(99) }},
	}
	for _, tc := range cases {
		r := base
		tc.mut(&r)
		if err := s.validateGrantRequest(&r); err == nil {
			t.Fatalf("%s: expected error", tc.name)
		}
	}
}

func TestGrantPermission_ApplicationOnlyAtOrg(t *testing.T) {
	repo := &grantStub{}
	s := &PermissionServiceImpl{permissionRepo: repo, auditRepo: &stubAuditRepo{}}
	err := s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-1",
		PermissionID: "p1", PermissionLevel: valueobject.PermissionLevelRead,
		GrantedBy: "admin",
	})
	if err == nil {
		t.Fatal("application cannot grant at workspace")
	}
	err = s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-1",
		PermissionID: "orgpm-modules", PermissionLevel: valueobject.PermissionLevelWrite,
		GrantedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastOrg == nil || repo.lastOrg.PrincipalType != valueobject.PrincipalTypeApplication {
		t.Fatalf("expected app org grant: %+v", repo.lastOrg)
	}
	if repo.lastOrg.PermissionLevel != valueobject.PermissionLevelWrite {
		t.Fatal("level")
	}
}

func TestGrantPermission_WithExpires(t *testing.T) {
	repo := &grantStub{}
	s := &PermissionServiceImpl{permissionRepo: repo, auditRepo: &stubAuditRepo{}}
	exp := time.Now().Add(24 * time.Hour)
	err := s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "p1", PermissionLevel: valueobject.PermissionLevelRead,
		GrantedBy: "admin", ExpiresAt: &exp,
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastOrg == nil || repo.lastOrg.ExpiresAt == nil {
		t.Fatal("expires_at not stored")
	}
}

func TestCheckPermission_ApplicationPrincipal(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-1",
			PermissionLevel: valueobject.PermissionLevelRead,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
			GrantedAt:       time.Now(),
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeApplication, PrincipalID: "app-1",
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("app principal should allow: %+v", res)
	}
}
