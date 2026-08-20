package service

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"
)

// staticEmailLookup 测试用邮箱解析
type staticEmailLookup map[string]string

func (m staticEmailLookup) GetUserEmail(ctx context.Context, userID string) (string, error) {
	if e, ok := m[userID]; ok && e != "" {
		return e, nil
	}
	return "", fmt.Errorf("no email for %s", userID)
}

// tempAwareRepo adds temporary permission + list helpers on top of stubPermRepo
type tempAwareRepo struct {
	stubPermRepo
	temp       *entity.TaskTemporaryPermission
	tempMarked bool
	presets    []*entity.PresetPermission
	defs       []*entity.PermissionDefinition
	byScope    []*entity.PermissionGrant
	byPrincipal []*entity.PermissionGrant
	revoked    []string
	updated    []string
}

func (t *tempAwareRepo) CheckTemporaryPermission(ctx context.Context, taskID uint, email, userID, permType string) (*entity.TaskTemporaryPermission, error) {
	if t.temp == nil || t.temp.TaskID != taskID || t.temp.PermissionType != permType {
		return nil, nil
	}
	// 双键：email 或 user_id 任一匹配
	if email != "" && t.temp.UserEmail != "" && !equalFoldEmail(email, t.temp.UserEmail) {
		if userID == "" || t.temp.UserID == "" || userID != t.temp.UserID {
			// 若 temp 未设置 email/user_id 约束则放行（兼容旧单测）
			if t.temp.UserEmail != "" || t.temp.UserID != "" {
				return nil, nil
			}
		}
	}
	if userID != "" && t.temp.UserID != "" && userID != t.temp.UserID {
		if email == "" || t.temp.UserEmail == "" || !equalFoldEmail(email, t.temp.UserEmail) {
			return nil, nil
		}
	}
	return t.temp, nil
}

func equalFoldEmail(a, b string) bool {
	return len(a) == len(b) && (a == b || strings.EqualFold(a, b))
}
func (t *tempAwareRepo) MarkTemporaryPermissionUsed(ctx context.Context, id uint) error {
	t.tempMarked = true
	if t.temp != nil {
		t.temp.IsUsed = true
	}
	return nil
}
func (t *tempAwareRepo) GetPresetPermissions(ctx context.Context, name string, scope valueobject.ScopeType) ([]*entity.PresetPermission, error) {
	return t.presets, nil
}
func (t *tempAwareRepo) ListPermissionDefinitions(context.Context) ([]*entity.PermissionDefinition, error) {
	return t.defs, nil
}
func (t *tempAwareRepo) ListPermissionsByScope(context.Context, valueobject.ScopeType, uint) ([]*entity.PermissionGrant, error) {
	return t.byScope, nil
}
func (t *tempAwareRepo) ListPermissionsByPrincipal(context.Context, valueobject.PrincipalType, string) ([]*entity.PermissionGrant, error) {
	return t.byPrincipal, nil
}
func (t *tempAwareRepo) GetPermissionDefinitionByName(ctx context.Context, name string) (*entity.PermissionDefinition, error) {
	for _, d := range t.defs {
		if d.ID == name || d.Name == name {
			return d, nil
		}
	}
	return nil, nil
}
func (t *tempAwareRepo) RevokeOrgPermission(ctx context.Context, id uint) error {
	t.revoked = append(t.revoked, "org")
	return nil
}
func (t *tempAwareRepo) RevokeProjectPermission(ctx context.Context, id uint) error {
	t.revoked = append(t.revoked, "proj")
	return nil
}
func (t *tempAwareRepo) RevokeWorkspacePermission(ctx context.Context, id uint) error {
	t.revoked = append(t.revoked, "ws")
	return nil
}
func (t *tempAwareRepo) UpdateOrgPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	t.updated = append(t.updated, "org")
	return nil
}
func (t *tempAwareRepo) UpdateProjectPermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	t.updated = append(t.updated, "proj")
	return nil
}
func (t *tempAwareRepo) UpdateWorkspacePermission(ctx context.Context, id uint, level valueobject.PermissionLevel) error {
	t.updated = append(t.updated, "ws")
	return nil
}
func (t *tempAwareRepo) GrantProjectPermission(ctx context.Context, p *entity.ProjectPermission) error {
	return nil
}

func TestCheckBatchPermissions(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
			PermissionLevel: valueobject.PermissionLevelRead,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
			GrantedAt:       time.Now(),
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	results, err := c.CheckBatchPermissions(context.Background(), []*CheckPermissionRequest{
		{UserID: "u1", ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelRead},
		{UserID: "u1", ResourceType: valueobject.ResourceTypeModules, ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelAdmin},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || !results[0].IsAllowed || results[1].IsAllowed {
		t.Fatalf("batch results: %+v", results)
	}
}

func TestCheckPermissionWithTemporary_RegularAllows(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 1, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
			PermissionLevel: valueobject.PermissionLevelWrite,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceExec},
			GrantedAt:       time.Now(),
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	tid := uint(9)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelWrite,
	}, &tid)
	if err != nil || !res.IsAllowed || res.Source != "regular" {
		t.Fatalf("expected regular allow: %+v err=%v", res, err)
	}
}

func TestCheckPermissionWithTemporary_TempAllows(t *testing.T) {
	repo := &tempAwareRepo{
		temp: &entity.TaskTemporaryPermission{
			ID: 1, TaskID: 42, PermissionType: "APPLY",
			ExpiresAt: time.Now().Add(time.Hour), IsUsed: false,
		},
	}
	c := newTestChecker(t, &repo.stubPermRepo, nil, nil)
	// rewire - newTestChecker only takes stubPermRepo. Use PermissionCheckerImpl directly.
	c = &PermissionCheckerImpl{
		permissionRepo: repo,
		teamRepo:       &stubTeamRepo{},
		projectRepo:    &stubProjectRepo{orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10}},
		auditRepo:      &stubAuditRepo{},
		userEmails:     staticEmailLookup{"u1": "alice@example.com"},
	}
	tid := uint(42)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeWorkspaceExec,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelWrite,
	}, &tid)
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed || res.Source != "temporary" {
		t.Fatalf("expected temporary allow: %+v", res)
	}
	if !repo.tempMarked {
		t.Fatal("temp perm should be marked used")
	}
}

func TestCheckPermissionWithTemporary_NoTempStillDeny(t *testing.T) {
	c := newTestChecker(t, nil, nil, nil)
	tid := uint(1)
	res, err := c.CheckPermissionWithTemporary(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1, RequiredLevel: valueobject.PermissionLevelRead,
	}, &tid)
	if err != nil {
		t.Fatal(err)
	}
	if res.IsAllowed {
		t.Fatal("should deny")
	}
}

func TestCheckPermission_ProjectLevelGrant(t *testing.T) {
	perm := &stubPermRepo{
		projPerms: []*entity.ProjectPermission{{
			ProjectID: 10, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
			PermissionLevel: valueobject.PermissionLevelAdmin,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeProjectSettings},
			GrantedAt:       time.Now(),
		}},
	}
	proj := &stubProjectRepo{orgByProj: map[uint]uint{10: 1}}
	c := newTestChecker(t, perm, nil, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeProjectSettings,
		ScopeType: valueobject.ScopeTypeProject, ScopeID: 10, RequiredLevel: valueobject.PermissionLevelAdmin,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("project grant: %+v", res)
	}
}

func TestCheckPermission_WorkspaceTeamGrant(t *testing.T) {
	db := openMemDB(t)
	_ = db.Exec(`CREATE TABLE workspaces (id INTEGER PRIMARY KEY, workspace_id TEXT)`)
	_ = db.Exec(`INSERT INTO workspaces (id, workspace_id) VALUES (100, 'ws-1')`)
	perm := &stubPermRepo{
		wsPerms: []*entity.WorkspacePermission{{
			WorkspaceID: "ws-1", PrincipalType: valueobject.PrincipalTypeTeam, PrincipalID: "team-a",
			PermissionLevel: valueobject.PermissionLevelWrite,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeWorkspaceManagement},
			GrantedAt:       time.Now(),
		}},
	}
	proj := &stubProjectRepo{
		db: db, orgByProj: map[uint]uint{10: 1}, projByWsNum: map[uint]uint{100: 10},
	}
	c := newTestChecker(t, perm, []string{"team-a"}, proj)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeWorkspaceManagement,
		ScopeType: valueobject.ScopeTypeWorkspace, ScopeID: 100, RequiredLevel: valueobject.PermissionLevelWrite,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("ws team grant: %+v", res)
	}
}

func TestCheckPermission_NumericScopeIDStr(t *testing.T) {
	perm := &stubPermRepo{
		orgPerms: []*entity.OrgPermission{{
			OrgID: 5, PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
			PermissionLevel: valueobject.PermissionLevelRead,
			Permission:      &entity.PermissionDefinition{ResourceType: valueobject.ResourceTypeModules},
			GrantedAt:       time.Now(),
		}},
	}
	c := newTestChecker(t, perm, nil, nil)
	res, err := c.CheckPermission(context.Background(), &CheckPermissionRequest{
		UserID: "u1", ResourceType: valueobject.ResourceTypeModules,
		ScopeType: valueobject.ScopeTypeOrganization, ScopeIDStr: "5", RequiredLevel: valueobject.PermissionLevelRead,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !res.IsAllowed {
		t.Fatalf("numeric scope id str: %+v", res)
	}
}

func TestPermissionService_RevokeModifyListPreset(t *testing.T) {
	repo := &tempAwareRepo{
		presets: []*entity.PresetPermission{
			{PermissionID: "p1", PermissionLevel: valueobject.PermissionLevelRead},
			{PermissionID: "p2", PermissionLevel: valueobject.PermissionLevelWrite},
		},
		defs: []*entity.PermissionDefinition{{ID: "p1", Name: "P1"}},
		byScope: []*entity.PermissionGrant{{PermissionID: "p1"}},
		byPrincipal: []*entity.PermissionGrant{{PermissionID: "p1"}},
	}
	s := &PermissionServiceImpl{permissionRepo: repo, auditRepo: &stubAuditRepo{}}

	// revoke all scopes
	for _, st := range []valueobject.ScopeType{
		valueobject.ScopeTypeOrganization, valueobject.ScopeTypeProject, valueobject.ScopeTypeWorkspace,
	} {
		if err := s.RevokePermission(context.Background(), &RevokePermissionRequest{
			ScopeType: st, AssignmentID: 1, RevokedBy: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.revoked) != 3 {
		t.Fatalf("revoked: %v", repo.revoked)
	}
	if err := s.RevokePermission(context.Background(), &RevokePermissionRequest{
		ScopeType: "NOPE", AssignmentID: 1, RevokedBy: "a",
	}); err == nil {
		t.Fatal("bad scope revoke")
	}

	// modify all scopes
	for _, st := range []valueobject.ScopeType{
		valueobject.ScopeTypeOrganization, valueobject.ScopeTypeProject, valueobject.ScopeTypeWorkspace,
	} {
		if err := s.ModifyPermission(context.Background(), &ModifyPermissionRequest{
			ScopeType: st, AssignmentID: 1, NewLevel: valueobject.PermissionLevelAdmin, ModifiedBy: "admin",
		}); err != nil {
			t.Fatal(err)
		}
	}
	if len(repo.updated) != 3 {
		t.Fatalf("updated: %v", repo.updated)
	}

	// preset
	if err := s.GrantPresetPermissions(context.Background(), &GrantPresetRequest{
		ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PresetName: "READ", GrantedBy: "admin",
	}); err != nil {
		t.Fatal(err)
	}
	if err := s.GrantPresetPermissions(context.Background(), &GrantPresetRequest{
		PresetName: "INVALID", ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1", GrantedBy: "a",
	}); err == nil {
		t.Fatal("invalid preset")
	}

	// empty preset
	repo.presets = nil
	if err := s.GrantPresetPermissions(context.Background(), &GrantPresetRequest{
		PresetName: "WRITE", ScopeType: valueobject.ScopeTypeOrganization, ScopeID: 1,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1", GrantedBy: "a",
	}); err == nil {
		t.Fatal("empty preset")
	}

	// list helpers
	if g, err := s.ListPermissions(context.Background(), valueobject.ScopeTypeOrganization, 1); err != nil || len(g) != 1 {
		t.Fatalf("list scope: %v %v", g, err)
	}
	if d, err := s.ListPermissionDefinitions(context.Background()); err != nil || len(d) != 1 {
		t.Fatalf("list defs: %v %v", d, err)
	}
	if g, err := s.ListPermissionsByPrincipal(context.Background(), valueobject.PrincipalTypeUser, "u1"); err != nil || len(g) != 1 {
		t.Fatalf("list principal: %v %v", g, err)
	}
	if def, err := s.GetPermissionDefinitionByID(context.Background(), "p1"); err != nil || def == nil {
		t.Fatalf("get def: %v", err)
	}
}

func TestPermissionService_GrantProject(t *testing.T) {
	repo := &tempAwareRepo{}
	s := &PermissionServiceImpl{permissionRepo: repo, auditRepo: &stubAuditRepo{}}
	if err := s.GrantPermission(context.Background(), &GrantPermissionRequest{
		ScopeType: valueobject.ScopeTypeProject, ScopeID: 3,
		PrincipalType: valueobject.PrincipalTypeUser, PrincipalID: "u1",
		PermissionID: "pjpm-1", PermissionLevel: valueobject.PermissionLevelWrite, GrantedBy: "a",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMapResourceToPermType(t *testing.T) {
	c := &PermissionCheckerImpl{}
	if c.mapResourceToPermType(valueobject.ResourceTypeWorkspaceExec) != "APPLY" {
		t.Fatal("exec -> APPLY")
	}
	if c.mapResourceToPermType(valueobject.ResourceTypeModules) != "" {
		t.Fatal("modules no temp")
	}
}
