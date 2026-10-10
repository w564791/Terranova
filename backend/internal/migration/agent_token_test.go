package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentTokenIsAdditive(t *testing.T) {
	for _, stmt := range agentTokenStatements() {
		upper := strings.ToUpper(stmt)
		if !strings.HasPrefix(upper, "ALTER TABLE PUBLIC.AGENTS") || !strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") || strings.Contains(upper, "DROP ") {
			t.Fatalf("statement must be an idempotent ADD COLUMN:\n%s", stmt)
		}
		if strings.Contains(upper, "NOT NULL") && !strings.Contains(upper, "DEFAULT") {
			t.Fatalf("NOT NULL column needs a default:\n%s", stmt)
		}
	}
}

func TestAgentTokenSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_agent_token.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range agentTokenStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}
