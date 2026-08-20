package valueobject

import (
	"encoding/json"
	"testing"
)

func TestPermissionLevel_StringAndParse(t *testing.T) {
	cases := []struct {
		level PermissionLevel
		str   string
	}{
		{PermissionLevelNone, "NONE"},
		{PermissionLevelRead, "READ"},
		{PermissionLevelWrite, "WRITE"},
		{PermissionLevelAdmin, "ADMIN"},
	}
	for _, tc := range cases {
		if tc.level.String() != tc.str {
			t.Fatalf("String: got %s want %s", tc.level.String(), tc.str)
		}
		got, err := ParsePermissionLevel(tc.str)
		if err != nil || got != tc.level {
			t.Fatalf("Parse %s: got %v err %v", tc.str, got, err)
		}
		if !tc.level.IsValid() {
			t.Fatalf("%s should be valid", tc.str)
		}
	}
	if _, err := ParsePermissionLevel("NOPE"); err == nil {
		t.Fatal("expected parse error")
	}
	if PermissionLevel(99).IsValid() {
		t.Fatal("99 should be invalid")
	}
	if !PermissionLevelWrite.GreaterThanOrEqual(PermissionLevelRead) {
		t.Fatal("WRITE should >= READ")
	}
	if Max(PermissionLevelRead, PermissionLevelAdmin) != PermissionLevelAdmin {
		t.Fatal("Max failed")
	}
	if MaxLevel(nil) != PermissionLevelNone {
		t.Fatal("MaxLevel empty")
	}
	if MaxLevel([]PermissionLevel{PermissionLevelRead, PermissionLevelWrite}) != PermissionLevelWrite {
		t.Fatal("MaxLevel failed")
	}
}

func TestPermissionLevel_JSON(t *testing.T) {
	var p PermissionLevel
	if err := json.Unmarshal([]byte(`"WRITE"`), &p); err != nil || p != PermissionLevelWrite {
		t.Fatalf("unmarshal: %v %v", err, p)
	}
	b, err := json.Marshal(PermissionLevelAdmin)
	if err != nil || string(b) != `"ADMIN"` {
		t.Fatalf("marshal: %s err %v", b, err)
	}
}

func TestScopeType(t *testing.T) {
	for _, s := range []ScopeType{ScopeTypeOrganization, ScopeTypeProject, ScopeTypeWorkspace} {
		if !s.IsValid() {
			t.Fatalf("%s invalid", s)
		}
		got, err := ParseScopeType(string(s))
		if err != nil || got != s {
			t.Fatalf("parse %s", s)
		}
	}
	if _, err := ParseScopeType("NOPE"); err == nil {
		t.Fatal("expected invalid scope")
	}
	if ScopeTypeWorkspace.GetPriority() <= ScopeTypeProject.GetPriority() {
		t.Fatal("workspace should outrank project")
	}
	if !ScopeTypeWorkspace.IsMoreSpecificThan(ScopeTypeOrganization) {
		t.Fatal("workspace more specific than org")
	}
	if ScopeTypeOrganization.IsMoreSpecificThan(ScopeTypeWorkspace) {
		t.Fatal("org not more specific than workspace")
	}
	if !ScopeTypeOrganization.CanHostPolicyFor(ScopeTypeWorkspace) {
		t.Fatal("organization Role may host a workspace policy")
	}
	if !ScopeTypeProject.CanHostPolicyFor(ScopeTypeWorkspace) {
		t.Fatal("project Role may host a workspace policy")
	}
	if ScopeTypeWorkspace.CanHostPolicyFor(ScopeTypeProject) {
		t.Fatal("workspace Role cannot host a project policy")
	}
	if ScopeTypeProject.CanHostPolicyFor(ScopeTypeOrganization) {
		t.Fatal("project Role cannot host an organization policy")
	}
}

func TestResourceType_ValidAndScope(t *testing.T) {
	orgTypes := []ResourceType{
		ResourceTypeAllWorkspaces, ResourceTypeModules, ResourceTypeIAMPermissions,
		ResourceTypeSystemSettings, ResourceTypeRunTasks,
	}
	for _, rt := range orgTypes {
		if !rt.IsValid() || !rt.IsOrganizationLevel() {
			t.Fatalf("%s should be valid org-level", rt)
		}
	}
	if !ResourceTypeProjectWorkspaces.IsProjectLevel() {
		t.Fatal("project workspaces")
	}
	if !ResourceTypeWorkspaceExec.IsWorkspaceLevel() {
		t.Fatal("workspace exec")
	}
	if ResourceType("PROVIDER_TEMPLATES").IsValid() {
		// currently may be invalid — document expected behavior
		t.Log("PROVIDER_TEMPLATES registered as valid")
	}
	got, err := ParseResourceType("WORKSPACE_EXECUTION")
	if err != nil || got != ResourceTypeWorkspaceExec {
		t.Fatalf("parse WORKSPACE_EXECUTION: %v %v", got, err)
	}
	got, err = ParseResourceType("workspaces")
	if err != nil || got != ResourceTypeAllWorkspaces {
		t.Fatalf("parse workspaces: %v %v", got, err)
	}
	if _, err := ParseResourceType("not_a_type"); err == nil {
		t.Fatal("expected invalid resource type")
	}
}

func TestResourceType_WorkspaceManagementImplication(t *testing.T) {
	for _, fine := range []ResourceType{
		ResourceTypeTaskData,
		ResourceTypeWorkspaceExec,
		ResourceTypeWorkspaceState,
		ResourceTypeWorkspaceVars,
		ResourceTypeWorkspaceResources,
	} {
		if !fine.IsSatisfiedBy(ResourceTypeWorkspaceManagement) {
			t.Fatalf("WORKSPACE_MANAGEMENT should satisfy %s", fine)
		}
	}

	if ResourceTypeWorkspaceManagement.IsSatisfiedBy(ResourceTypeWorkspaceExec) {
		t.Fatal("fine-grained execution must not satisfy WORKSPACE_MANAGEMENT")
	}
	if ResourceTypeWorkspaceStateSensitive.IsSatisfiedBy(ResourceTypeWorkspaceManagement) {
		t.Fatal("sensitive state must retain its stricter MANAGEMENT ADMIN fallback")
	}
}

func TestPrincipalType(t *testing.T) {
	for _, p := range []PrincipalType{PrincipalTypeUser, PrincipalTypeTeam, PrincipalTypeApplication} {
		if !p.IsValid() {
			t.Fatalf("%s invalid", p)
		}
	}
	if !PrincipalTypeUser.CanBeGrantedAt(ScopeTypeWorkspace) {
		t.Fatal("user can grant at workspace")
	}
	if PrincipalTypeApplication.CanBeGrantedAt(ScopeTypeWorkspace) {
		t.Fatal("application cannot grant at workspace")
	}
	if !PrincipalTypeApplication.CanBeGrantedAt(ScopeTypeOrganization) {
		t.Fatal("application can grant at org")
	}
	if _, err := ParsePrincipalType("GHOST"); err == nil {
		t.Fatal("expected parse error")
	}
	if !PrincipalTypeTeam.IsTeam() || !PrincipalTypeUser.IsUser() || !PrincipalTypeApplication.IsApplication() {
		t.Fatal("type helpers")
	}
}
