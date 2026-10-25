package migration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSandboxRegistrationStatementsIdempotent(t *testing.T) {
	for _, stmt := range sandboxRegistrationStatements() {
		upper := strings.ToUpper(stmt)
		ok := strings.Contains(upper, "IF NOT EXISTS") ||
			strings.Contains(upper, "IF EXISTS") ||
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

func TestSandboxRegistrationSQLInSync(t *testing.T) {
	path := filepath.Join("..", "..", "migrations", "add_sandbox_registration.sql")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	body := string(content)
	for _, needle := range []string{
		"CREATE TABLE IF NOT EXISTS public.sandboxes",
		"CREATE TABLE IF NOT EXISTS public.shell_sessions",
		"uq_sandboxes_org_ref",
		"chk_sandboxes_network_mode",
		"network_mode",
		"agentcore_external",
		"shell_debug",
	} {
		if !strings.Contains(body, needle) {
			t.Fatalf("SQL mirror missing %q", needle)
		}
	}
}

func TestSandboxRegistrationRegistered(t *testing.T) {
	found := false
	for _, d := range definitions() {
		if d.version == "20261025_01_sandbox_registration" {
			found = true
			if d.contract != "sandbox-registration-shell-sessions-v1" {
				t.Fatalf("contract = %q", d.contract)
			}
			if d.apply == nil {
				t.Fatal("apply is nil")
			}
		}
	}
	if !found {
		t.Fatal("20261025_01_sandbox_registration not registered")
	}
}

func TestSandboxRegistrationHasNetworkModeAndNoCallbackURL(t *testing.T) {
	joined := strings.Join(sandboxRegistrationStatements(), "\n")
	if !strings.Contains(joined, "network_mode") {
		t.Fatal("sandboxes must persist network_mode snapshot")
	}
	lower := strings.ToLower(joined)
	for _, leak := range []string{"callback_url", "webhook_url", "invoke_url"} {
		if strings.Contains(lower, leak) {
			t.Fatalf("control plane is pull-based; must not store %s", leak)
		}
	}
}
