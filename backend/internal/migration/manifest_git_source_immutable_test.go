package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestGitSourceImmutableIsIdempotent(t *testing.T) {
	for _, stmt := range manifestGitSourceImmutableStatements() {
		upper := strings.ToUpper(stmt)
		ok := strings.Contains(upper, "CREATE OR REPLACE FUNCTION") ||
			strings.Contains(upper, "DROP TRIGGER IF EXISTS") ||
			strings.Contains(upper, "CREATE TRIGGER")
		if !ok {
			t.Fatalf("unexpected statement:\n%s", stmt)
		}
		for _, forbidden := range []string{"DROP TABLE", "DELETE FROM", "TRUNCATE ", "ALTER TABLE PUBLIC.MANIFESTS DROP"} {
			if strings.Contains(upper, forbidden) {
				t.Fatalf("found %q in:\n%s", forbidden, stmt)
			}
		}
	}
}

func TestManifestGitSourceImmutableProtectsFields(t *testing.T) {
	fn := strings.Join(manifestGitSourceImmutableStatements(), "\n")
	for _, col := range []string{"source_type", "git_repo_url", "git_subpath", "github_installation_id"} {
		if !strings.Contains(fn, "OLD."+col) || !strings.Contains(fn, "NEW."+col) {
			t.Fatalf("trigger must compare %s", col)
		}
	}
	if !strings.Contains(fn, "git_source_immutable") {
		t.Fatal("exception message must name git_source_immutable")
	}
	// Must not lock git_latest_* (webhook updates those).
	if strings.Contains(fn, "git_latest") {
		t.Fatal("trigger must not touch git_latest_*")
	}
}

func TestManifestGitSourceImmutableSQLInSync(t *testing.T) {
	for _, path := range []string{
		filepath.Join("..", "..", "migrations", "add_manifest_git_source_immutable.sql"),
		filepath.Join("..", "..", "..", "manifests", "db", "init_seed_data.sql"),
	} {
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		sql := normalizeSQL(string(content))
		for _, stmt := range manifestGitSourceImmutableStatements() {
			if !strings.Contains(sql, normalizeSQL(stmt)+";") {
				t.Fatalf("%s is missing statement:\n%s", path, stmt)
			}
		}
	}
}
