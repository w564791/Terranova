package services

import (
	"errors"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestCMDBWorkspaceCountsAreOrganizationScopedAndRejectDuplicateBindings(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=private"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	for _, statement := range []string{
		"CREATE TABLE projects (id INTEGER PRIMARY KEY, org_id INTEGER NOT NULL)",
		"CREATE TABLE workspace_project_relations (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL, project_id INTEGER NOT NULL)",
		"CREATE TABLE workspaces (workspace_id TEXT NOT NULL, name TEXT NOT NULL)",
		"CREATE TABLE resource_index (id INTEGER PRIMARY KEY, workspace_id TEXT NOT NULL, resource_mode TEXT NOT NULL, last_synced_at DATETIME)",
		"CREATE TABLE cmdb_sync_logs (source_type TEXT, source_id TEXT, source_name TEXT, triggered_by TEXT, status TEXT, started_at DATETIME, completed_at DATETIME, resources_synced INTEGER, resources_added INTEGER, resources_updated INTEGER, resources_deleted INTEGER, error_message TEXT)",
		"INSERT INTO projects (id, org_id) VALUES (1, 1), (2, 2)",
		"INSERT INTO workspace_project_relations (id, workspace_id, project_id) VALUES (1, 'ws-org-1', 1), (2, 'ws-org-2', 2), (3, 'ws-corrupt', 1), (4, 'ws-corrupt', 2)",
		"INSERT INTO workspaces (workspace_id, name) VALUES ('ws-org-1', 'Org 1'), ('ws-org-2', 'Org 2'), ('ws-corrupt', 'Corrupt')",
		"INSERT INTO resource_index (id, workspace_id, resource_mode) VALUES (1, 'ws-org-1', 'managed'), (2, 'ws-org-2', 'managed'), (3, 'ws-corrupt', 'managed')",
		"INSERT INTO cmdb_sync_logs (source_type, source_id, source_name, triggered_by, status, started_at) VALUES ('workspace', 'ws-org-1', 'Org 1', 'manual', 'success', CURRENT_TIMESTAMP), ('workspace', 'ws-org-2', 'Org 2', 'manual', 'success', CURRENT_TIMESTAMP), ('workspace', 'ws-corrupt', 'Corrupt', 'manual', 'success', CURRENT_TIMESTAMP)",
	} {
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("seed database: %v", err)
		}
	}

	service := NewCMDBService(db)
	counts, err := service.GetWorkspaceResourceCountsInOrganization(1)
	if err != nil {
		t.Fatalf("get scoped counts: %v", err)
	}
	if len(counts) != 1 || counts[0].WorkspaceID != "ws-org-1" || counts[0].ResourceCount != 1 {
		t.Fatalf("organization 1 must see only its valid workspace, got %+v", counts)
	}

	history, err := service.GetSyncHistoryInOrganization(1, 1, 10)
	if err != nil {
		t.Fatalf("get scoped sync history: %v", err)
	}
	if history.Total != 1 || len(history.Syncs) != 1 || history.Syncs[0].SourceID != "ws-org-1" {
		t.Fatalf("organization 1 must see only its valid sync log, got %+v", history)
	}

	if err := service.ensureWorkspaceInOrganization(1, "ws-corrupt"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("duplicate workspace relation must fail closed, got %v", err)
	}
}
