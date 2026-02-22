package services

import (
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// newTestSolver creates a SchemaSolver with schema pre-loaded
// Uses in-memory SQLite to satisfy the non-nil db check in Solve()
func newTestSolver(schema map[string]*SchemaFieldDef) *SchemaSolver {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		panic("failed to open test db: " + err.Error())
	}
	return &SchemaSolver{
		db:     db,
		schema: schema,
	}
}

// intPtr helper for *int fields
func intPtr(v int) *int { return &v }

// float64Ptr helper for *float64 fields
func float64Ptr(v float64) *float64 { return &v }

// ========== Solve — enum validation ==========

func TestSolve_EnumValid(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"engine": {Type: "string", Options: []interface{}{"mysql", "postgres", "aurora"}},
	})

	result := solver.Solve(map[string]interface{}{"engine": "postgres"})
	if !result.Success {
		t.Fatalf("expected success, got feedbacks: %v", feedbackMessages(result))
	}
}

func TestSolve_EnumInvalid(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"engine": {Type: "string", Options: []interface{}{"mysql", "postgres"}},
	})

	result := solver.Solve(map[string]interface{}{"engine": "sqlite"})
	if result.Success {
		t.Fatal("expected failure for invalid enum value")
	}
	if !result.NeedAIFix {
		t.Error("expected NeedAIFix=true")
	}
	assertFeedbackField(t, result, "engine")
}

// ========== Solve — type validation ==========

func TestSolve_TypeMismatch(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"count": {Type: "number"},
	})

	result := solver.Solve(map[string]interface{}{"count": "not-a-number"})
	if result.Success {
		t.Fatal("expected failure for type mismatch")
	}
	assertFeedbackField(t, result, "count")
}

func TestSolve_TypeCompatible_IntegerAcceptsNumber(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"port": {Type: "integer"},
	})

	// JSON numbers are float64 in Go
	result := solver.Solve(map[string]interface{}{"port": float64(8080)})
	if !result.Success {
		t.Fatalf("integer should accept float64: %v", feedbackMessages(result))
	}
}

func TestSolve_TypeCompatible_ObjectAndMap(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"tags": {Type: "map"},
	})

	result := solver.Solve(map[string]interface{}{
		"tags": map[string]interface{}{"env": "prod"},
	})
	if !result.Success {
		t.Fatalf("map type should accept Go map: %v", feedbackMessages(result))
	}
}

// ========== Solve — string constraints ==========

func TestSolve_StringMinLength(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"name": {Type: "string", MinLength: intPtr(3)},
	})

	result := solver.Solve(map[string]interface{}{"name": "ab"})
	if result.Success {
		t.Fatal("expected failure for string too short")
	}
	assertFeedbackField(t, result, "name")
}

func TestSolve_StringMaxLength(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"name": {Type: "string", MaxLength: intPtr(5)},
	})

	result := solver.Solve(map[string]interface{}{"name": "toolongname"})
	if result.Success {
		t.Fatal("expected failure for string too long")
	}
	assertFeedbackField(t, result, "name")
}

func TestSolve_StringPattern(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"bucket": {Type: "string", Pattern: `^[a-z0-9-]+$`},
	})

	tests := []struct {
		name    string
		value   string
		success bool
	}{
		{"valid", "my-bucket-123", true},
		{"uppercase", "My-Bucket", false},
		{"spaces", "my bucket", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := solver.Solve(map[string]interface{}{"bucket": tt.value})
			if result.Success != tt.success {
				t.Errorf("pattern check for %q: got success=%v, want %v", tt.value, result.Success, tt.success)
			}
		})
	}
}

// ========== Solve — number constraints ==========

func TestSolve_NumberMinimum(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"port": {Type: "number", Minimum: float64Ptr(1)},
	})

	result := solver.Solve(map[string]interface{}{"port": float64(0)})
	if result.Success {
		t.Fatal("expected failure for number below minimum")
	}
	assertFeedbackField(t, result, "port")
}

func TestSolve_NumberMaximum(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"port": {Type: "number", Maximum: float64Ptr(65535)},
	})

	result := solver.Solve(map[string]interface{}{"port": float64(70000)})
	if result.Success {
		t.Fatal("expected failure for number above maximum")
	}
	assertFeedbackField(t, result, "port")
}

func TestSolve_NumberInRange(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"port": {Type: "number", Minimum: float64Ptr(1), Maximum: float64Ptr(65535)},
	})

	result := solver.Solve(map[string]interface{}{"port": float64(8080)})
	if !result.Success {
		t.Fatalf("expected success for number in range: %v", feedbackMessages(result))
	}
}

// ========== Solve — array constraints ==========

func TestSolve_ArrayMinItems(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"subnets": {Type: "array", MinItems: intPtr(2)},
	})

	result := solver.Solve(map[string]interface{}{
		"subnets": []interface{}{"subnet-1"},
	})
	if result.Success {
		t.Fatal("expected failure for too few array items")
	}
	assertFeedbackField(t, result, "subnets")
}

func TestSolve_ArrayMaxItems(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"cidrs": {Type: "array", MaxItems: intPtr(2)},
	})

	result := solver.Solve(map[string]interface{}{
		"cidrs": []interface{}{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"},
	})
	if result.Success {
		t.Fatal("expected failure for too many array items")
	}
	assertFeedbackField(t, result, "cidrs")
}

// ========== Solve — conflicts ==========

func TestSolve_ConflictDetected(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"vpc_id":              {Type: "string", ConflictsWith: []string{"create_vpc"}},
		"create_vpc":          {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{
		"vpc_id":     "vpc-123",
		"create_vpc": true,
	})
	if result.Success {
		t.Fatal("expected failure for conflicting fields")
	}
}

func TestSolve_NoConflict(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"vpc_id":     {Type: "string", ConflictsWith: []string{"create_vpc"}},
		"create_vpc": {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{
		"vpc_id": "vpc-123",
	})
	if !result.Success {
		t.Fatalf("expected success when no conflict: %v", feedbackMessages(result))
	}
}

// ========== Solve — dependencies ==========

func TestSolve_MissingDependency(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"subnet_id": {Type: "string", DependsOn: []string{"vpc_id"}},
		"vpc_id":    {Type: "string"},
	})

	result := solver.Solve(map[string]interface{}{
		"subnet_id": "subnet-123",
	})
	if result.Success {
		t.Fatal("expected failure for missing dependency")
	}
	assertFeedbackField(t, result, "subnet_id")
}

func TestSolve_DependencySatisfied(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"subnet_id": {Type: "string", DependsOn: []string{"vpc_id"}},
		"vpc_id":    {Type: "string"},
	})

	result := solver.Solve(map[string]interface{}{
		"subnet_id": "subnet-123",
		"vpc_id":    "vpc-123",
	})
	if !result.Success {
		t.Fatalf("expected success with dependency satisfied: %v", feedbackMessages(result))
	}
}

// ========== Solve — required fields ==========

func TestSolve_RequiredFieldMissing(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"name":   {Type: "string", Required: true},
		"region": {Type: "string", Required: true},
	})

	result := solver.Solve(map[string]interface{}{"name": "test"})
	if result.Success {
		t.Fatal("expected failure for missing required field")
	}
	assertFeedbackField(t, result, "region")
}

func TestSolve_RequiredFieldWithDefault(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"region": {Type: "string", Required: true, Default: "us-east-1"},
	})

	// Missing required field with default should NOT cause error
	result := solver.Solve(map[string]interface{}{})
	if !result.Success {
		t.Fatalf("required field with default should not error: %v", feedbackMessages(result))
	}
}

// ========== Solve — implies rules ==========

func TestSolve_ImpliesRule(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"high_availability": {
			Type: "boolean",
			Implies: &ImpliesRule{
				When: true,
				Then: map[string]interface{}{"multi_az": true},
			},
		},
		"multi_az": {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{
		"high_availability": true,
	})
	if !result.Success {
		t.Fatalf("expected success: %v", feedbackMessages(result))
	}
	if result.Params["multi_az"] != true {
		t.Error("expected multi_az to be auto-set to true")
	}
	if len(result.AppliedRules) == 0 {
		t.Error("expected at least one applied rule")
	}
}

func TestSolve_ImpliesRule_NotTriggered(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"high_availability": {
			Type: "boolean",
			Implies: &ImpliesRule{
				When: true,
				Then: map[string]interface{}{"multi_az": true},
			},
		},
		"multi_az": {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{
		"high_availability": false,
	})
	if _, exists := result.Params["multi_az"]; exists {
		t.Error("multi_az should not be set when condition not met")
	}
}

func TestSolve_ImpliesRule_ExistingValueNotOverridden(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"high_availability": {
			Type: "boolean",
			Implies: &ImpliesRule{
				When: true,
				Then: map[string]interface{}{"multi_az": true},
			},
		},
		"multi_az": {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{
		"high_availability": true,
		"multi_az":          false, // explicitly set, should not be overridden
	})
	if result.Params["multi_az"] != false {
		t.Error("implies rule should not override existing value")
	}
}

// ========== Solve — conditional rules ==========

func TestSolve_ConditionalRule_ThenBranch(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"engine": {
			Type: "string",
			Conditional: &ConditionalRule{
				If: &Condition{Field: "engine", Operator: "equals", Value: "aurora"},
				Then: &FieldRequirement{
					Required: []string{"cluster_size"},
				},
			},
		},
		"cluster_size": {Type: "number"},
	})

	result := solver.Solve(map[string]interface{}{"engine": "aurora"})
	if result.Success {
		t.Fatal("expected failure: cluster_size required when engine=aurora")
	}
	assertFeedbackField(t, result, "cluster_size")
}

func TestSolve_ConditionalRule_ElseBranch(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"mode": {
			Type: "string",
			Conditional: &ConditionalRule{
				If:   &Condition{Field: "mode", Operator: "equals", Value: "cluster"},
				Then: &FieldRequirement{Required: []string{"node_count"}},
				Else: &FieldRequirement{Forbidden: []string{"node_count"}},
			},
		},
		"node_count": {Type: "number"},
	})

	// mode=single + node_count present → forbidden
	result := solver.Solve(map[string]interface{}{"mode": "single", "node_count": float64(3)})
	if result.Success {
		t.Fatal("expected failure: node_count forbidden when mode!=cluster")
	}
}

func TestSolve_ConditionalRule_SetValues(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"env": {
			Type: "string",
			Conditional: &ConditionalRule{
				If: &Condition{Field: "env", Operator: "equals", Value: "production"},
				Then: &FieldRequirement{
					SetValues: map[string]interface{}{"backup_enabled": true},
				},
			},
		},
		"backup_enabled": {Type: "boolean"},
	})

	result := solver.Solve(map[string]interface{}{"env": "production"})
	if !result.Success {
		t.Fatalf("expected success: %v", feedbackMessages(result))
	}
	if result.Params["backup_enabled"] != true {
		t.Error("expected backup_enabled to be auto-set to true")
	}
}

// ========== Solve — evaluateCondition ==========

func TestEvaluateCondition_Operators(t *testing.T) {
	solver := newTestSolver(nil)
	params := map[string]interface{}{
		"region": "us-east-1",
		"count":  float64(3),
	}

	tests := []struct {
		name     string
		cond     *Condition
		expected bool
	}{
		{"exists-true", &Condition{Field: "region", Operator: "exists"}, true},
		{"exists-false", &Condition{Field: "missing", Operator: "exists"}, false},
		{"not_exists-true", &Condition{Field: "missing", Operator: "not_exists"}, true},
		{"not_exists-false", &Condition{Field: "region", Operator: "not_exists"}, false},
		{"equals-true", &Condition{Field: "region", Operator: "equals", Value: "us-east-1"}, true},
		{"equals-false", &Condition{Field: "region", Operator: "equals", Value: "eu-west-1"}, false},
		{"not_equals-true", &Condition{Field: "region", Operator: "not_equals", Value: "eu-west-1"}, true},
		{"not_equals-false", &Condition{Field: "region", Operator: "not_equals", Value: "us-east-1"}, false},
		{"in-true", &Condition{Field: "region", Operator: "in", Value: []interface{}{"us-east-1", "us-west-2"}}, true},
		{"in-false", &Condition{Field: "region", Operator: "in", Value: []interface{}{"eu-west-1", "ap-southeast-1"}}, false},
		{"nil-condition", nil, false},
		{"unknown-operator", &Condition{Field: "region", Operator: "like"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := solver.evaluateCondition(tt.cond, params)
			if got != tt.expected {
				t.Errorf("evaluateCondition: got %v, want %v", got, tt.expected)
			}
		})
	}
}

// ========== Solve — map must_include ==========

func TestSolve_MapMustInclude(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"tags": {Type: "map", MustInclude: []string{"Name", "Environment"}},
	})

	result := solver.Solve(map[string]interface{}{
		"tags": map[string]interface{}{"Name": "test"},
	})
	if result.Success {
		t.Fatal("expected failure for missing required map key 'Environment'")
	}
}

func TestSolve_MapMustInclude_AllPresent(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"tags": {Type: "map", MustInclude: []string{"Name", "Environment"}},
	})

	result := solver.Solve(map[string]interface{}{
		"tags": map[string]interface{}{"Name": "test", "Environment": "dev"},
	})
	if !result.Success {
		t.Fatalf("expected success: %v", feedbackMessages(result))
	}
}

// ========== Solve — nil/empty edge cases ==========

func TestSolve_NilParams(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"optional": {Type: "string"},
	})

	result := solver.Solve(nil)
	if !result.Success {
		t.Fatalf("nil params with no required fields should succeed: %v", feedbackMessages(result))
	}
}

func TestSolve_UnknownFields(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"name": {Type: "string"},
	})

	// Extra fields not in schema should pass through (no validation error)
	result := solver.Solve(map[string]interface{}{
		"name":    "test",
		"unknown": "extra",
	})
	if !result.Success {
		t.Fatalf("unknown fields should not cause failure: %v", feedbackMessages(result))
	}
	if result.Params["unknown"] != "extra" {
		t.Error("unknown fields should be preserved in params")
	}
}

func TestSolve_NilDB_NilSchema(t *testing.T) {
	solver := &SchemaSolver{db: nil, schema: nil}
	result := solver.Solve(map[string]interface{}{"key": "value"})
	if result.Success {
		t.Fatal("expected failure when db is nil and schema not loaded")
	}
}

// ========== Solve — deep copy ==========

func TestSolve_DeepCopy(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"tags": {Type: "map"},
	})

	original := map[string]interface{}{
		"tags": map[string]interface{}{"env": "dev"},
	}

	result := solver.Solve(original)

	// Modify result params — should not affect original
	if tagsMap, ok := result.Params["tags"].(map[string]interface{}); ok {
		tagsMap["env"] = "prod"
	}

	origTags := original["tags"].(map[string]interface{})
	if origTags["env"] != "dev" {
		t.Error("Solve should deep-copy params; original was modified")
	}
}

// ========== Solve — combined validation ==========

func TestSolve_MultipleErrors(t *testing.T) {
	solver := newTestSolver(map[string]*SchemaFieldDef{
		"engine": {Type: "string", Required: true, Options: []interface{}{"mysql", "postgres"}},
		"port":   {Type: "number", Minimum: float64Ptr(1024), Maximum: float64Ptr(65535)},
	})

	result := solver.Solve(map[string]interface{}{
		"engine": "sqlite",      // invalid enum
		"port":   float64(80),   // below minimum
	})
	if result.Success {
		t.Fatal("expected failure with multiple errors")
	}
	// Should have at least 2 feedbacks
	errorCount := 0
	for _, fb := range result.Feedbacks {
		if fb.Type == FeedbackTypeError {
			errorCount++
		}
	}
	if errorCount < 2 {
		t.Errorf("expected at least 2 error feedbacks, got %d", errorCount)
	}
	if !result.NeedAIFix {
		t.Error("expected NeedAIFix=true")
	}
	if result.AIInstructions == "" {
		t.Error("expected AIInstructions to be generated")
	}
}

// ========== Helpers ==========

func feedbackMessages(result *SolverResult) string {
	var msgs []string
	for _, fb := range result.Feedbacks {
		msgs = append(msgs, fb.Message)
	}
	return strings.Join(msgs, "; ")
}

func assertFeedbackField(t *testing.T, result *SolverResult, field string) {
	t.Helper()
	for _, fb := range result.Feedbacks {
		if fb.Field == field {
			return
		}
	}
	t.Errorf("expected feedback for field %q, got: %s", field, feedbackMessages(result))
}
