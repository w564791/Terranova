package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

func orgPolicy(roleID uint, rt valueobject.ResourceType, level string, permScope valueobject.ScopeType) *entity.RolePolicy {
	return &entity.RolePolicy{
		RoleID: roleID, PermissionID: string(rt), PermissionLevel: level, ScopeType: "ORGANIZATION",
		ResourceType: string(rt), PermissionScopeLevel: string(permScope),
	}
}

func TestParseWorkspaceCapability(t *testing.T) {
	cap, err := ParseWorkspaceCapability("WORKSPACE_RESOURCES:WRITE")
	if err != nil || cap.ResourceType != valueobject.ResourceTypeWorkspaceResources || cap.Level != valueobject.PermissionLevelWrite {
		t.Fatalf("got %+v, %v", cap, err)
	}
	if cap, err := ParseWorkspaceCapability("workspace_state:read"); err != nil || cap.ResourceType != valueobject.ResourceTypeWorkspaceState {
		t.Fatalf("case-insensitive parse failed: %+v, %v", cap, err)
	}
	for _, bad := range []string{
		"", "WORKSPACE_RESOURCES", "WORKSPACE_RESOURCES:", ":WRITE", "WORKSPACE_RESOURCES:WRITE:X",
		"NOPE:WRITE", "WORKSPACE_RESOURCES:SUPER", "WORKSPACE_RESOURCES:NONE",
		"MANIFESTS:WRITE", // org-level resource is not a workspace capability
		"PROJECT_SETTINGS:READ",
	} {
		if _, err := ParseWorkspaceCapability(bad); err == nil {
			t.Fatalf("ParseWorkspaceCapability(%q) must fail", bad)
		}
	}
}

func TestWorkspaceListAccess_CapabilityNarrowsToCapableWorkspaces(t *testing.T) {
	db := setupWorkspaceListAccessDB(t)
	now := time.Now()
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{
			// deployer: org-wide list + RESOURCES WRITE on ws-project-a only
			{UserID: "u-dep", RoleID: 20, RoleName: "org-reader", ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: now},
			{UserID: "u-dep", RoleID: 21, RoleName: "ws-a-resources", ScopeType: "WORKSPACE", ScopeID: 100, AssignedAt: now},
			// manifest author: org-wide list + MANIFESTS ADMIN, no workspace write
			{UserID: "u-mf", RoleID: 22, RoleName: "manifest-admin", ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: now},
			// org-wide WORKSPACE_MANAGEMENT WRITE (umbrella) => full capability
			{UserID: "u-mgmt", RoleID: 23, RoleName: "org-ws-admin", ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: now},
			// RESOURCES READ only on ws-project-b
			{UserID: "u-dep", RoleID: 24, RoleName: "ws-b-resources-read", ScopeType: "WORKSPACE", ScopeID: 200, AssignedAt: now},
		},
		policies: map[uint][]*entity.RolePolicy{
			20: {orgPolicy(20, valueobject.ResourceTypeAllWorkspaces, "READ", valueobject.ScopeTypeOrganization)},
			21: {{RoleID: 21, PermissionID: "wr", PermissionLevel: "WRITE", ScopeType: "WORKSPACE",
				ResourceType: string(valueobject.ResourceTypeWorkspaceResources), PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace)}},
			22: {
				orgPolicy(22, valueobject.ResourceTypeAllWorkspaces, "READ", valueobject.ScopeTypeOrganization),
				orgPolicy(22, valueobject.ResourceTypeManifests, "ADMIN", valueobject.ScopeTypeOrganization),
			},
			23: {orgPolicy(23, valueobject.ResourceTypeWorkspaceManagement, "WRITE", valueobject.ScopeTypeWorkspace)},
			24: {{RoleID: 24, PermissionID: "wr-read", PermissionLevel: "READ", ScopeType: "WORKSPACE",
				ResourceType: string(valueobject.ResourceTypeWorkspaceResources), PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace)}},
		},
	}
	resolver := NewWorkspaceListAccessService(db, newTestChecker(t, perm, nil, workspaceListProjectRepo(db)))
	capability := &WorkspaceCapability{ResourceType: valueobject.ResourceTypeWorkspaceResources, Level: valueobject.PermissionLevelWrite}
	resolve := func(user string) *WorkspaceListAccess {
		t.Helper()
		access, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{
			UserID: user, OrgID: 1, Capability: capability,
		})
		if err != nil {
			t.Fatal(err)
		}
		return access
	}

	dep := resolve("u-dep")
	if dep.FullOrganization || !dep.HasAccess || len(dep.WorkspaceIDs) != 1 || dep.WorkspaceIDs[0] != "ws-project-a" {
		t.Fatalf("deployer must only see ws-project-a (READ on ws-project-b is insufficient): %+v", dep)
	}
	if !dep.HasAnyWorkspace() || !dep.AllowsWorkspace("ws-project-a") || dep.AllowsWorkspace("ws-project-b") {
		t.Fatalf("helpers disagree with allow-list: %+v", dep)
	}

	mf := resolve("u-mf")
	if !mf.HasAccess || mf.FullOrganization || len(mf.WorkspaceIDs) != 0 || mf.HasAnyWorkspace() {
		t.Fatalf("MANIFESTS ADMIN must never imply a deploy capability: %+v", mf)
	}

	mgmt := resolve("u-mgmt")
	if !mgmt.FullOrganization || !mgmt.HasAnyWorkspace() {
		t.Fatalf("org-scoped WORKSPACE_MANAGEMENT WRITE satisfies WORKSPACE_RESOURCES WRITE org-wide: %+v", mgmt)
	}

	// Without a capability the plain list semantics are unchanged.
	plain, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{UserID: "u-mf", OrgID: 1})
	if err != nil || !plain.FullOrganization {
		t.Fatalf("plain list must remain organization-wide: %+v, %v", plain, err)
	}

	none := resolve("u-none")
	if none.HasAccess {
		t.Fatalf("no grant must stay unauthorized with a capability: %+v", none)
	}
}
