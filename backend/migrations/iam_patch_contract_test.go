package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestApplicationRolePatchDoesNotUseVolatileIndexPredicate(t *testing.T) {
	content, err := os.ReadFile("patch_iam_application_roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	if strings.Contains(sql, "expires_at IS NULL OR expires_at > NOW()") {
		t.Fatal("application role migration must not use NOW() in a partial unique-index predicate")
	}
	if !strings.Contains(sql, "uq_app_roles_identity") {
		t.Fatal("application role migration must enforce a stable logical assignment identity")
	}
}

func TestLegacyDirectGrantPatchPreservesExpiryAndTenant(t *testing.T) {
	content, err := os.ReadFile("patch_legacy_direct_grants_to_roles.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(content)
	for _, required := range []string{
		"AND (expires_at IS NULL OR expires_at > NOW())",
		"g.expires_at IS NOT DISTINCT FROM",
		"INSERT INTO iam_roles (org_id, name",
		"assigned_at, expires_at, reason",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("legacy direct-grant migration is missing %q", required)
		}
	}
}
