package services

import (
	"context"
	"testing"
	"time"

	"iac-platform/internal/models"

	"github.com/golang-jwt/jwt/v5"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupShellTokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	t.Setenv("SIGNING_ROOT_KEY", "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")
	t.Setenv("ENV", "development")
	db, err := gorm.Open(sqlite.Open("file:shelltok_"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{
		`CREATE TABLE sandboxes (
			id TEXT PRIMARY KEY, org_id INTEGER, workspace_id TEXT, sandbox_ref TEXT,
			driver TEXT, network_mode TEXT, status TEXT, created_by TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE shell_sessions (
			id TEXT PRIMARY KEY, sandbox_id TEXT, org_id INTEGER, status TEXT,
			expires_at DATETIME, closed_at DATETIME, created_by TEXT,
			created_at DATETIME, updated_at DATETIME
		)`,
		`CREATE TABLE shell_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT, shell_session_id TEXT, sandbox_id TEXT,
			org_id INTEGER, workspace_id TEXT, token_hash TEXT UNIQUE,
			expires_at DATETIME, revoked_at DATETIME, created_at DATETIME
		)`,
	} {
		if err := db.Exec(s).Error; err != nil {
			t.Fatal(err)
		}
	}
	return db
}

func TestIssueShellToken_ClaimsAndRevoke(t *testing.T) {
	db := setupShellTokenDB(t)
	svc := NewStateTokenService(db)
	now := time.Now()
	sb := &models.Sandbox{
		ID: "sb-1", OrgID: 1, WorkspaceID: "ws-a",
		SandboxRef: "ref-1", Driver: models.SandboxDriverShellDebug,
		Status: models.SandboxStatusReady, CreatedBy: "u1",
	}
	if err := db.Create(sb).Error; err != nil {
		t.Fatal(err)
	}
	sess := &models.ShellSession{
		ID: "sh-1", SandboxID: sb.ID, OrgID: 1,
		Status:    models.ShellSessionStatusActive,
		ExpiresAt: now.Add(MaxShellSessionTTL), CreatedBy: "u1",
	}
	if err := db.Create(sess).Error; err != nil {
		t.Fatal(err)
	}
	tok, exp, err := svc.IssueShellToken(context.Background(), nil, sess, sb)
	if err != nil {
		t.Fatal(err)
	}
	if tok == "" || !exp.After(now) {
		t.Fatalf("tok/exp empty")
	}
	if exp.Sub(now) > MaxShellSessionTTL+time.Second {
		t.Fatalf("exp beyond hard cap: %v", exp.Sub(now))
	}
	var claims ShellTokenClaims
	parser := jwt.NewParser(jwt.WithoutClaimsValidation())
	if _, _, err := parser.ParseUnverified(tok, &claims); err != nil {
		t.Fatal(err)
	}
	if claims.Type != StateTokenTypeShell || claims.ShellSessionID != sess.ID {
		t.Fatalf("claims=%+v", claims)
	}
	if claims.WorkspaceID != "ws-a" || claims.OrgID != 1 || claims.SandboxID != sb.ID {
		t.Fatalf("claims=%+v", claims)
	}
	caller, err := svc.Authenticate(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if caller.Type != StateTokenTypeShell || !caller.ReadOnly() {
		t.Fatalf("%+v", caller)
	}
	if err := EndShellSession(context.Background(), db, sess.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), tok); err == nil {
		t.Fatal("expected revoke")
	}
	var n int64
	db.Table("shell_tokens").Where("shell_session_id = ? AND revoked_at IS NULL", sess.ID).Count(&n)
	if n != 0 {
		t.Fatalf("active=%d", n)
	}
}
