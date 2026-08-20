package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"iac-platform/internal/domain/entity"
	"iac-platform/internal/domain/valueobject"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// levelMapChecker returns fixed effective levels per resource type
type levelMapChecker struct {
	levels map[valueobject.ResourceType]valueobject.PermissionLevel
	err    error
	calls  int
}

func (m *levelMapChecker) CheckPermission(ctx context.Context, req *CheckPermissionRequest) (*CheckPermissionResult, error) {
	m.calls++
	if m.err != nil {
		return nil, m.err
	}
	have := valueobject.PermissionLevelNone
	if m.levels != nil {
		if lv, ok := m.levels[req.ResourceType]; ok {
			have = lv
		}
	}
	allowed := have != valueobject.PermissionLevelNone && have >= req.RequiredLevel
	return &CheckPermissionResult{
		IsAllowed:      allowed,
		EffectiveLevel: have,
		DenyReason:     getDenyForTest(have, req.RequiredLevel),
	}, nil
}

func getDenyForTest(have, need valueobject.PermissionLevel) string {
	if have == valueobject.PermissionLevelNone {
		return "No permission"
	}
	if have < need {
		return "Insufficient"
	}
	return ""
}

func (m *levelMapChecker) CheckPermissionWithTemporary(ctx context.Context, req *CheckPermissionRequest, taskID *uint) (*CheckPermissionResult, error) {
	return m.CheckPermission(ctx, req)
}
func (m *levelMapChecker) CheckBatchPermissions(ctx context.Context, reqs []*CheckPermissionRequest) ([]*CheckPermissionResult, error) {
	return nil, nil
}
func (m *levelMapChecker) GetUserTeams(ctx context.Context, userID string) ([]string, error) {
	return nil, nil
}

func setupAntiEscDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&entity.Role{}, &entity.RolePolicy{}, &entity.PermissionDefinition{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func seedRoleWithPolicies(t *testing.T, db *gorm.DB, name string, isSystem bool, policies []struct {
	permID, level, rt string
}) uint {
	t.Helper()
	role := &entity.Role{Name: name, DisplayName: name, IsSystem: isSystem, IsActive: true}
	if err := db.Create(role).Error; err != nil {
		t.Fatal(err)
	}
	for _, p := range policies {
		resourceType := valueobject.ResourceType(p.rt)
		_ = db.Where("id = ?", p.permID).FirstOrCreate(&entity.PermissionDefinition{
			ID: p.permID, Name: p.permID, ResourceType: resourceType,
			ScopeLevel: resourceType.GetScopeLevel(), DisplayName: p.permID, IsSystem: true,
		})
		if err := db.Create(&entity.RolePolicy{
			RoleID: role.ID, PermissionID: p.permID, PermissionLevel: p.level, ScopeType: string(resourceType.GetScopeLevel()),
		}).Error; err != nil {
			t.Fatal(err)
		}
	}
	return role.ID
}

func TestEnsureCanAssignRole_SubsetAllows(t *testing.T) {
	db := setupAntiEscDB(t)
	roleID := seedRoleWithPolicies(t, db, "ws-reader", false, []struct{ permID, level, rt string }{
		{"perm-ws", "READ", "WORKSPACES"},
		{"perm-mod", "READ", "MODULES"},
	})
	chk := &levelMapChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceType("WORKSPACES"): valueobject.PermissionLevelWrite,
		valueobject.ResourceTypeModules:        valueobject.PermissionLevelAdmin,
	}}
	svc := NewRoleAntiEscalationService(db, chk)
	err := svc.EnsureCanAssignRole(context.Background(), "actor-1", false, roleID,
		valueobject.ScopeTypeOrganization, 1)
	if err != nil {
		t.Fatalf("expected allow: %v", err)
	}
	if chk.calls != 2 {
		t.Fatalf("expected 2 checks, got %d", chk.calls)
	}
}

func TestEnsureCanAssignRole_EscalationDenied(t *testing.T) {
	db := setupAntiEscDB(t)
	roleID := seedRoleWithPolicies(t, db, "ws-admin", false, []struct{ permID, level, rt string }{
		{"perm-ws-a", "ADMIN", "WORKSPACES"},
	})
	// actor only has READ
	chk := &levelMapChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceType("WORKSPACES"): valueobject.PermissionLevelRead,
	}}
	svc := NewRoleAntiEscalationService(db, chk)
	err := svc.EnsureCanAssignRole(context.Background(), "actor-1", false, roleID,
		valueobject.ScopeTypeOrganization, 1)
	if !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("want ErrPrivilegeEscalation, got %v", err)
	}
	if !strings.Contains(err.Error(), "WORKSPACES") {
		t.Fatalf("error should mention resource: %v", err)
	}
}

func TestEnsureCanAssignRole_SystemAdminRoleRestricted(t *testing.T) {
	db := setupAntiEscDB(t)
	roleID := seedRoleWithPolicies(t, db, "admin", true, []struct{ permID, level, rt string }{
		{"perm-iam", "ADMIN", "IAM_USERS"},
	})
	// even with full checker allow, non-system-admin cannot assign admin
	chk := &levelMapChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceType("IAM_USERS"): valueobject.PermissionLevelAdmin,
	}}
	svc := NewRoleAntiEscalationService(db, chk)
	err := svc.EnsureCanAssignRole(context.Background(), "actor-1", false, roleID,
		valueobject.ScopeTypeOrganization, 1)
	if !errors.Is(err, ErrSystemRoleRestricted) {
		t.Fatalf("want system restricted, got %v", err)
	}
	// system admin may assign without policy subset check
	if err := svc.EnsureCanAssignRole(context.Background(), "super", true, roleID,
		valueobject.ScopeTypeOrganization, 1); err != nil {
		t.Fatalf("system admin should assign admin role: %v", err)
	}
}

func TestEnsureCanAssignRole_EmptyPoliciesDeniedForNonAdmin(t *testing.T) {
	db := setupAntiEscDB(t)
	role := &entity.Role{Name: "empty", DisplayName: "E", IsActive: true}
	_ = db.Create(role)
	svc := NewRoleAntiEscalationService(db, &levelMapChecker{})
	if err := svc.EnsureCanAssignRole(context.Background(), "a", false, role.ID,
		valueobject.ScopeTypeProject, 9); !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("empty role must deny non-admin: %v", err)
	}
	if err := svc.EnsureCanAssignRole(context.Background(), "a", true, role.ID,
		valueobject.ScopeTypeOrganization, 1); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureCanAssignRole_MissingResourceTypeDenied(t *testing.T) {
	db := setupAntiEscDB(t)
	role := &entity.Role{Name: "broken", DisplayName: "B", IsActive: true}
	_ = db.Create(role)
	// policy without permission_definitions row
	_ = db.Create(&entity.RolePolicy{
		RoleID: role.ID, PermissionID: "ghost", PermissionLevel: "READ", ScopeType: "ORGANIZATION",
	})
	svc := NewRoleAntiEscalationService(db, &levelMapChecker{})
	err := svc.EnsureCanAssignRole(context.Background(), "a", false, role.ID,
		valueobject.ScopeTypeOrganization, 1)
	if !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("want escalation for missing def, got %v", err)
	}
}

func TestEnsureCanAddRolePolicy(t *testing.T) {
	db := setupAntiEscDB(t)
	_ = db.Create(&entity.PermissionDefinition{
		ID: "p1", Name: "p1", ResourceType: valueobject.ResourceTypeModules, ScopeLevel: valueobject.ScopeTypeOrganization, DisplayName: "M",
	})
	_ = db.Create(&entity.PermissionDefinition{
		ID: "p-ws", Name: "p-ws", ResourceType: valueobject.ResourceTypeWorkspaceExec, ScopeLevel: valueobject.ScopeTypeWorkspace, DisplayName: "WS",
	})
	chk := &levelMapChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeModules:       valueobject.PermissionLevelRead,
		valueobject.ResourceTypeWorkspaceExec: valueobject.PermissionLevelRead,
	}}
	svc := NewRoleAntiEscalationService(db, chk)

	role := &entity.Role{Name: "custom", DisplayName: "C", IsActive: true}
	_ = db.Create(role)
	// READ ok
	if err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, role.ID, "p1", "READ", valueobject.ScopeTypeOrganization,
		valueobject.ScopeTypeOrganization, 1); err != nil {
		t.Fatal(err)
	}
	// A workspace resource may intentionally be included in an organization
	// Role. The assignment scope is an ancestor of the resource scope, not a
	// mismatch.
	if err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, role.ID, "p-ws", "READ", valueobject.ScopeTypeOrganization,
		valueobject.ScopeTypeOrganization, 1); err != nil {
		t.Fatalf("organization-hosted workspace policy should be valid: %v", err)
	}
	// PermissionDefinition 的组织级 scope 不能以项目级 policy 写入，即使调用方
	// 绕过了 HTTP handler，service 仍应 fail-closed。
	if err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, role.ID, "p1", "READ", valueobject.ScopeTypeProject,
		valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("scope mismatch must be denied: %v", err)
	}
	// WRITE denied
	err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, role.ID, "p1", "WRITE", valueobject.ScopeTypeOrganization,
		valueobject.ScopeTypeOrganization, 1)
	if !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("want deny write: %v", err)
	}
	// unknown perm
	if err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, role.ID, "nope", "READ", valueobject.ScopeTypeOrganization,
		valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrPermissionDefNotFound) {
		t.Fatalf("want def not found: %v", err)
	}
	// system role policy mutate denied
	sys := &entity.Role{Name: "admin", DisplayName: "A", IsSystem: true, IsActive: true}
	_ = db.Create(sys)
	if err := svc.EnsureCanAddRolePolicy(context.Background(), "a", false, sys.ID, "p1", "READ", valueobject.ScopeTypeOrganization,
		valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrSystemRolePolicyReadonly) {
		t.Fatalf("system role policy: %v", err)
	}
}

func TestEnsureCanCloneRole(t *testing.T) {
	db := setupAntiEscDB(t)
	src := seedRoleWithPolicies(t, db, "src", false, []struct{ permID, level, rt string }{
		{"c1", "WRITE", "MODULES"},
	})
	admin := seedRoleWithPolicies(t, db, "admin", true, nil)

	chk := &levelMapChecker{levels: map[valueobject.ResourceType]valueobject.PermissionLevel{
		valueobject.ResourceTypeModules: valueobject.PermissionLevelRead, // insufficient for WRITE
	}}
	svc := NewRoleAntiEscalationService(db, chk)
	if err := svc.EnsureCanCloneRole(context.Background(), "a", false, src,
		valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrPrivilegeEscalation) {
		t.Fatalf("clone escalate: %v", err)
	}

	if err := svc.EnsureCanCloneRole(context.Background(), "a", false, admin,
		valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrSystemRoleRestricted) {
		t.Fatalf("clone admin: %v", err)
	}
	if err := svc.EnsureCanCloneRole(context.Background(), "super", true, admin,
		valueobject.ScopeTypeOrganization, 1); err != nil {
		t.Fatal(err)
	}
}

func TestIsPrivilegeEscalationError(t *testing.T) {
	if !IsPrivilegeEscalationError(ErrPrivilegeEscalation) {
		t.Fatal()
	}
	if !IsPrivilegeEscalationError(fmtWrap(ErrSystemRoleRestricted)) {
		t.Fatal()
	}
	if IsPrivilegeEscalationError(errors.New("other")) {
		t.Fatal()
	}
}

func fmtWrap(err error) error {
	return errors.Join(err, errors.New("wrap"))
}

func TestNilCheckerFailClosed(t *testing.T) {
	svc2 := NewRoleAntiEscalationService(nil, nil)
	if err := svc2.EnsureCanAssignRole(context.Background(), "a", false, 1, valueobject.ScopeTypeOrganization, 1); !errors.Is(err, ErrAntiEscalationMisconfigured) {
		t.Fatalf("want misconfigured: %v", err)
	}
}

func TestEnsureAssignmentScopeInAuthOrg(t *testing.T) {
	db := setupAntiEscDB(t)
	_ = db.Exec(`CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER)`)
	_ = db.Exec(`INSERT INTO projects (id, org_id) VALUES (10, 1), (20, 2)`)
	svc := NewRoleAntiEscalationService(db, &levelMapChecker{})
	if err := svc.EnsureAssignmentScopeInAuthOrg(context.Background(), valueobject.ScopeTypeOrganization, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureAssignmentScopeInAuthOrg(context.Background(), valueobject.ScopeTypeOrganization, 2, 1); !errors.Is(err, ErrScopeOutsideAuthOrg) {
		t.Fatalf("%v", err)
	}
	if err := svc.EnsureAssignmentScopeInAuthOrg(context.Background(), valueobject.ScopeTypeProject, 10, 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.EnsureAssignmentScopeInAuthOrg(context.Background(), valueobject.ScopeTypeProject, 20, 1); !errors.Is(err, ErrScopeOutsideAuthOrg) {
		t.Fatalf("%v", err)
	}
}
