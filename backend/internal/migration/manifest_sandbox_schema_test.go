package migration

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var wsRun = regexp.MustCompile(`\s+`)

func normalizeSQL(s string) string { return strings.TrimSpace(wsRun.ReplaceAllString(s, " ")) }

func TestManifestSandboxSchemaIsAdditiveAndIdempotent(t *testing.T) {
	for _, stmt := range manifestSandboxSchemaStatements() {
		upper := strings.ToUpper(stmt)
		for _, forbidden := range []string{"DROP ", "DELETE FROM", "TRUNCATE ", "UPDATE PUBLIC.", "RENAME ", "ALTER COLUMN", " TYPE "} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("statement must be additive, found %q in:\n%s", forbidden, stmt)
			}
		}
		switch {
		case strings.HasPrefix(upper, "ALTER TABLE"):
			if !strings.Contains(upper, "ADD COLUMN IF NOT EXISTS") {
				t.Fatalf("ALTER must be ADD COLUMN IF NOT EXISTS:\n%s", stmt)
			}
		case strings.HasPrefix(upper, "CREATE TABLE"), strings.HasPrefix(upper, "CREATE INDEX"):
			if !strings.Contains(upper, "IF NOT EXISTS") {
				t.Fatalf("CREATE must be IF NOT EXISTS:\n%s", stmt)
			}
		case strings.HasPrefix(upper, "DO $$"):
			if !strings.Contains(upper, "IF NOT EXISTS") {
				t.Fatalf("constraint block must be guarded:\n%s", stmt)
			}
		default:
			t.Fatalf("unexpected statement kind:\n%s", stmt)
		}
		if !strings.Contains(stmt, "public.") {
			t.Fatalf("tables must be schema-qualified (seed clears search_path):\n%s", stmt)
		}
	}
	// new NOT NULL columns on existing tables need a default (no rewrite / no failure on data)
	for _, stmt := range manifestSandboxSchemaStatements() {
		if strings.HasPrefix(stmt, "ALTER TABLE") && strings.Contains(stmt, "NOT NULL") && !strings.Contains(stmt, "DEFAULT") {
			t.Fatalf("NOT NULL column on existing table requires a DEFAULT:\n%s", stmt)
		}
	}
}

func TestManifestSandboxSchemaCarriesSpecConstraints(t *testing.T) {
	all := normalizeSQL(strings.Join(manifestSandboxSchemaStatements(), "\n"))
	for _, want := range []string{
		"source_type character varying(16) NOT NULL DEFAULT 'native'",
		"CHECK (source_type IN ('native', 'git'))",
		"CONSTRAINT chk_manifest_runs_sandbox_preview_only CHECK (runner <> 'sandbox' OR purpose = 'preview')",
		"CONSTRAINT chk_sandbox_sessions_network_mode CHECK (network_mode = 'vpc')",
		"FOREIGN KEY (run_id, workspace_id, purpose) REFERENCES public.manifest_runs(id, workspace_id, purpose)",
		"FOREIGN KEY (session_id, workspace_id) REFERENCES public.sandbox_sessions(id, workspace_id)",
		"token_hash character varying(64) NOT NULL",
		"approved_bundle_hash character varying(64)",
		"approved_plan_hash character varying(64)",
		"manifest_deployments ADD COLUMN IF NOT EXISTS sensitive_keys jsonb",
		"workspace_tasks ADD COLUMN IF NOT EXISTS sensitive_keys jsonb",
	} {
		if !strings.Contains(all, normalizeSQL(want)) {
			t.Fatalf("schema is missing %q", want)
		}
	}
	// sessions must not store token secrets
	for _, stmt := range manifestSandboxSchemaStatements() {
		if strings.Contains(stmt, "public.sandbox_sessions (") && strings.Contains(strings.ToLower(stmt), "token") {
			t.Fatal("sandbox_sessions must not store tokens")
		}
	}
}

// The psql patch and the seed must carry exactly the Go job's statements.
func TestManifestSandboxSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_sandbox_schema.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range manifestSandboxSchemaStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
		if !strings.Contains(sql, "convert_to('terranova-bundle-v1', 'UTF8')") ||
			!strings.Contains(sql, `ORDER BY f.path COLLATE "C"`) {
			t.Fatalf("%s is missing the bundle_hash backfill", path)
		}
	}
}
