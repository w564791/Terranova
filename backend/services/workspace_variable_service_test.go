package services

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func TestGetVariableInWorkspace_BindsWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE workspace_variables (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT,
  variable_id TEXT,
  key TEXT,
  value TEXT,
  version INTEGER DEFAULT 1,
  is_deleted INTEGER DEFAULT 0
);`).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(
		`INSERT INTO workspace_variables (id, workspace_id, key, value) VALUES (1, 'ws-a', 'k', 'v')`,
	).Error; err != nil {
		t.Fatal(err)
	}

	svc := NewWorkspaceVariableService(db)

	// correct workspace
	v, err := svc.GetVariableInWorkspace("ws-a", 1)
	if err != nil || v == nil {
		t.Fatalf("expected variable in ws-a: %v", err)
	}

	// wrong workspace → not found (IDOR blocked)
	if _, err := svc.GetVariableInWorkspace("ws-b", 1); err == nil {
		t.Fatal("expected not found for other workspace")
	}
}

func TestGetVariableVersionInWorkspace_BindsWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`
CREATE TABLE workspace_variables (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT,
  variable_id TEXT,
  key TEXT,
  value TEXT,
  version INTEGER DEFAULT 1,
  is_deleted INTEGER DEFAULT 0
);`)
	// 同 variable_id 不同 workspace（模拟跨租户碰撞）
	_ = db.Exec(`INSERT INTO workspace_variables (workspace_id, variable_id, key, value, version)
		VALUES ('ws-a', 'vid-shared', 'k', 'a', 2), ('ws-b', 'vid-shared', 'k', 'b', 2)`)

	svc := NewWorkspaceVariableService(db)
	v, err := svc.GetVariableVersionInWorkspace("ws-a", "vid-shared", 2)
	if err != nil || v == nil || v.Value != "a" {
		t.Fatalf("ws-a v2: err=%v val=%v", err, v)
	}
	if _, err := svc.GetVariableVersionInWorkspace("ws-b", "vid-shared", 2); err != nil {
		t.Fatalf("ws-b should find own row: %v", err)
	}
	// 裸 GetVariableVersion 不绑 WS（遗留）；业务入口不得使用
	legacy, err := svc.GetVariableVersion("vid-shared", 2)
	if err != nil || legacy == nil {
		t.Fatalf("legacy global get: %v", err)
	}
	// 错误 workspace + version 不存在
	if _, err := svc.GetVariableVersionInWorkspace("ws-a", "vid-shared", 99); err == nil {
		t.Fatal("missing version must fail")
	}
}

func TestVariableMutationsBoundToWorkspace(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`
CREATE TABLE workspace_variables (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  workspace_id TEXT,
  variable_id TEXT,
  key TEXT,
  value TEXT,
  version INTEGER DEFAULT 1,
  is_deleted INTEGER DEFAULT 0,
  variable_type TEXT,
  value_format TEXT,
  sensitive INTEGER DEFAULT 0,
  description TEXT,
  created_by TEXT
);`).Error; err != nil {
		t.Fatal(err)
	}
	_ = db.Exec(`INSERT INTO workspace_variables (id, workspace_id, variable_id, key, value, version, is_deleted)
		VALUES (1, 'ws-a', 'vid-1', 'k', 'v', 1, 0)`)

	svc := NewWorkspaceVariableService(db)

	// update other workspace fails
	if _, err := svc.UpdateVariableInWorkspace("ws-b", 1, 1, map[string]interface{}{"value": "x"}); err == nil {
		t.Fatal("update cross-ws must fail")
	}
	// delete other workspace fails
	if err := svc.DeleteVariableInWorkspace("ws-b", 1); err == nil {
		t.Fatal("delete cross-ws must fail")
	}
	// by variable_id
	if _, err := svc.GetLatestByVariableIDInWorkspace("ws-b", "vid-1"); err == nil {
		t.Fatal("get by variable_id cross-ws must fail")
	}
	if _, err := svc.GetLatestByVariableIDInWorkspace("ws-a", "vid-1"); err != nil {
		t.Fatal(err)
	}
}
