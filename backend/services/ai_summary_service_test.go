package services

import (
	"encoding/json"
	"iac-platform/internal/models"
	"testing"
	"time"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupSummaryTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	// Migrate tables
	db.AutoMigrate(&models.AIConfig{}, &models.WorkspaceTask{}, &models.AIPlanSummary{}, &models.AIApplySummary{})
	return db
}

func TestGeneratePlanSummary_NoAIConfig(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create a task with changes but NO AI config
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     2,
		ChangesChange:  1,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	// Verify no record created
	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no plan summary records, got %d", count)
	}
}

func TestGeneratePlanSummary_NoChanges(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create AI config
	db.Create(&models.AIConfig{
		ServiceType:  "bedrock",
		ModelID:      "test-model",
		AWSRegion:    "us-west-2",
		Enabled:      false,
		Capabilities: models.StringArray{"summary"},
	})

	// Create task with zero changes
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     0,
		ChangesChange:  0,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 0 {
		t.Errorf("expected no plan summary records, got %d", count)
	}
}

func TestGeneratePlanSummary_DuplicatePrevention(t *testing.T) {
	db := setupSummaryTestDB(t)

	// Create AI config
	db.Create(&models.AIConfig{
		ServiceType:  "bedrock",
		ModelID:      "test-model",
		AWSRegion:    "us-west-2",
		Enabled:      false,
		Capabilities: models.StringArray{"summary"},
	})

	// Create task
	task := models.WorkspaceTask{
		WorkspaceID:    "ws-test",
		Status:         "success",
		ChangesAdd:     3,
		ChangesChange:  0,
		ChangesDestroy: 0,
	}
	db.Create(&task)

	// Pre-create existing summary
	db.Create(&models.AIPlanSummary{
		ID:          "plsm-existing12345678",
		TaskID:      task.ID,
		WorkspaceID: "ws-test",
		Status:      "completed",
		CreatedAt:   time.Now(),
	})

	svc := NewAISummaryService(db)
	svc.GeneratePlanSummary(task.ID)

	// Should still be just 1 record
	var count int64
	db.Model(&models.AIPlanSummary{}).Count(&count)
	if count != 1 {
		t.Errorf("expected 1 plan summary record (existing), got %d", count)
	}
}

func TestGetPlanSummary(t *testing.T) {
	db := setupSummaryTestDB(t)

	db.Create(&models.AIPlanSummary{
		ID:              "plsm-test1234567890",
		TaskID:          42,
		WorkspaceID:     "ws-test",
		Status:          "completed",
		ChangesOverview: "test overview",
		CreatedAt:       time.Now(),
	})

	svc := NewAISummaryService(db)

	// Found
	result := svc.GetPlanSummary(42)
	if result == nil {
		t.Fatal("expected plan summary, got nil")
	}
	if result.ChangesOverview != "test overview" {
		t.Errorf("wrong overview: %s", result.ChangesOverview)
	}

	// Not found
	result = svc.GetPlanSummary(999)
	if result != nil {
		t.Error("expected nil for non-existent task")
	}
}

func TestInferServiceDisruption_CMDBNotFound(t *testing.T) {
	// AI flagged dependency_break but omitted service_disruption; CMDB shows resource not found
	factors := []string{"permission_scope_change", "dependency_break"}
	counts := map[string]int{"permission_scope_change": 1, "dependency_break": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "params": {"query": "existing-role"}, "result": {"found": true}},
		{"tool_name": "query_resource_attributes", "params": {"query": "missing-role"}, "result": {"found": false}}
	]`)
	impact := []byte(`{"details": [{"action": "update", "resource": "aws_s3_bucket_policy.this"}]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	factorSet := make(map[string]bool)
	for _, f := range got {
		factorSet[f] = true
	}
	if !factorSet["service_disruption"] {
		t.Error("R2 should have inferred service_disruption when CMDB resource not found")
	}
	if counts["service_disruption"] != 1 {
		t.Errorf("expected service_disruption count=1, got %d", counts["service_disruption"])
	}
}

func TestInferServiceDisruption_AllFound(t *testing.T) {
	// CMDB lookups all found=true - should NOT inject
	factors := []string{"permission_scope_change", "dependency_break"}
	counts := map[string]int{"permission_scope_change": 1, "dependency_break": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "result": {"found": true}},
		{"tool_name": "query_resource_attributes", "result": {"found": true}}
	]`)
	impact := []byte(`{"details": [{"action": "update", "resource": "aws_s3_bucket_policy.this"}]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	for _, f := range got {
		if f == "service_disruption" {
			t.Error("should not infer service_disruption when all CMDB lookups found")
		}
	}
}

func TestInferServiceDisruption_NoPrerequisiteFactor(t *testing.T) {
	// CMDB not found but no dependency_break or permission_scope_change
	factors := []string{"configuration_drift"}
	counts := map[string]int{"configuration_drift": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "params": {"query": "some-resource"}, "result": {"found": false}}
	]`)
	impact := []byte(`{"details": [{"action": "update", "resource": "aws_s3_bucket.this"}]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	for _, f := range got {
		if f == "service_disruption" {
			t.Error("should not infer service_disruption without prerequisite factors")
		}
	}
}

func TestInferServiceDisruption_AlreadyFlagged(t *testing.T) {
	// service_disruption already present - should not duplicate
	factors := []string{"dependency_break", "service_disruption"}
	counts := map[string]int{"dependency_break": 1, "service_disruption": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "params": {"query": "some-resource"}, "result": {"found": false}}
	]`)
	impact := []byte(`{"details": [{"action": "update", "resource": "aws_s3_bucket_policy.this"}]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	count := 0
	for _, f := range got {
		if f == "service_disruption" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 service_disruption, got %d", count)
	}
}

func TestInferServiceDisruption_CreateOnly(t *testing.T) {
	// All conditions met except actions are pure create - should NOT inject
	factors := []string{"permission_scope_change", "dependency_break"}
	counts := map[string]int{"permission_scope_change": 1, "dependency_break": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "params": {"query": "some-resource"}, "result": {"found": false}}
	]`)
	impact := []byte(`{"details": [
		{"action": "create", "resource": "aws_s3_bucket.new"},
		{"action": "create", "resource": "aws_s3_bucket_policy.new"}
	]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	for _, f := range got {
		if f == "service_disruption" {
			t.Error("should not infer service_disruption for pure create actions")
		}
	}
}

func TestInferServiceDisruption_MixedActions(t *testing.T) {
	// Has both create and update - update should trigger inference
	factors := []string{"dependency_break"}
	counts := map[string]int{"dependency_break": 1}
	toolCalls := []byte(`[
		{"tool_name": "query_resource_attributes", "params": {"query": "some-resource"}, "result": {"found": false}}
	]`)
	impact := []byte(`{"details": [
		{"action": "create", "resource": "aws_s3_bucket.new"},
		{"action": "update", "resource": "aws_s3_bucket_policy.existing"}
	]}`)

	got := inferServiceDisruption(factors, counts, toolCalls, impact)

	factorSet := make(map[string]bool)
	for _, f := range got {
		factorSet[f] = true
	}
	if !factorSet["service_disruption"] {
		t.Error("R2 should trigger when mixed actions include update")
	}
}

func TestHasCMDBNotFound(t *testing.T) {
	tests := []struct {
		name string
		raw  []byte
		want bool
	}{
		{"empty", nil, false},
		{"no match tool", []byte(`[{"tool_name": "query_cmdb_dependencies", "result": {"found": false}}]`), false},
		{"found true", []byte(`[{"tool_name": "query_resource_attributes", "params": {"query": "role-a"}, "result": {"found": true}}]`), false},
		// found=false with no code diff → fall back to trigger
		{"found false no diff", []byte(`[{"tool_name": "query_resource_attributes", "params": {"query": "role-a"}, "result": {"found": false}}]`), true},
		// found=false but query only in old code → corrective, don't trigger
		{"corrective old ref", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"policy": "arn:aws:iam::123:role/ec2-instance-rolese"}], "new": [{"policy": "arn:aws:iam::123:role/ec2-instance-role"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "ec2-instance-rolese"}, "result": {"found": false}},
			{"tool_name": "query_resource_attributes", "params": {"query": "ec2-instance-role"}, "result": {"found": true}}
		]`), false},
		// found=false and query in new code → real risk
		{"new ref not found", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"role": "old-role"}], "new": [{"role": "new-role-typo"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "new-role-typo"}, "result": {"found": false}}
		]`), true},
		// found=false, query in both old and new → conservative, trigger
		{"ambiguous both", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"role": "shared-role"}], "new": [{"role": "shared-role"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "shared-role"}, "result": {"found": false}}
		]`), true},
		// found=false, query in neither old nor new → conservative, trigger
		{"not in code", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"role": "aaa"}], "new": [{"role": "bbb"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "unknown-role"}, "result": {"found": false}}
		]`), true},
		// multiple corrective refs, all old-only → don't trigger
		{"multiple corrective", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"p": "role-a-typo and role-b-typo"}], "new": [{"p": "role-a and role-b"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "role-a-typo"}, "result": {"found": false}},
			{"tool_name": "query_resource_attributes", "params": {"query": "role-b-typo"}, "result": {"found": false}},
			{"tool_name": "query_resource_attributes", "params": {"query": "role-a"}, "result": {"found": true}},
			{"tool_name": "query_resource_attributes", "params": {"query": "role-b"}, "result": {"found": true}}
		]`), false},
		// one corrective + one real risk → trigger
		{"mixed corrective and risk", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": true, "diff": [{"old": [{"p": "old-typo"}], "new": [{"p": "new-missing"}]}]}},
			{"tool_name": "query_resource_attributes", "params": {"query": "old-typo"}, "result": {"found": false}},
			{"tool_name": "query_resource_attributes", "params": {"query": "new-missing"}, "result": {"found": false}}
		]`), true},
		// code diff has_changes=false → no leaves, fall back
		{"no changes in diff", []byte(`[
			{"tool_name": "query_resource_code_diff", "result": {"has_changes": false}},
			{"tool_name": "query_resource_attributes", "params": {"query": "some-role"}, "result": {"found": false}}
		]`), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := hasCMDBNotFound(tt.raw); got != tt.want {
				t.Errorf("hasCMDBNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCollectStringLeaves(t *testing.T) {
	raw := json.RawMessage(`{"a": "hello", "b": {"c": "world", "d": 123}, "e": ["foo", "bar", 456]}`)
	leaves := collectStringLeaves(raw)

	expected := map[string]bool{"hello": false, "world": false, "foo": false, "bar": false}
	for _, l := range leaves {
		if _, ok := expected[l]; ok {
			expected[l] = true
		}
	}
	for k, found := range expected {
		if !found {
			t.Errorf("expected leaf %q not found", k)
		}
	}
	// Should not contain numbers
	for _, l := range leaves {
		if l == "123" || l == "456" {
			t.Errorf("number %q should not be in string leaves", l)
		}
	}
}

func TestRetryPlanSummary_OnlyFailedAllowed(t *testing.T) {
	db := setupSummaryTestDB(t)

	db.Create(&models.AIPlanSummary{
		ID:          "plsm-completed123456",
		TaskID:      42,
		WorkspaceID: "ws-test",
		Status:      "completed",
		CreatedAt:   time.Now(),
	})

	svc := NewAISummaryService(db)
	err := svc.RetryPlanSummary(42)
	if err == nil {
		t.Error("expected error when retrying completed summary")
	}
}
