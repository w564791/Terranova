package service

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// --- minimal stubs for CheckPermission integration ---

type stubPermRepo struct {
	orgPerms  []*entity.OrgPermission
	projPerms []*entity.ProjectPermission
	wsPerms   []*entity.WorkspacePermission
	userRoles []*entity.UserRole
	teamRoles []*entity.UserRole
	appRoles  []*entity.UserRole
	policies  map[uint][]*entity.RolePolicy // roleID -> policies
}

func (s *stubPermRepo) QueryOrgPermissions(ctx context.Context, orgID uint, pt valueobject.PrincipalType, ids []string, rt valueobject.ResourceType) ([]*entity.OrgPermission, error) {
	var out []*entity.OrgPermission
	for _, p := range s.orgPerms {
		if p.OrgID == orgID && p.PrincipalType == pt && containsStr(ids, p.PrincipalID) {
			if p.Permission != nil && p.Permission.ResourceType == rt {
				out = append(out, p)
			} else if p.Permission == nil {
				out = append(out, p)
			}
		}
	}
	return out, nil
}
func (s *stubPermRepo) QueryProjectPermissions(ctx context.Context, projectID uint, pt valueobject.PrincipalType, ids []string, rt valueobject.ResourceType) ([]*entity.ProjectPermission, error) {
	var out []*entity.ProjectPermission
	for _, p := range s.projPerms {
		if p.ProjectID == projectID && p.PrincipalType == pt && containsStr(ids, p.PrincipalID) {
			if p.Permission == nil || p.Permission.ResourceType == rt {
				out = append(out, p)
			}
		}
	}
	return out, nil
}
func (s *stubPermRepo) QueryWorkspacePermissions(ctx context.Context, workspaceID string, pt valueobject.PrincipalType, ids []string, rt valueobject.ResourceType) ([]*entity.WorkspacePermission, error) {
	var out []*entity.WorkspacePermission
	for _, p := range s.wsPerms {
		if p.WorkspaceID == workspaceID && p.PrincipalType == pt && containsStr(ids, p.PrincipalID) {
			if p.Permission == nil || p.Permission.ResourceType == rt {
				out = append(out, p)
			}
		}
	}
	return out, nil
}
func (s *stubPermRepo) GrantOrgPermission(context.Context, *entity.OrgPermission) error {
	return nil
}
func (s *stubPermRepo) GrantProjectPermission(context.Context, *entity.ProjectPermission) error {
	return nil
}
func (s *stubPermRepo) GrantWorkspacePermission(context.Context, *entity.WorkspacePermission) error {
	return nil
}
func (s *stubPermRepo) RevokeOrgPermission(context.Context, uint) error     { return nil }
func (s *stubPermRepo) RevokeProjectPermission(context.Context, uint) error { return nil }
func (s *stubPermRepo) RevokeWorkspacePermission(context.Context, uint) error {
	return nil
}
func (s *stubPermRepo) UpdateOrgPermission(context.Context, uint, valueobject.PermissionLevel) error {
	return nil
}
func (s *stubPermRepo) UpdateProjectPermission(context.Context, uint, valueobject.PermissionLevel) error {
	return nil
}
func (s *stubPermRepo) UpdateWorkspacePermission(context.Context, uint, valueobject.PermissionLevel) error {
	return nil
}
func (s *stubPermRepo) ListPermissionsByScope(context.Context, valueobject.ScopeType, uint) ([]*entity.PermissionGrant, error) {
	return nil, nil
}
func (s *stubPermRepo) GetPermissionDefinitionByName(context.Context, string) (*entity.PermissionDefinition, error) {
	return nil, nil
}
func (s *stubPermRepo) ListPermissionDefinitions(context.Context) ([]*entity.PermissionDefinition, error) {
	return nil, nil
}
func (s *stubPermRepo) GetPresetPermissions(context.Context, string, valueobject.ScopeType) ([]*entity.PresetPermission, error) {
	return nil, nil
}
func (s *stubPermRepo) CheckTemporaryPermission(context.Context, uint, string, string, string) (*entity.TaskTemporaryPermission, error) {
	return nil, nil
}
func (s *stubPermRepo) CreateTemporaryPermission(context.Context, *entity.TaskTemporaryPermission) error {
	return nil
}
func (s *stubPermRepo) MarkTemporaryPermissionUsed(context.Context, uint) error { return nil }
func (s *stubPermRepo) QueryUserRoles(ctx context.Context, userID string, st valueobject.ScopeType, scopeID uint) ([]*entity.UserRole, error) {
	var out []*entity.UserRole
	for _, r := range s.userRoles {
		if r.UserID == userID && r.ScopeType == string(st) && r.ScopeID == scopeID {
			out = append(out, r)
		}
	}
	return out, nil
}
func (s *stubPermRepo) QueryTeamRoles(ctx context.Context, teamIDs []string, st valueobject.ScopeType, scopeID uint) ([]*entity.UserRole, error) {
	var out []*entity.UserRole
	for _, r := range s.teamRoles {
		if r.ScopeType != string(st) || r.ScopeID != scopeID {
			continue
		}
		// 测试约定：UserID 存 team_id；空 UserID 表示“任意 team”（仅兼容旧用例，新用例应显式写 team_id）
		if len(teamIDs) > 0 && r.UserID != "" && !containsStr(teamIDs, r.UserID) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *stubPermRepo) QueryApplicationRoles(ctx context.Context, applicationPrincipalIDs []string, st valueobject.ScopeType, scopeID uint) ([]*entity.UserRole, error) {
	var out []*entity.UserRole
	for _, r := range s.appRoles {
		if r.ScopeType != string(st) || r.ScopeID != scopeID {
			continue
		}
		// UserRole 是 ApplicationRole 查询的投影；测试中以 UserID 保存 app_key。
		if r.UserID != "" && !containsStr(applicationPrincipalIDs, r.UserID) {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
func (s *stubPermRepo) QueryRolePolicies(ctx context.Context, roleID uint, st valueobject.ScopeType) ([]*entity.RolePolicy, error) {
	if s.policies == nil {
		return nil, nil
	}
	var out []*entity.RolePolicy
	for _, p := range s.policies[roleID] {
		if p.ScopeType == string(st) {
			// 生产仓储会投影 PermissionDefinition.scope_level；stub 用资源类型
			// 的规范 scope 模拟该投影，单个用例可显式填入异常值验证 fail-closed。
			if p.PermissionScopeLevel == "" {
				p.PermissionScopeLevel = string(valueobject.ResourceType(p.ResourceType).GetScopeLevel())
			}
			out = append(out, p)
		}
	}
	return out, nil
}
func (s *stubPermRepo) GetPermissionDefinition(context.Context, uint) (*entity.PermissionDefinition, error) {
	return nil, nil
}
func (s *stubPermRepo) ListPermissionsByPrincipal(context.Context, valueobject.PrincipalType, string) ([]*entity.PermissionGrant, error) {
	return nil, nil
}

type stubTeamRepo struct {
	teams []string
}

func (s *stubTeamRepo) CreateTeam(context.Context, *entity.Team) error { return nil }
func (s *stubTeamRepo) GetTeamByID(context.Context, string) (*entity.Team, error) {
	return nil, nil
}
func (s *stubTeamRepo) GetTeamByName(context.Context, uint, string) (*entity.Team, error) {
	return nil, nil
}
func (s *stubTeamRepo) ListTeamsByOrg(context.Context, uint) ([]*entity.Team, error) { return nil, nil }
func (s *stubTeamRepo) UpdateTeam(context.Context, *entity.Team) error               { return nil }
func (s *stubTeamRepo) DeleteTeam(context.Context, string) error                     { return nil }
func (s *stubTeamRepo) AddMember(context.Context, *entity.TeamMember) error          { return nil }
func (s *stubTeamRepo) AddMemberWithOrganizationMembership(context.Context, *entity.TeamMember, uint) error {
	return nil
}
func (s *stubTeamRepo) RemoveMember(context.Context, string, string) error { return nil }
func (s *stubTeamRepo) UpdateMemberRole(context.Context, string, string, entity.TeamRole) error {
	return nil
}
func (s *stubTeamRepo) ListMembers(context.Context, string) ([]*entity.TeamMember, error) {
	return nil, nil
}
func (s *stubTeamRepo) GetUserTeams(context.Context, string) ([]string, error) {
	return s.teams, nil
}
func (s *stubTeamRepo) GetUserTeamsInOrg(context.Context, string, uint) ([]*entity.Team, error) {
	return nil, nil
}
func (s *stubTeamRepo) IsMember(context.Context, string, string) (bool, error)     { return false, nil }
func (s *stubTeamRepo) IsMaintainer(context.Context, string, string) (bool, error) { return false, nil }
func (s *stubTeamRepo) BatchGetUserTeams(context.Context, []string) (map[string][]string, error) {
	return nil, nil
}

type stubProjectRepo struct {
	db          *gorm.DB
	orgByProj   map[uint]uint
	projByWsNum map[uint]uint
	wsSemToNum  map[string]uint
}

func (s *stubProjectRepo) CreateProject(context.Context, *entity.Project) error { return nil }
func (s *stubProjectRepo) GetProjectByID(context.Context, uint) (*entity.Project, error) {
	return nil, nil
}
func (s *stubProjectRepo) GetProjectByName(context.Context, uint, string) (*entity.Project, error) {
	return nil, nil
}
func (s *stubProjectRepo) ListProjectsByOrg(context.Context, uint, *bool) ([]*entity.Project, error) {
	return nil, nil
}
func (s *stubProjectRepo) GetDefaultProject(context.Context, uint) (*entity.Project, error) {
	return nil, nil
}
func (s *stubProjectRepo) UpdateProject(context.Context, *entity.Project) error { return nil }
func (s *stubProjectRepo) DeleteProject(context.Context, uint) error            { return nil }
func (s *stubProjectRepo) GetOrgIDByProjectID(ctx context.Context, projectID uint) (uint, error) {
	return s.orgByProj[projectID], nil
}
func (s *stubProjectRepo) ListWorkspacesByProject(context.Context, uint) ([]string, error) {
	return nil, nil
}
func (s *stubProjectRepo) GetProjectByWorkspaceID(context.Context, string) (*entity.Project, error) {
	return nil, nil
}
func (s *stubProjectRepo) AssignWorkspaceToProject(context.Context, string, uint) error { return nil }
func (s *stubProjectRepo) RemoveWorkspaceFromProject(context.Context, string) error     { return nil }
func (s *stubProjectRepo) GetWorkspaceProjectRelation(context.Context, string) (*entity.WorkspaceProjectRelation, error) {
	return nil, nil
}
func (s *stubProjectRepo) GetDB() *gorm.DB { return s.db }
func (s *stubProjectRepo) GetWorkspaceIDBySemanticID(ctx context.Context, semanticID string) (uint, error) {
	return s.wsSemToNum[semanticID], nil
}
func (s *stubProjectRepo) GetProjectIDByWorkspaceID(ctx context.Context, workspaceID uint) (uint, error) {
	return s.projByWsNum[workspaceID], nil
}

type stubAuditRepo struct{}

func (s *stubAuditRepo) LogPermissionChange(context.Context, *entity.PermissionAuditLog) error {
	return nil
}
func (s *stubAuditRepo) LogResourceAccess(context.Context, *entity.AccessLog) error { return nil }
func (s *stubAuditRepo) QueryPermissionHistory(context.Context, valueobject.ScopeType, uint, time.Time, time.Time, int) ([]*entity.PermissionAuditLog, error) {
	return nil, nil
}
func (s *stubAuditRepo) QueryAccessHistory(context.Context, string, string, string, string, int, time.Time, time.Time, int) ([]*entity.AccessLog, error) {
	return nil, nil
}
func (s *stubAuditRepo) QueryDeniedAccess(context.Context, time.Time, time.Time, int) ([]*entity.AccessLog, error) {
	return nil, nil
}
func (s *stubAuditRepo) QueryPermissionChangesByPrincipal(context.Context, valueobject.PrincipalType, string, time.Time, time.Time, int) ([]*entity.PermissionAuditLog, error) {
	return nil, nil
}
func (s *stubAuditRepo) QueryPermissionChangesByPerformer(context.Context, string, time.Time, time.Time, int) ([]*entity.PermissionAuditLog, error) {
	return nil, nil
}

func containsStr(ids []string, id string) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func newTestChecker(t *testing.T, perm *stubPermRepo, teams []string, proj *stubProjectRepo) *PermissionCheckerImpl {
	t.Helper()
	if proj == nil {
		proj = &stubProjectRepo{
			orgByProj:   map[uint]uint{10: 1},
			projByWsNum: map[uint]uint{100: 10},
			wsSemToNum:  map[string]uint{"ws-1": 100},
		}
	}
	if perm == nil {
		perm = &stubPermRepo{}
	}
	return &PermissionCheckerImpl{
		permissionRepo: perm,
		teamRepo:       &stubTeamRepo{teams: teams},
		projectRepo:    proj,
		auditRepo:      &stubAuditRepo{},
	}
}

func TestCheckPermission_NoGrantsDenied(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID:        "user-1",
		ResourceType:  valueobject.ResourceTypeAllWorkspaces,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelNone {
		t.Fatalf("expected deny none: %+v", res)
	}
	if res.DenyReason != "No permission" {
		t.Fatalf("deny reason: %q", res.DenyReason)
	}
}

func TestCheckPermission_OrgWriteAllows(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID:           1,
			PrincipalType:   valueobject.PrincipalTypeUser,
			PrincipalID:     "user-1",
			PermissionLevel: valueobject.PermissionLevelWrite,
			Permission: &entity.PermissionDefinition{
				ResourceType: valueobject.ResourceTypeAllWorkspaces,
			},
			GrantedAt: time.Now(),
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID:        "user-1",
		ResourceType:  valueobject.ResourceTypeAllWorkspaces,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       1,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("expected allow write: %+v", res)
	}
}

func TestCheckPermission_WorkspaceReadBeatsOrgWrite(t *testing.T) {
	// Direct grants at org WRITE + ws READ for same resource type
	// Need workspace collection which hits GetDB — use sqlite for workspace_id lookup
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
			PermissionLevel: valueobject.PermissionLevelWrite,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
			GrantedAt:       time.Now(),
		}},
		wsPerms: []*entity.WorkspacePermission{{
			WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
			PermissionLevel: valueobject.PermissionLevelRead,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
			GrantedAt:       time.Now(),
		}},
	}
	proj := &stubProjectRepo{
		db:          db,
		orgByProj:   map[uint]uint{10: 1},
		projByWsNum: map[uint]uint{100: 10},
		wsSemToNum:  map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID:        "user-1",
		ResourceType:  valueobject.ResourceTypeWorkspaceManagement,
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeID:       100,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	// WS layer READ wins → insufficient for WRITE
	if res.IsAllowed {
		t.Fatalf("expected deny: ws READ should beat org WRITE, got %+v", res)
	}
	if res.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("effective want READ got %s", res.EffectiveLevel)
	}
}

func TestCheckPermission_ProjectManagementAllowsFineWorkspaceResource(t *testing.T) {
	// A MANAGEMENT grant at a project's assignment scope must cover ordinary
	// workspace operations below that project.
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{projPerms: []*entity.ProjectPermission{{
		ProjectID:       10,
		PrincipalType:   valueobject.PrincipalTypeUser,
		PrincipalID:     "user-1",
		PermissionLevel: valueobject.PermissionLevelWrite,
		Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
		GrantedAt:       time.Now(),
	}}}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("project MANAGEMENT WRITE should cover workspace execution WRITE: %+v", res)
	}
	admin, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if admin.IsAllowed || admin.EffectiveLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("MANAGEMENT WRITE must not satisfy workspace execution ADMIN: %+v", admin)
	}
}

func TestCheckPermission_OrganizationManagementRoleAllowsFineWorkspaceResource(t *testing.T) {
	// The implication must work for Role policies too, including a Role assigned
	// to the target workspace's ancestor organization.
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "user-1", RoleID: 21, RoleName: "org-workspace-manager",
			ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1, AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			21: {{
				RoleID: 21, PermissionID: "workspace-management", PermissionLevel: "WRITE",
				ScopeType: string(valueobject.ScopeTypeOrganization), ResourceType: string(valueobject.ResourceTypeWorkspaceManagement),
			}},
		},
	}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceVars,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("organization MANAGEMENT Role should cover workspace variables WRITE: %+v", res)
	}
}

func TestCheckPermission_FineGrantWinsOverSameScopeManagement(t *testing.T) {
	// An explicit fine-grained grant is a constraint at its assignment layer;
	// MANAGEMENT is only its fallback and must not silently elevate it.
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{wsPerms: []*entity.WorkspacePermission{
		{
			WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
			PermissionLevel: valueobject.PermissionLevelRead,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceExec},
			GrantedAt:       time.Now(),
		},
		{
			WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
			PermissionLevel: valueobject.PermissionLevelWrite,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
			GrantedAt:       time.Now(),
		},
	}}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("same-scope fine READ must win over MANAGEMENT WRITE: %+v", res)
	}
}

func TestCheckPermission_FineGrantDoesNotImplyManagement(t *testing.T) {
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{wsPerms: []*entity.WorkspacePermission{{
		WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
		PermissionLevel: valueobject.PermissionLevelAdmin,
		Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceExec},
		GrantedAt:       time.Now(),
	}}}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceManagement,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelNone {
		t.Fatalf("fine-grained execution grant must not imply MANAGEMENT: %+v", res)
	}
}

func TestCheckPermission_ManagementReadDoesNotSatisfySensitiveState(t *testing.T) {
	// State content is intentionally outside the generic implication. Its
	// routes retain the explicit SENSITIVE READ or MANAGEMENT ADMIN policy.
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}

	perm := &stubPermRepo{wsPerms: []*entity.WorkspacePermission{{
		WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
		PermissionLevel: valueobject.PermissionLevelRead,
		Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
		GrantedAt:       time.Now(),
	}}}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceStateSensitive,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelNone {
		t.Fatalf("MANAGEMENT READ must not expose sensitive state content: %+v", res)
	}
}

func TestCheckPermission_TeamGrant(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-a",
			PermissionLevel: valueobject.PermissionLevelAdmin,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
			GrantedAt:       time.Now(),
		}},
	}
	c := newTestChecker(t, perm, []string{"team-a"}, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID:        "user-1",
		ResourceType:  valueobject.ResourceTypeModules,
		ScopeType:     valueobject.ScopeTypeOrganization,
		ScopeID:       1,
		RequiredLevel: valueobject.PermissionLevelAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("team grant should allow: %+v", res)
	}
}

func TestCheckPermission_ExpiredGrant(t *testing.T) {
	past := time.Now().Add(-time.Hour)
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
			PermissionLevel: valueobject.PermissionLevelAdmin,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
			GrantedAt:       time.Now().Add(-2 * time.Hour),
			ExpiresAt:       &past,
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed {
		t.Fatal("expired grant must deny")
	}
}

func TestCheckPermission_RoleAtOrg(t *testing.T) {
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "user-1", RoleID: 7, RoleName: "viewer",
			ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1,
			AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			7: {{
				RoleID: 7, PermissionID: "p1",
				PermissionLevel: "READ",
				ScopeType:       string(valueobject.ScopeTypeOrganization),
				ResourceType:    string(valueobject.ResourceTypeModules),
			}},
		},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("role grant: %+v", res)
	}
}

func TestCheckPermission_OrganizationRoleCanCoverWorkspaceResource(t *testing.T) {
	// The policy's scope is the Role-assignment layer. An organization Role
	// containing a workspace resource is the documented way to cover every
	// workspace in that organization.
	db := openMemDB(t)
	if err := db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`).Error; err != nil {
		t.Fatal(err)
	}
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "user-1", RoleID: 18, RoleName: "org-operator",
			ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1,
			AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			18: {{
				RoleID: 18, PermissionID: "workspace-exec", PermissionLevel: "WRITE",
				ScopeType: string(valueobject.ScopeTypeOrganization), ResourceType: string(valueobject.ResourceTypeWorkspaceExec),
			}},
		},
	}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}, wsSemToNum: map[string]uint{"ws-1": 100},
	}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelWrite {
		t.Fatalf("organization Role should cover workspace execution: %+v", res)
	}
}

func TestCheckPermission_RolePolicyScopeMismatchIsIgnored(t *testing.T) {
	// 历史错误数据可能把组织级 MODULES permission 对应的 definition scope
	// 错配成 PROJECT；求值必须 fail-closed，不能因 role assignment 在组织级
	// 就把它放大为组织授权。
	perm := &stubPermRepo{
		userRoles: []*entity.UserRole{{
			UserID: "user-1", RoleID: 17, RoleName: "broken-policy-role",
			ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1,
			AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			17: {{
				RoleID: 17, PermissionID: "modules-wrong-scope", PermissionLevel: "ADMIN",
				ScopeType: string(valueobject.ScopeTypeOrganization), ResourceType: string(valueobject.ResourceTypeModules),
				PermissionScopeLevel: string(valueobject.ScopeTypeProject),
			}},
		},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "user-1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelNone {
		t.Fatalf("scope-mismatched policy must not grant: %+v", res)
	}
}

func TestCheckPermission_ApplicationRoleAtOrgAllowsAndIgnoresIllegalNarrowRole(t *testing.T) {
	// Application 的合法组织级 Role 应当在其组织内的 workspace 请求上生效；
	// 迁移前/手工写入的 workspace 级 Application Role 必须被忽略，不能提升权限。
	perm := &stubPermRepo{
		appRoles: []*entity.UserRole{
			{
				UserID: "app-key-1", RoleID: 9, RoleName: "app-reader",
				ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1,
				AssignedAt: time.Now(),
			},
			{
				UserID: "app-key-1", RoleID: 10, RoleName: "illegal-workspace-admin",
				ScopeType: string(valueobject.ScopeTypeWorkspace), ScopeID: 100,
				AssignedAt: time.Now(),
			},
		},
		policies: map[uint][]*entity.RolePolicy{
			9: {{
				RoleID: 9, PermissionID: "modules-read", PermissionLevel: "READ",
				ScopeType: string(valueobject.ScopeTypeOrganization), ResourceType: string(valueobject.ResourceTypeModules),
			}},
			10: {{
				RoleID: 10, PermissionID: "modules-admin", PermissionLevel: "ADMIN",
				ScopeType: string(valueobject.ScopeTypeWorkspace), ResourceType: string(valueobject.ResourceTypeModules),
			}},
		},
	}
	c := newTestChecker(t, perm, nil, nil)

	read, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeApplication,
		PrincipalID:   "app-key-1",
		ResourceType:  valueobject.ResourceTypeModules,
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeID:       100,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !read.IsAllowed || read.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("organization application role should grant READ: %+v", read)
	}

	write, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeApplication,
		PrincipalID:   "app-key-1",
		ResourceType:  valueobject.ResourceTypeModules,
		ScopeType:     valueobject.ScopeTypeWorkspace,
		ScopeID:       100,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if write.IsAllowed || write.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("illegal narrow application role must not be evaluated: %+v", write)
	}
}

func TestCheckPermission_ValidateMissingUser(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	_, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestCheckPermission_TeamPrincipalUsesTeamGrantsOnly(t *testing.T) {
	// user personal grant must NOT apply when principal is TEAM
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{
			{
				OrgID: 1, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "user-1",
				PermissionLevel: valueobject.PermissionLevelAdmin,
				Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
				GrantedAt:       time.Now(),
			},
			{
				OrgID: 1, PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-99",
				PermissionLevel: valueobject.PermissionLevelRead,
				Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
				GrantedAt:       time.Now(),
			},
		},
	}
	c := newTestChecker(t, perm, nil, nil)

	// TEAM principal → only team READ
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "team:team-99", PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-99",
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.EffectiveLevel != valueobject.PermissionLevelRead {
		t.Fatalf("team principal: %+v", res)
	}
	// TEAM cannot use user ADMIN for WRITE
	res2, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-99",
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.IsAllowed {
		t.Fatal("team must not inherit user personal ADMIN")
	}
}

func TestCheckPermission_TeamRoleGrant(t *testing.T) {
	perm := &stubPermRepo{
		teamRoles: []*entity.UserRole{{
			UserID: "team-1", RoleID: 8, RoleName: "dev",
			ScopeType: string(valueobject.ScopeTypeOrganization), ScopeID: 1,
			AssignedAt: time.Now(),
		}},
		policies: map[uint][]*entity.RolePolicy{
			8: {{
				RoleID: 8, PermissionID: "p1", PermissionLevel: "WRITE",
				ScopeType:    string(valueobject.ScopeTypeOrganization),
				ResourceType: string(valueobject.ResourceTypeModules),
			}},
		},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-1",
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("team role should allow write: %+v", res)
	}

	// 非成员 team 不得继承该 role
	res2, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-other",
		ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res2.IsAllowed {
		t.Fatal("other team must not inherit team-1 role")
	}
}

func openMemDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}
