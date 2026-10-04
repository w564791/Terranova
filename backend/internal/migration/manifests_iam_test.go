package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestIAMPermissionsMirrorCatalogResources(t *testing.T) {
	byResource := map[string]orgCatalogPermission{}
	for _, p := range manifestIAMPermissions() {
		byResource[p.resource] = p
	}
	m, ok := byResource["MANIFESTS"]
	if !ok {
		t.Fatal("MANIFESTS permission definition must be registered")
	}
	want := map[string]string{"admin": "ADMIN", "org_admin": "ADMIN", "developer": "READ", "viewer": "READ", "user": "READ"}
	got := map[string]string{}
	for _, g := range m.grants {
		got[g.role] = g.level
	}
	for role, level := range want {
		if got[role] != level {
			t.Fatalf("MANIFESTS grant for %s = %q, want %q", role, got[role], level)
		}
	}
	if len(got) != len(want) {
		t.Fatalf("unexpected MANIFESTS grants: %v", got)
	}
	if _, ok := byResource["VARIABLE_SETS"]; !ok {
		t.Fatal("VARIABLE_SETS permission definition must be registered")
	}
}

func TestManifestIAMStatementsAreAdditive(t *testing.T) {
	for _, sql := range []string{insertOrgPermissionDefinitionSQL, insertBuiltinRoleOrgPolicySQL} {
		upper := strings.ToUpper(sql)
		for _, forbidden := range []string{"DELETE ", "UPDATE ", "DROP ", "TRUNCATE "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("manifest IAM migration must be additive, found %q", forbidden)
			}
		}
		if !strings.Contains(sql, "NOT EXISTS") {
			t.Fatal("manifest IAM migration statements must be idempotent")
		}
	}
	if !strings.Contains(insertBuiltinRoleOrgPolicySQL, "r.is_system = true") || !strings.Contains(insertBuiltinRoleOrgPolicySQL, "r.org_id = 0") {
		t.Fatal("policies must only be attached to built-in system Roles")
	}
}

// The psql patch and the seed must carry the same definitions as the Go job.
func TestManifestIAMSQLPatchAndSeedInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_iam_permissions.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := string(content)
		for _, p := range manifestIAMPermissions() {
			if !strings.Contains(sql, "'"+p.id+"'") {
				t.Fatalf("%s is missing permission definition %s", path, p.id)
			}
			for _, g := range p.grants {
				frag := "'" + g.level + "', '" + p.resource + "', '" + g.role + "'"
				if !strings.Contains(sql, frag) {
					t.Fatalf("%s is missing grant %s", path, frag)
				}
			}
		}
	}
}
