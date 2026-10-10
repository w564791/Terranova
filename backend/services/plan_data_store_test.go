package services

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"

	"iac-platform/internal/crypto"
	"iac-platform/internal/models"
)

// PG: one cleanup pass deletes plans of terminal and expired tasks, seals
// legacy plaintext of tasks that may still apply, and is idempotent.
func TestCleanupPlanData_PG(t *testing.T) {
	db := setupVarsetTestDB(t)
	plain := []byte("PK\x03\x04legacy plaintext plan secret")
	insert := func(status models.TaskStatus, data []byte) uint {
		var id uint
		if err := db.Raw(`INSERT INTO workspace_tasks (workspace_id, task_type, status, execution_mode, plan_data)
			VALUES ('ws-plan-data', 'plan_and_apply', ?, 'local', ?) RETURNING id`, string(status), data).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}
	defer db.Exec(`DELETE FROM workspace_tasks WHERE workspace_id = 'ws-plan-data'`)

	applied := insert(models.TaskStatusApplied, plain)
	legacy := insert(models.TaskStatusApplyPending, plain)
	expiredID := insert(models.TaskStatusApplyPending, nil)
	exp, _ := crypto.SealPlanData(expiredID, plain, time.Now().Add(-time.Minute))
	db.Exec(`UPDATE workspace_tasks SET plan_data = ? WHERE id = ?`, exp, expiredID)
	valid := insert(models.TaskStatusDecisionRequired, nil)
	ok, _ := SealTaskPlanData(valid, plain)
	db.Exec(`UPDATE workspace_tasks SET plan_data = ? WHERE id = ?`, ok, valid)
	planOnly := insert(models.TaskStatusSuccess, ok)

	res, err := CleanupPlanData(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if res.Sealed != 1 || res.Purged != 3 {
		t.Fatalf("result %+v, want sealed=1 purged=3", res)
	}
	load := func(id uint) *models.WorkspaceTask {
		var task models.WorkspaceTask
		db.Select("id, plan_data").First(&task, id)
		return &task
	}
	for _, id := range []uint{applied, expiredID, planOnly} {
		if len(load(id).PlanData) != 0 {
			t.Errorf("task %d kept plan_data", id)
		}
	}
	lt := load(legacy)
	if bytes.Contains(lt.PlanData, []byte("secret")) {
		t.Fatal("legacy plaintext not sealed")
	}
	if got, err := OpenTaskPlanData(lt); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("sealed legacy plan does not open: %v", err)
	}
	if got, err := OpenTaskPlanData(load(valid)); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("valid plan damaged: %v", err)
	}
	again, err := CleanupPlanData(context.Background(), db)
	if err != nil || again.Sealed != 0 || again.Purged != 0 {
		t.Fatalf("second pass not idempotent: %+v %v", again, err)
	}
}

func TestPlanBytes_LocalRequiresEnvelope(t *testing.T) {
	plain := []byte("PK\x03\x04plan")
	sealed, err := SealTaskPlanData(5, plain)
	if err != nil {
		t.Fatal(err)
	}
	local := &TerraformExecutor{db: testDBOrSkip(t)}
	if got, err := local.planBytes(&models.WorkspaceTask{ID: 5, PlanData: sealed}); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("local open: %v", err)
	}
	if _, err := local.planBytes(&models.WorkspaceTask{ID: 5, PlanData: plain}); !errors.Is(err, crypto.ErrPlanDataNotSealed) {
		t.Fatalf("local plaintext accepted: %v", err)
	}
	if _, err := local.planBytes(&models.WorkspaceTask{ID: 6, PlanData: sealed}); !errors.Is(err, crypto.ErrPlanDataIntegrity) {
		t.Fatalf("plan of another task accepted: %v", err)
	}
	agent := &TerraformExecutor{} // agent: plan arrives decrypted from the platform
	if got, err := agent.planBytes(&models.WorkspaceTask{ID: 5, PlanData: plain}); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("agent plan: %v", err)
	}
	_, err = agent.planBytes(&models.WorkspaceTask{ID: 5})
	if code, reason, msg := classifyTaskFailure(err, "x"); code != models.TaskErrorCodePlanExpired || reason != "plan_data_missing" || msg == "x" {
		t.Fatalf("missing plan: code=%q reason=%q msg=%q", code, reason, msg)
	}
	exp, _ := crypto.SealPlanData(5, plain, time.Now().Add(-time.Second))
	_, err = local.planBytes(&models.WorkspaceTask{ID: 5, PlanData: exp})
	if TaskErrorCode(err) != models.TaskErrorCodePlanExpired {
		t.Fatalf("expired plan: %v", err)
	}
}

func testDBOrSkip(t *testing.T) *gorm.DB {
	t.Helper()
	return setupVarsetTestDB(t)
}

func TestPlanDataTTL_Env(t *testing.T) {
	t.Setenv(PlanDataTTLEnv, "")
	if PlanDataTTL() != DefaultPlanDataTTL {
		t.Fatal("default TTL")
	}
	t.Setenv(PlanDataTTLEnv, "72h")
	if PlanDataTTL() != 72*time.Hour {
		t.Fatal("env TTL")
	}
	t.Setenv(PlanDataTTLEnv, "-1h")
	if PlanDataTTL() != DefaultPlanDataTTL {
		t.Fatal("invalid TTL must fall back")
	}
}

// PG: legacy (key version 0, JWT_SECRET-rooted) envelopes are re-sealed under
// DATA_ENCRYPTION_KEY with their expiry kept; a second pass changes nothing;
// the startup check requires JWT_SECRET only while such rows remain.
func TestCleanupPlanData_ReencryptsLegacyEnvelopes_PG(t *testing.T) {
	db := setupVarsetTestDB(t)
	ctx := context.Background()
	plain := []byte("PK\x03\x04plan with secret value")
	for _, stmt := range []string{
		"ALTER TABLE workspace_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
		"ALTER TABLE varset_variables ADD COLUMN IF NOT EXISTS key_version smallint NOT NULL DEFAULT 0",
	} {
		if err := db.Exec(stmt).Error; err != nil {
			t.Fatal(err)
		}
	}
	db.Exec(`DELETE FROM workspace_variables WHERE sensitive AND key_version = 0`)
	db.Exec(`DELETE FROM varset_variables WHERE sensitive AND key_version = 0`)
	db.Exec(`DELETE FROM workspace_tasks WHERE plan_data IS NOT NULL AND get_byte(plan_data, 4) = 1`)
	defer db.Exec(`DELETE FROM workspace_tasks WHERE workspace_id = 'ws-plan-reenc'`)
	insert := func() uint {
		var id uint
		if err := db.Raw(`INSERT INTO workspace_tasks (workspace_id, task_type, status, execution_mode)
			VALUES ('ws-plan-reenc', 'plan_and_apply', 'apply_pending', 'local') RETURNING id`).Scan(&id).Error; err != nil {
			t.Fatal(err)
		}
		return id
	}

	// legacy envelope, written before DATA_ENCRYPTION_KEY existed
	t.Setenv("JWT_SECRET", "plan-reenc-legacy-secret")
	t.Setenv("ENV", "development")
	t.Setenv("DATA_ENCRYPTION_KEY", "")
	id := insert()
	expires := time.Now().Add(2 * time.Hour).Truncate(time.Second)
	legacyBlob, err := crypto.SealPlanData(id, plain, expires)
	if err != nil {
		t.Fatal(err)
	}
	if kv, _ := crypto.PlanDataKeyVersion(legacyBlob); kv != 0 {
		t.Fatalf("legacy envelope key version %d", kv)
	}
	db.Exec(`UPDATE workspace_tasks SET plan_data = ? WHERE id = ?`, legacyBlob, id)

	t.Setenv("ENV", "production")
	t.Setenv("DATA_ENCRYPTION_KEY", reencTestKey(t))
	t.Setenv("DATA_ENCRYPTION_KEY_VERSION", "")

	if n, err := CountLegacyPlanDataRows(ctx, db); err != nil || n != 1 {
		t.Fatalf("legacy plan_data rows = %d, %v", n, err)
	}
	t.Setenv("JWT_SECRET", "")
	if err := CheckLegacyKeyAvailable(ctx, db); err == nil || !strings.Contains(err.Error(), "workspace_tasks.plan_data=1") {
		t.Fatalf("startup check with a legacy plan_data envelope and no JWT_SECRET: %v", err)
	}
	t.Setenv("JWT_SECRET", "plan-reenc-legacy-secret")

	res, err := CleanupPlanData(ctx, db)
	if err != nil || res.Reencrypted != 1 {
		t.Fatalf("cleanup: %+v %v", res, err)
	}
	var task models.WorkspaceTask
	db.Select("id, plan_data").First(&task, id)
	if kv, ok := crypto.PlanDataKeyVersion(task.PlanData); !ok || kv != 1 {
		t.Fatalf("re-encrypted key version %d %v", kv, ok)
	}
	if _, exp := crypto.ParsePlanDataHeader(task.PlanData); !exp.Equal(expires) {
		t.Fatalf("expiry changed: %v -> %v", expires, exp)
	}
	// JWT_SECRET no longer involved
	t.Setenv("JWT_SECRET", "")
	if got, err := OpenTaskPlanData(&task); err != nil || !bytes.Equal(got, plain) {
		t.Fatalf("re-encrypted plan does not open without JWT_SECRET: %v", err)
	}
	if err := CheckLegacyKeyAvailable(ctx, db); err != nil {
		t.Fatalf("startup check after re-encryption: %v", err)
	}
	before := task.PlanData
	again, err := CleanupPlanData(ctx, db)
	if err != nil || again.Reencrypted != 0 {
		t.Fatalf("second pass: %+v %v", again, err)
	}
	db.Select("id, plan_data").First(&task, id)
	if !bytes.Equal(before, task.PlanData) {
		t.Fatal("second pass rewrote an already re-encrypted envelope")
	}
}
