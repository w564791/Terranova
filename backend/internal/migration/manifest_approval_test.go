package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestApprovalIsAdditive(t *testing.T) {
	for _, stmt := range manifestApprovalStatements() {
		upper := strings.ToUpper(stmt)
		additive := strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") ||
			strings.HasPrefix(upper, "CREATE UNIQUE INDEX IF NOT EXISTS") ||
			(strings.HasPrefix(upper, "DO $$") && strings.Contains(upper, "IF NOT EXISTS (SELECT 1 FROM PG_CONSTRAINT") &&
				strings.Contains(upper, "ADD CONSTRAINT"))
		if !additive {
			t.Fatalf("statement must be an idempotent ADD COLUMN / CREATE INDEX / guarded ADD CONSTRAINT:\n%s", stmt)
		}
		for _, forbidden := range []string{"DROP ", "DELETE FROM", "TRUNCATE ", "UPDATE PUBLIC.", "RENAME ", "ALTER COLUMN", " TYPE "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("statement must be additive, found %q in:\n%s", forbidden, stmt)
			}
		}
		if strings.Contains(upper, "ADD COLUMN") && strings.Contains(upper, "NOT NULL") && !strings.Contains(upper, "DEFAULT") {
			t.Fatalf("NOT NULL column needs a default:\n%s", stmt)
		}
	}
}

func TestManifestApprovalCheckRestrictsToAgentApprovalRuns(t *testing.T) {
	var check string
	for _, stmt := range manifestApprovalStatements() {
		if strings.Contains(stmt, "chk_manifest_runs_approval CHECK") {
			check = stmt
		}
	}
	for _, want := range []string{"purpose = 'approval'", "runner = 'agent'", "approved_bundle_hash = bundle_hash"} {
		if !strings.Contains(check, want) {
			t.Fatalf("chk_manifest_runs_approval missing %q:\n%s", want, check)
		}
	}
}

func TestManifestApprovalSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_approval.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range manifestApprovalStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}
