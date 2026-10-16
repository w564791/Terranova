package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVariableSetOrgRequiredNotValidAndInSync(t *testing.T) {
	stmts := strings.Join(variableSetOrgRequiredStatements(), "\n")
	for _, s := range []string{"chk_variable_sets_specific_org", "scope <> 'specific' OR org_id IS NOT NULL", "NOT VALID", "IF NOT EXISTS"} {
		if !strings.Contains(stmts, s) {
			t.Fatalf("statements missing %q", s)
		}
	}
	sqlFile, err := os.ReadFile(filepath.Join("..", "..", "migrations", "add_variable_set_org_required.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(sqlFile), "CHECK (scope <> 'specific' OR org_id IS NOT NULL) NOT VALID") {
		t.Fatal("SQL mirror out of sync")
	}
	if strings.Contains(strings.ToUpper(stmts), "VALIDATE CONSTRAINT") {
		t.Fatal("VALIDATE is an ops step, not part of the migration")
	}
}
