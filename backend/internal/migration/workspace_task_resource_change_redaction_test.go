package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceTaskResourceChangeRedactionIsAdditive(t *testing.T) {
	for _, stmt := range workspaceTaskResourceChangeRedactionStatements() {
		upper := strings.ToUpper(stmt)
		if !strings.HasPrefix(upper, "ALTER TABLE PUBLIC.") || !strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") ||
			strings.Contains(upper, "NOT NULL") || strings.Contains(upper, "DROP ") {
			t.Fatalf("statement must be an idempotent nullable ADD COLUMN:\n%s", stmt)
		}
	}
}

func TestWorkspaceTaskResourceChangeRedactionSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_workspace_task_resource_change_redaction.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range workspaceTaskResourceChangeRedactionStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}
