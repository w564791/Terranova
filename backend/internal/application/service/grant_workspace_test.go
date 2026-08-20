package service

import (
	"context"
	"testing"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

func TestGrantPermission_Workspace(t *testing.T) {
	db := openMemDB(t)
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (7, 'ws-grant')`)

	repo := &grantStub{}
	s := &PermissionServiceImpl{permissionRepo: repo, auditRepo: &stubAuditRepo{}, db: db}
	err := s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 7,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "wspm-1", PermissionLevel: valueobject.PermissionLevelAdmin, GrantedBy: "admin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if repo.lastWs == nil || repo.lastWs.WorkspaceID != "ws-grant" {
		t.Fatalf("ws grant: %+v", repo.lastWs)
	}
	if repo.lastWs.PermissionLevel != valueobject.PermissionLevelAdmin {
		t.Fatal("level")
	}
}

func TestGrantPermission_WorkspaceNotFound(t *testing.T) {
	db := openMemDB(t)
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`)
	s := &PermissionServiceImpl{permissionRepo: &grantStub{}, auditRepo: &stubAuditRepo{}, db: db}
	err := s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 999,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "wspm-1", PermissionLevel: valueobject.PermissionLevelRead, GrantedBy: "admin",
	})
	if err == nil {
		t.Fatal("expected workspace not found")
	}
}

func TestLogAccess_Smoke(t *testing.T) {
	c := &PermissionCheckerImpl{auditRepo: &stubAuditRepo{}}
	c.logAccess(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	}, &CheckPermissionResult{IsAllowed: false, DenyReason: "No permission"}, 0)
}

func TestParseUint_viaScopeIDStrInvalid(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	// invalid non-workspace scope id str
	_, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeIDStr: "not-num", RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err == nil {
		t.Fatal("expected invalid scope_id format")
	}
}

// ensure grantStub implements GrantWorkspacePermission
var _ interface {
	GrantWorkspacePermission(context.Context, *entity.WorkspacePermission) error
} = (*grantStub)(nil)
