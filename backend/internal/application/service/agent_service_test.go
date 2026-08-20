package service

import (
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupAgentServiceDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE applications (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  org_id INTEGER,
  name TEXT,
  app_key TEXT,
  app_secret TEXT,
  is_active INTEGER,
  expires_at DATETIME,
  last_used_at DATETIME,
  workspace_tag_filter TEXT
);`)
	return db
}

func TestAgentService_ValidateApplication(t *testing.T) {
	db := setupAgentServiceDB(t)
	svc := NewAgentService(db)
	plain := "s3cret"
	_ = db.Exec(`INSERT INTO applications (id, org_id, name, app_key, app_secret, is_active)
		VALUES (1,1,'a','k1',?,1)`, hashAppSecret(plain))

	app, err := svc.ValidateApplication("k1", plain)
	if err != nil || app.ID != 1 {
		t.Fatalf("%v %+v", err, app)
	}
	if _, err := svc.ValidateApplication("k1", "wrong"); err == nil {
		t.Fatal("wrong secret")
	}
	if _, err := svc.ValidateApplication("nope", plain); err == nil {
		t.Fatal("unknown key")
	}

	past := time.Now().Add(-time.Hour)
	_ = db.Exec(`UPDATE applications SET expires_at = ? WHERE id = 1`, past)
	if _, err := svc.ValidateApplication("k1", plain); err == nil {
		t.Fatal("expired")
	}
}

func TestAgentService_GenerateIDs(t *testing.T) {
	svc := NewAgentService(nil)
	id1 := svc.GenerateAgentID()
	id2 := svc.GenerateAgentID()
	if len(id1) < 10 || id1[:6] != "agent-" {
		t.Fatalf("%s", id1)
	}
	if id1 == id2 {
		// extremely unlikely; soft check
		t.Log("same id (rare)")
	}
	h := svc.GenerateTokenHash("tok")
	if len(h) != 64 {
		t.Fatalf("hash len %d", len(h))
	}
}
