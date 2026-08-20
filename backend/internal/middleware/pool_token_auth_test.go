package middleware

import (
	"testing"

	"iac-platform/internal/models"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupPoolAllowDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
CREATE TABLE pool_allowed_workspaces (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  pool_id TEXT NOT NULL,
  workspace_id TEXT NOT NULL,
  status TEXT NOT NULL DEFAULT 'active',
  allowed_at DATETIME,
  revoked_at DATETIME
);`).Error; err != nil {
		t.Fatalf("create table: %v", err)
	}
	return db
}

func TestCheckPoolWorkspaceAccess_ActiveOnly(t *testing.T) {
	db := setupPoolAllowDB(t)

	// active
	if err := db.Exec(
		`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES (?,?,?)`,
		"pool-a", "ws-1", models.AllowanceStatusActive,
	).Error; err != nil {
		t.Fatal(err)
	}
	// revoked
	if err := db.Exec(
		`INSERT INTO pool_allowed_workspaces (pool_id, workspace_id, status) VALUES (?,?,?)`,
		"pool-a", "ws-2", models.AllowanceStatusRevoked,
	).Error; err != nil {
		t.Fatal(err)
	}

	if !checkPoolWorkspaceAccess(db, "pool-a", "ws-1") {
		t.Fatal("active allowance should allow access")
	}
	if checkPoolWorkspaceAccess(db, "pool-a", "ws-2") {
		t.Fatal("revoked allowance must NOT allow access")
	}
	if checkPoolWorkspaceAccess(db, "pool-a", "ws-missing") {
		t.Fatal("missing allowance must deny")
	}
}
