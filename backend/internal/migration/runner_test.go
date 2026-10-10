package migration

import (
	"strings"
	"testing"
)

func TestDefinitionsAreOrderedAndVersioned(t *testing.T) {
	definitions := definitions()
	want := []string{
		"20260717_01_iam_multitenant_hardening",
		"20260717_02_iam_role_data_transition",
		"20260717_03_iam_release_completeness",
		"20260717_04_iam_application_role_transition",
		"20260717_05_iam_role_tenant_reconciliation",
		"20261004_01_manifest_iam_resource",
		"20261004_02_manifest_sandbox_schema",
		"20261004_04_manifest_bundle_v2",
		"20261010_01_workspace_task_error_code",
		"20261010_02_workspace_task_plan_json_redaction",
		"20261010_03_workspace_task_error_reason",
		"20261010_04_workspace_task_resource_change_redaction",
		"20261010_10_variable_key_version",
		"20261010_11_agent_token",
		"20261010_12_run_token_binding",
		"20261010_13_manifest_approval",
	}
	if len(definitions) != len(want) {
		t.Fatalf("migration count = %d, want %d", len(definitions), len(want))
	}
	seen := make(map[string]struct{}, len(definitions))
	for i, migration := range definitions {
		if migration.version != want[i] {
			t.Fatalf("migration[%d] = %q, want %q", i, migration.version, want[i])
		}
		if migration.contract == "" || migration.apply == nil {
			t.Fatalf("migration %q must have a contract and apply function", migration.version)
		}
		if _, exists := seen[migration.version]; exists {
			t.Fatalf("duplicate migration version %q", migration.version)
		}
		seen[migration.version] = struct{}{}
		if checksum := checksumFor(migration); len(checksum) != 64 {
			t.Fatalf("migration %q checksum length = %d, want SHA-256 hex", migration.version, len(checksum))
		}
	}
}

func TestLegacyApplicationPrincipalNormalizationStatementsBindResourceTenant(t *testing.T) {
	statements := legacyApplicationPrincipalNormalizationStatements()
	if len(statements) != 3 {
		t.Fatalf("normalization statements = %d, want 3", len(statements))
	}

	for i, wantFragments := range [][]string{
		{"a.org_id = op.org_id"},
		{"p.id = pp.project_id", "a.org_id = p.org_id"},
		{"w.workspace_id = wp.workspace_id", "p.id = wpr.project_id", "a.org_id = p.org_id"},
	} {
		for _, fragment := range wantFragments {
			if !strings.Contains(statements[i], fragment) {
				t.Fatalf("normalization statement %d must contain %q:\n%s", i, fragment, statements[i])
			}
		}
	}
}

func TestRoleTenantBoundaryAllowsOnlyInactiveUnownedCustomRoles(t *testing.T) {
	const want = "(is_system = true OR is_active = false OR org_id > 0)"
	if iamRoleTenantCheckExpression != want {
		t.Fatalf("role tenant check = %q, want %q", iamRoleTenantCheckExpression, want)
	}
}
