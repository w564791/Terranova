package service

import (
	"context"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTeamTokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	// 每个测试独立 in-memory DB，避免 cache=shared 串数据
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`CREATE TABLE teams (team_id TEXT PRIMARY KEY, name TEXT);`)
	_ = db.Exec(`INSERT INTO teams (team_id, name) VALUES ('team-1', 't1');`)
	_ = db.Exec(`
CREATE TABLE team_tokens (
  token_id_hash TEXT PRIMARY KEY,
  team_id TEXT,
  token_name TEXT,
  token_hash TEXT,
  is_active INTEGER,
  created_at DATETIME,
  created_by TEXT,
  expires_at DATETIME,
  revoked_at DATETIME,
  revoked_by TEXT,
  last_used_at DATETIME
);`)
	return db
}

func TestGenerateToken_Max24h(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")

	// never-expire request (0) → forced to 1 day
	resp, err := svc.GenerateToken(context.Background(), "team-1", "tok-a", "user-1", 0)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp.ExpiresAt == nil {
		t.Fatal("expires_at must be set (no never-expire)")
	}
	delta := time.Until(*resp.ExpiresAt)
	if delta > 25*time.Hour || delta < 23*time.Hour {
		t.Fatalf("expected ~24h expiry, got %v", delta)
	}

	// 30 days request → capped to 1 day
	resp2, err := svc.GenerateToken(context.Background(), "team-1", "tok-b", "user-1", 30)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if resp2.ExpiresAt == nil {
		t.Fatal("expires_at required")
	}
	delta2 := time.Until(*resp2.ExpiresAt)
	if delta2 > 25*time.Hour {
		t.Fatalf("expected cap at 24h, got %v", delta2)
	}
}

func TestRevokeTokenByName(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.GenerateToken(context.Background(), "team-1", "tok-revoke", "user-1", 1); err != nil {
		t.Fatal(err)
	}
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "tok-revoke", "user-1"); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := svc.RevokeTokenByName(context.Background(), "team-1", "tok-revoke", "user-1"); err == nil {
		t.Fatal("second revoke should fail")
	}
}

func TestGenerateToken_RejectDuplicateActiveName(t *testing.T) {
	db := setupTeamTokenDB(t)
	svc := NewTeamTokenService(db, "test-secret-key-at-least-32-bytes!!")
	if _, err := svc.GenerateToken(context.Background(), "team-1", "same-name", "user-1", 1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.GenerateToken(context.Background(), "team-1", "same-name", "user-1", 1); err == nil {
		t.Fatal("duplicate active token_name must fail")
	}
}
