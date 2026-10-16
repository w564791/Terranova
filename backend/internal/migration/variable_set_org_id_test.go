package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariableSetOrgIDStatementsIdempotent(t *testing.T) {
	for _, stmt := range variableSetOrgIDStatements() {
		upper := strings.ToUpper(stmt)
		ok := strings.Contains(upper, "IF NOT EXISTS") ||
			strings.Contains(upper, "IF EXISTS") ||
			strings.Contains(upper, "DROP INDEX IF EXISTS") ||
			strings.HasPrefix(strings.TrimSpace(upper), "COMMENT ON")
		if !ok {
			t.Fatalf("statement must be idempotent:\n%s", stmt)
		}
		for _, forbidden := range []string{"DROP TABLE", "TRUNCATE ", "DELETE FROM"} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("DDL must not contain %q:\n%s", forbidden, stmt)
			}
		}
	}
}

func TestVariableSetOrgIDSQLInSync(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "add_variable_set_org_id.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(content)
	for _, needle := range []string{
		"org_id",
		"chk_variable_sets_org_scope",
		"idx_variable_sets_org_name",
		"idx_variable_sets_global_name",
		"DROP INDEX IF EXISTS public.idx_variable_sets_name",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("SQL mirror missing %q", needle)
		}
	}
}

func TestVariableSetOrgIDRegistered(t *testing.T) {
	found := false
	for _, d := range definitions() {
		if d.version == "20261016_01_variable_set_org_id" {
			found = true
			if d.contract != "variable-set-org-id-v1" {
				t.Fatalf("contract = %q", d.contract)
			}
			if d.apply == nil {
				t.Fatal("apply is nil")
			}
		}
	}
	if !found {
		t.Fatal("20261016_01_variable_set_org_id not registered")
	}
}
