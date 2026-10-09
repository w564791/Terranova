package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceTaskErrorCodeIsAdditiveAndIdempotent(t *testing.T) {
	for _, stmt := range workspaceTaskErrorCodeStatements() {
		upper := strings.ToUpper(stmt)
		if !strings.HasPrefix(upper, "ALTER TABLE PUBLIC.") || !strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") ||
			strings.Contains(upper, "NOT NULL") || strings.Contains(upper, "DROP ") {
			t.Fatalf("statement must be an idempotent nullable ADD COLUMN:\n%s", stmt)
		}
	}
}

// The psql patch and the seed must carry exactly the Go job's statements.
func TestWorkspaceTaskErrorCodeSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_workspace_task_error_code.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range workspaceTaskErrorCodeStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}

func TestWorkspaceTaskErrorCodeRegistered(t *testing.T) {
	for _, d := range definitions() {
		if d.version == "20261010_01_workspace_task_error_code" {
			return
		}
	}
	t.Fatal("migration 20261010_01_workspace_task_error_code is not registered")
}
