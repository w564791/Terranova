package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTokenBindingIsAdditive(t *testing.T) {
	for _, stmt := range runTokenBindingStatements() {
		upper := strings.ToUpper(stmt)
		additive := strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") || strings.HasPrefix(upper, "CREATE INDEX IF NOT EXISTS")
		if !additive || strings.Contains(upper, "DROP ") {
			t.Fatalf("statement must be an idempotent ADD COLUMN / CREATE INDEX:\n%s", stmt)
		}
		if strings.Contains(upper, "ADD COLUMN") && strings.Contains(upper, "NOT NULL") && !strings.Contains(upper, "DEFAULT") {
			t.Fatalf("NOT NULL column needs a default:\n%s", stmt)
		}
	}
}

func TestRunTokenBindingSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_run_token_binding.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range runTokenBindingStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}
