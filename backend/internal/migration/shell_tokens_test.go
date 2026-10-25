package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestShellTokensStatementsIdempotent(t *testing.T) {
	for _, stmt := range shellTokensStatements() {
		upper := strings.ToUpper(stmt)
		ok := strings.Contains(upper, "IF NOT EXISTS") ||
			strings.Contains(upper, "IF EXISTS") ||
			strings.HasPrefix(strings.TrimSpace(upper), "COMMENT ON")
		if !ok {
			t.Fatalf("statement must be idempotent:\n%s", stmt)
		}
	}
}

func TestShellTokensSQLInSync(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "add_shell_tokens.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(content)
	for _, needle := range []string{
		"CREATE TABLE IF NOT EXISTS public.shell_tokens",
		"shell_session_id",
		"token_hash",
		"revoked_at",
		"uq_shell_tokens_token_hash",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("SQL mirror missing %q", needle)
		}
	}
}

func TestShellTokensRegistered(t *testing.T) {
	found := false
	for _, d := range definitions() {
		if d.version == "20261025_02_shell_tokens" {
			found = true
			if d.contract != "shell-tokens-v1" {
				t.Fatalf("contract = %q", d.contract)
			}
			if d.apply == nil {
				t.Fatal("apply is nil")
			}
		}
	}
	if !found {
		t.Fatal("20261025_02_shell_tokens not registered")
	}
}
