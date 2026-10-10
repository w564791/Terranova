package handlers

import (
	"context"
	"encoding/json"
	"testing"

	"iac-platform/internal/models"
	"iac-platform/services"
)

type mismatchAudit struct {
	Action    string
	NewValues string
}

func mismatchAudits(t *testing.T, e *bundleEnv) []map[string]interface{} {
	t.Helper()
	var rows []mismatchAudit
	e.db.Raw(`SELECT action, new_values FROM audit_logs WHERE action = 'version.bundle_hash_mismatch' ORDER BY id`).Scan(&rows)
	var out []map[string]interface{}
	for _, r := range rows {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(r.NewValues), &m); err != nil {
			t.Fatal(err)
		}
		out = append(out, m)
	}
	return out
}

// An agent-reported bundle hash mismatch is audited with source agent; the
// platform re-hashes the stored files and leaves an intact version valid.
func TestAgentBundleHashMismatch_IntactVersionNotMarked(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`INSERT INTO manifest_deployments (id, manifest_id, version_id, workspace_id, status) VALUES ('mfd-pub', 'mf-1', ?, 'ws-new', 'active')`, id)
	e.db.Exec(`UPDATE workspaces SET manifest_deployment_id = 'mfd-pub', manifest_active_tag = 'v2.0.0' WHERE workspace_id = 'ws-new'`)

	task := &models.WorkspaceTask{ID: 9, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan}
	marked, err := services.RecordAgentBundleHashMismatch(context.Background(), e.db, task, "agent-b")
	if err != nil || marked {
		t.Fatalf("intact version: marked=%v err=%v", marked, err)
	}
	if h, r := versionRow(t, e.db, id); h == nil || r != nil {
		t.Fatalf("agent report alone must not mark the version: %v %v", h, r)
	}
	a := mismatchAudits(t, e)
	if len(a) != 1 || a[0]["source"] != "agent" || a[0]["agent_id"] != "agent-b" || a[0]["version_id"] != id ||
		a[0]["version_marked"] != false || a[0]["manifest_id"] != "mf-1" {
		t.Fatalf("audit: %+v", a)
	}
}

// When the platform's own re-check fails too, the version is marked
// hash_mismatch (with the platform's own audit row).
func TestAgentBundleHashMismatch_PlatformRecheckMarks(t *testing.T) {
	e := newBundleEnv(t)
	e.putDraft(t, "u1", "main.tf", pubMain)
	id, _ := e.publish(t, "v2.0.0")
	e.db.Exec(`UPDATE manifest_files SET content = CAST('resource "null_resource" "evil" {}' AS BLOB) WHERE version_id = ?`, id)

	// a Run task of that version (external_files.manifest_version_id)
	task := &models.WorkspaceTask{ID: 11, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan,
		ExternalFiles: models.JSONB{"files": []interface{}{map[string]interface{}{"path": "main.tf"}}, "manifest_version_id": id}}
	marked, err := services.RecordAgentBundleHashMismatch(context.Background(), e.db, task, "agent-b")
	if err != nil || !marked {
		t.Fatalf("tampered version: marked=%v err=%v", marked, err)
	}
	if h, r := versionRow(t, e.db, id); h != nil || r == nil || *r != "hash_mismatch" {
		t.Fatalf("platform re-check must mark the version: %v %v", h, r)
	}
	a := mismatchAudits(t, e)
	if len(a) != 2 || a[0]["source"] != "agent" || a[1]["source"] == "agent" {
		t.Fatalf("want agent audit then platform re-check audit: %+v", a)
	}
}

// A draft Run has no version: audit only.
func TestAgentBundleHashMismatch_DraftRunAuditOnly(t *testing.T) {
	e := newBundleEnv(t)
	task := &models.WorkspaceTask{ID: 12, WorkspaceID: "ws-new", TaskType: models.TaskTypePlan,
		ExternalFiles: models.JSONB{"files": []interface{}{map[string]interface{}{"path": "main.tf"}}, "bundle_hash": "x"}}
	if marked, err := services.RecordAgentBundleHashMismatch(context.Background(), e.db, task, "agent-b"); err != nil || marked {
		t.Fatalf("draft run: %v %v", marked, err)
	}
	if a := mismatchAudits(t, e); len(a) != 1 || a[0]["version_id"] != "" {
		t.Fatalf("audit: %+v", a)
	}
}
