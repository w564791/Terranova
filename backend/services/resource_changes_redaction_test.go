package services

import (
	"context"
	"strings"
	"testing"

	"iac-platform/internal/models"
)

// PG: a value sensitive only in a varset (not in HCL, not flagged by the
// provider) never reaches workspace_task_resource_changes: neither on a new
// write (platform derivation from plan_json) nor after the backfill of legacy
// agent-uploaded rows. Rows without a plan_json to derive from are purged.
func TestResourceChanges_VarsetSensitive_PG(t *testing.T) {
	db := setupVarsetTestDB(t)
	for _, stmt := range []string{
		`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS plan_json_redaction_version smallint`,
		`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS sensitive_keys jsonb`,
		`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS variable_overrides jsonb`,
		`ALTER TABLE workspace_tasks ADD COLUMN IF NOT EXISTS error_reason character varying(64)`,
		`ALTER TABLE workspace_task_resource_changes ADD COLUMN IF NOT EXISTS redaction_version smallint`,
		`ALTER TABLE workspace_task_resource_changes ADD COLUMN IF NOT EXISTS details_purged boolean DEFAULT false`,
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	ws := "ws-rc-redact"
	db.Exec("INSERT INTO workspaces (workspace_id, name, execution_mode, terraform_version, workdir, state_backend) VALUES (?, 'rc-redact', 'local', 'latest', '/workspace', 'local') ON CONFLICT (workspace_id) DO NOTHING", ws)
	defer db.Exec("DELETE FROM workspaces WHERE workspace_id = ?", ws)
	defer db.Exec("DELETE FROM workspace_task_resource_changes WHERE workspace_id = ?", ws)
	defer db.Exec("DELETE FROM workspace_tasks WHERE workspace_id = ?", ws)

	vs, err := NewVariableSetService(db).Create("rc-redact-global", "test", "global", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanupVarset(db, vs.VarsetID)
	if _, err := NewVarsetVariableService(db).Create(vs.VarsetID, "api_key", platformSecret, "",
		models.VariableTypeTerraform, models.ValueFormatString, true, nil); err != nil {
		t.Fatal(err)
	}
	vsnap, _, err := NewVariableSnapshotService(db).CreateSnapshot(ws, nil)
	if err != nil || vsnap == nil {
		t.Fatalf("snapshot: %v", err)
	}
	defer db.Exec("DELETE FROM variable_snapshots WHERE vsnap_id = ?", *vsnap)

	// raw plan (as an older agent / pre-redaction row would hold it): the
	// varset secret flows into a resource attribute without any sensitivity flag
	rawPlan := `{"variables": {"api_key": {"value": "` + platformSecret + `"}},
		"resource_changes": [{"address": "aws_s3_bucket.b", "type": "aws_s3_bucket", "name": "b",
		  "change": {"actions": ["update"], "before": {"tag": "old"}, "after": {"tag": "` + platformSecret + `", "region": "eu-west-1"},
		  "after_unknown": {"arn": true}, "after_sensitive": {}}}]}`
	newTask := func(plan *string) uint {
		var id uint
		var p interface{}
		if plan != nil {
			p = *plan
		}
		if err := db.Raw(`INSERT INTO workspace_tasks (workspace_id, task_type, status, execution_mode, plan_json, variable_snapshot_id)
			VALUES (?, 'plan', 'success', 'agent', CAST(? AS jsonb), ?) RETURNING id`, ws, p, *vsnap).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	type rcRow struct {
		ID               uint
		ResourceAddress  string
		Action           string
		ApplyStatus      string
		Before, After    *string
		AfterUnknown     *string
		RedactionVersion *int
		DetailsPurged    bool
	}
	rows := func(taskID uint) []rcRow {
		var out []rcRow
		db.Raw(`SELECT id, resource_address, action, apply_status, changes_before::text AS before, changes_after::text AS after,
			after_unknown::text AS after_unknown, redaction_version, details_purged
			FROM workspace_task_resource_changes WHERE task_id = ? ORDER BY id`, taskID).Scan(&out)
		return out
	}
	noSecret := func(where string, rs []rcRow) {
		for _, r := range rs {
			for _, v := range []*string{r.Before, r.After, r.AfterUnknown} {
				if v != nil && strings.Contains(*v, platformSecret) {
					t.Fatalf("%s: varset secret in resource change %d: %s", where, r.ID, *v)
				}
			}
		}
	}

	// 1. new write: derived by the platform from plan_json
	t1 := newTask(&rawPlan)
	var task models.WorkspaceTask
	if err := db.First(&task, t1).Error; err != nil {
		t.Fatal(err)
	}
	if n, err := NewPlanParserService(db).StoreResourceChangesFromPlanJSON(&task); err != nil || n != 1 {
		t.Fatalf("derive: n=%d err=%v", n, err)
	}
	got := rows(t1)
	noSecret("new write", got)
	if len(got) != 1 || got[0].After == nil || !strings.Contains(*got[0].After, "eu-west-1") ||
		got[0].RedactionVersion == nil || *got[0].RedactionVersion != int(ResourceChangesRedactionVersion) || got[0].DetailsPurged {
		t.Fatalf("new write row: %+v", got)
	}

	// 2. legacy rows (raw agent upload, unmarked)
	legacy := func(taskID uint, addr string) {
		if err := db.Exec(`INSERT INTO workspace_task_resource_changes
			(task_id, workspace_id, resource_address, resource_type, resource_name, action, changes_before, changes_after, after_unknown, apply_status, created_at, updated_at)
			VALUES (?, ?, ?, 'aws_s3_bucket', 'b', 'update', CAST(? AS jsonb), CAST(? AS jsonb), CAST(? AS jsonb), 'completed', now(), now())`,
			taskID, ws, addr, `{"tag":"old"}`, `{"tag":"`+platformSecret+`"}`, `{"note":"`+platformSecret+`"}`).Error; err != nil {
			t.Fatal(err)
		}
	}
	t2 := newTask(&rawPlan) // has plan_json: re-derived
	legacy(t2, "aws_s3_bucket.b")
	legacy(t2, "aws_s3_bucket.gone") // not in the plan: purged
	t3 := newTask(nil)               // no plan_json: purged
	legacy(t3, "aws_s3_bucket.b")

	res, err := BackfillResourceChangeRedaction(context.Background(), db, 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.Rederived != 1 || res.Purged != 2 {
		t.Fatalf("backfill result %+v", res)
	}
	r2, r3 := rows(t2), rows(t3)
	noSecret("backfill", append(r2, r3...))
	if r2[0].After == nil || !strings.Contains(*r2[0].After, "eu-west-1") || r2[0].DetailsPurged || r2[0].ApplyStatus != "completed" {
		t.Fatalf("re-derived row: %+v", r2[0])
	}
	for _, r := range []rcRow{r2[1], r3[0]} {
		if r.Before != nil || r.After != nil || r.AfterUnknown != nil || !r.DetailsPurged ||
			r.ResourceAddress == "" || r.Action != "update" || r.ApplyStatus != "completed" {
			t.Fatalf("purged row must keep address/action and null all details: %+v", r)
		}
	}
	for _, r := range append(r2, r3...) {
		if r.RedactionVersion == nil || *r.RedactionVersion != int(ResourceChangesRedactionVersion) {
			t.Fatalf("row %d not marked", r.ID)
		}
	}
	// the API shape carries details_purged
	var api []models.WorkspaceTaskResourceChange
	db.Where("task_id = ?", t3).Find(&api)
	if len(api) != 1 || !api[0].DetailsPurged {
		t.Fatalf("model DetailsPurged: %+v", api)
	}

	again, err := BackfillResourceChangeRedaction(context.Background(), db, 1)
	if err != nil || again.Tasks != 0 {
		t.Fatalf("re-run touched rows: %+v %v", again, err)
	}
}
