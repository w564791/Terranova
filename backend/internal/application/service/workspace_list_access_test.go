package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupWorkspaceListAccessDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL)`,
		`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)`,
		`CREATE TABLE workspace_project_relations (workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)`,
		`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-project-a'), (200, 'ws-project-b'), (300, 'ws-other-org')`,
		`INSERT INTO projects (id, org_id) VALUES (10, 1), (20, 1), (30, 2), (40, 1)`,
		`INSERT INTO workspace_project_relations (workspace_id, project_id) VALUES ('ws-project-a', 10), ('ws-project-b', 20), ('ws-other-org', 30)`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatalf("schema: %v\n%s", err, stmt)
		}
	}
	return db
}

func workspaceListProjectRepo(db *gorm.DB) *stubProjectRepo {
	return &stubProjectRepo{
		db:          db,
		orgByProj:   map[uint]uint{10: 1, 20: 1, 30: 2, 40: 1},
		projByWsNum: map[uint]uint{100: 10, 200: 20, 300: 30},
		wsSemToNum: map[string]uint{
			"ws-project-a": 100,
			"ws-project-b": 200,
			"ws-other-org": 300,
		},
	}
}

func TestWorkspaceListAccess_ProjectRoleOnlyListsProjectDescendants(t *testing.T) {
	db := setupWorkspaceListAccessDB(t)
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "u-project", RoleID: 7, RoleName: "project-workspace-reader",
			ScopeType: "PROJECT", ScopeID: 10, AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			7: {{
				RoleID:               7,
				PermissionID:         "workspace-management",
				PermissionLevel:      "READ",
				ScopeType:            "PROJECT",
				ResourceType:         string(valueobject.ResourceTypeWorkspaceManagement),
				PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace),
			}},
		},
	}
	checker := newTestChecker(t, perm, nil, workspaceListProjectRepo(db))
	resolver := NewWorkspaceListAccessService(db, checker)

	access, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{
		UserID: "u-project", OrgID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if access.FullOrganization || !access.HasAccess {
		t.Fatal("project Role must not become an organization-wide list grant")
	}
	if len(access.WorkspaceIDs) != 1 || access.WorkspaceIDs[0] != "ws-project-a" {
		t.Fatalf("want only project descendant, got %+v", access)
	}
}

func TestWorkspaceListAccess_WorkspaceRoleCannotListSibling(t *testing.T) {
	db := setupWorkspaceListAccessDB(t)
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "u-workspace", RoleID: 8, RoleName: "workspace-reader",
			ScopeType: "WORKSPACE", ScopeID: 100, AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			8: {{
				RoleID:               8,
				PermissionID:         "workspace-management",
				PermissionLevel:      "READ",
				ScopeType:            "WORKSPACE",
				ResourceType:         string(valueobject.ResourceTypeWorkspaceManagement),
				PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace),
			}},
		},
	}
	resolver := NewWorkspaceListAccessService(db, newTestChecker(t, perm, nil, workspaceListProjectRepo(db)))

	access, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{
		UserID: "u-workspace", OrgID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if access.FullOrganization || !access.HasAccess || len(access.WorkspaceIDs) != 1 || access.WorkspaceIDs[0] != "ws-project-a" {
		t.Fatalf("workspace Role leaked a sibling: %+v", access)
	}
}

func TestWorkspaceListAccess_OrganizationGrantKeepsFullListAndNoGrantIsEmpty(t *testing.T) {
	db := setupWorkspaceListAccessDB(t)
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{
			{
				UserID: "u-org", RoleID: 9, RoleName: "org-workspace-reader",
				ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: time.Now(),
			},
			{
				UserID: "u-org-management", RoleID: 10, RoleName: "org-management-reader",
				ScopeType: "ORGANIZATION", ScopeID: 1, AssignedAt: time.Now(),
			},
		},
		policies: map[uint][]*entity.RolePolicy{
			9: {{
				RoleID:               9,
				PermissionID:         "workspaces",
				PermissionLevel:      "READ",
				ScopeType:            "ORGANIZATION",
				ResourceType:         string(valueobject.ResourceTypeAllWorkspaces),
				PermissionScopeLevel: string(valueobject.ScopeTypeOrganization),
			}},
			10: {{
				RoleID:               10,
				PermissionID:         "workspace-management",
				PermissionLevel:      "READ",
				ScopeType:            "ORGANIZATION",
				ResourceType:         string(valueobject.ResourceTypeWorkspaceManagement),
				PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace),
			}},
		},
	}
	resolver := NewWorkspaceListAccessService(db, newTestChecker(t, perm, nil, workspaceListProjectRepo(db)))

	full, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{UserID: "u-org", OrgID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !full.FullOrganization || !full.HasAccess || full.WorkspaceIDs != nil {
		t.Fatalf("organization grant must retain unrestricted list semantics: %+v", full)
	}

	fullManagement, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{UserID: "u-org-management", OrgID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if !fullManagement.FullOrganization || !fullManagement.HasAccess || fullManagement.WorkspaceIDs != nil {
		t.Fatalf("organization-scoped workspace management must retain unrestricted list semantics: %+v", fullManagement)
	}

	none, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{UserID: "u-none", OrgID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if none.FullOrganization || none.HasAccess || len(none.WorkspaceIDs) != 0 {
		t.Fatalf("principal without grants must not receive workspace IDs: %+v", none)
	}
}

func TestWorkspaceListAccess_EmptyProjectRoleIsAuthorizedEmptyList(t *testing.T) {
	db := setupWorkspaceListAccessDB(t)
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "u-empty-project", RoleID: 11, RoleName: "empty-project-reader",
			ScopeType: "PROJECT", ScopeID: 40, AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			11: {{
				RoleID:               11,
				PermissionID:         "workspace-management",
				PermissionLevel:      "READ",
				ScopeType:            "PROJECT",
				ResourceType:         string(valueobject.ResourceTypeWorkspaceManagement),
				PermissionScopeLevel: string(valueobject.ScopeTypeWorkspace),
			}},
		},
	}
	resolver := NewWorkspaceListAccessService(db, newTestChecker(t, perm, nil, workspaceListProjectRepo(db)))

	access, err := resolver.ResolveWorkspaceListAccess(context.Background(), WorkspaceListAccessRequest{
		UserID: "u-empty-project", OrgID: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if access.FullOrganization || !access.HasAccess || len(access.WorkspaceIDs) != 0 {
		t.Fatalf("empty project Role should authorize an empty scoped list: %+v", access)
	}
}
