package services

import (
	"bytes"
	"context"
	"errors"
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
	if code, msg := classifyTaskFailure(err, "x"); code != models.TaskErrorCodePlanExpired || msg == "x" {
		t.Fatalf("missing plan: code=%q msg=%q", code, msg)
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
