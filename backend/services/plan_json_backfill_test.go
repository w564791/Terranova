package services

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// PG: historical plan_json rows are rewritten in place with the same
// RedactPlanJSON (HCL markers + the task's platform-side set), marked, and a
// second run touches nothing.
func TestBackfillPlanJSONRedaction_PG(t *testing.T) {
	db := setupVarsetTestDB(t)
	db.Exec(`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_json_redaction_version smallint`)
	db.Exec(`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS sensitive_keys jsonb`)
	db.Exec(`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS variable_overrides jsonb`)
	defer db.Exec(`DELETE FROM workspace_tasks WHERE workspace_id = 'ws-backfill'`)

	insert := func(plan string, overrides string) uint {
		var id uint
		var ov interface{}
		if overrides != "" {
			ov = overrides
		}
		if err := db.Raw(`INSERT INTO workspace_tasks (workspace_id, task_type, status, execution_mode, plan_json, variable_overrides)
			VALUES ('ws-backfill', 'plan', 'success', 'local', CAST(? AS jsonb), CAST(? AS jsonb)) RETURNING id`, plan, ov).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	hcl := insert(`{"variables": {"db_password": {"value": "hcl-secret-1"}},
		"configuration": {"root_module": {"variables": {"db_password": {"sensitive": true}}}},
		"resource_changes": [{"address": "a.b", "change": {"before": null, "after": {"password": "hcl-secret-1"}, "after_sensitive": {"password": true}}}]}`, "")
	ovr := insert(`{"variables": {"ovr": {"value": "override-secret-xyz"}},
		"resource_changes": [{"address": "a.c", "change": {"before": null, "after": {"tag": "override-secret-xyz"}, "after_sensitive": {}}}]}`,
		`{"ovr": "override-secret-xyz"}`) // sensitive_keys NULL => sensitive
	clean := insert(`{"variables": {"region": {"value": "eu-west-1"}}}`, "")

	res, err := BackfillPlanJSONRedaction(context.Background(), db, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rewritten < 2 || res.Unchanged < 1 {
		t.Fatalf("result %+v", res)
	}
	read := func(id uint) (string, *int) {
		var row struct {
			Plan    string
			Version *int
		}
		db.Raw(`SELECT plan_json::text AS plan, plan_json_redaction_version AS version FROM workspace_tasks WHERE id = ?`, id).Scan(&row)
		return row.Plan, row.Version
	}
	for _, id := range []uint{hcl, ovr, clean} {
		plan, v := read(id)
		if v == nil || *v != PlanJSONRedactionVersion {
			t.Errorf("task %d not marked", id)
		}
		if strings.Contains(plan, "hcl-secret-1") || strings.Contains(plan, "override-secret-xyz") {
			t.Errorf("task %d still holds a secret: %s", id, plan)
		}
	}
	if plan, _ := read(clean); !strings.Contains(plan, "eu-west-1") {
		t.Error("clean plan altered")
	}
	var m map[string]interface{}
	p, _ := read(ovr)
	json.Unmarshal([]byte(p), &m)
	if m["variables"].(map[string]interface{})["ovr"].(map[string]interface{})["value"] != SensitivePlaceholder {
		t.Fatalf("override variable not masked with the marker: %s", p)
	}

	again, err := BackfillPlanJSONRedaction(context.Background(), db, 2)
	if err != nil || again.Scanned != 0 {
		t.Fatalf("re-run touched rows: %+v %v", again, err)
	}
	// marker reset (e.g. a version bump): re-redaction is a no-op rewrite
	db.Exec(`UPDATE workspace_tasks SET plan_json_redaction_version = NULL WHERE id = ?`, hcl)
	before, _ := read(hcl)
	third, err := BackfillPlanJSONRedaction(context.Background(), db, 2)
	after, _ := read(hcl)
	if err != nil || third.Unchanged != 1 || third.Rewritten != 0 || before != after {
		t.Fatalf("idempotence: %+v %v", third, err)
	}
}
