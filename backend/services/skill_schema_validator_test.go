package services

import (
	"encoding/json"
	"testing"

	"iac-platform/internal/models"
)

// TestSchemaValidator_ValidJSON tests that valid output passes validation
func TestSchemaValidator_ValidJSON(t *testing.T) {
	validator := NewSkillSchemaValidator()

	// Register schema
	schema := SkillOutputSchema{
		RequiredFields: []string{"status", "result"},
		EnumFields: map[string][]string{
			"status": {"success", "failure", "pending"},
		},
	}
	validator.RegisterSchema("test_skill", schema)

	// Valid output
	output := json.RawMessage(`{
		"status": "success",
		"result": "Operation completed",
		"extra_field": "ignored"
	}`)

	result := validator.Validate("test_skill", output)

	if !result.Valid {
		t.Errorf("Expected Valid=true, got Valid=%v", result.Valid)
	}
	if result.Score != 100 {
		t.Errorf("Expected Score=100, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictPass) {
		t.Errorf("Expected Verdict=%s, got Verdict=%s", models.AssessmentVerdictPass, result.Verdict)
	}
	if len(result.MissingFields) != 0 {
		t.Errorf("Expected no missing fields, got %v", result.MissingFields)
	}
	if len(result.InvalidEnums) != 0 {
		t.Errorf("Expected no invalid enums, got %v", result.InvalidEnums)
	}
}

// TestSchemaValidator_MissingRequiredFields tests that missing required fields fail validation
func TestSchemaValidator_MissingRequiredFields(t *testing.T) {
	validator := NewSkillSchemaValidator()

	schema := SkillOutputSchema{
		RequiredFields: []string{"status", "result", "timestamp"},
		EnumFields:     map[string][]string{},
	}
	validator.RegisterSchema("test_skill", schema)

	// Missing "result" and "timestamp" fields
	output := json.RawMessage(`{
		"status": "success"
	}`)

	result := validator.Validate("test_skill", output)

	if result.Valid {
		t.Errorf("Expected Valid=false, got Valid=%v", result.Valid)
	}
	if result.Score != 0 {
		t.Errorf("Expected Score=0, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictFail) {
		t.Errorf("Expected Verdict=%s, got Verdict=%s", models.AssessmentVerdictFail, result.Verdict)
	}
	if len(result.MissingFields) != 2 {
		t.Errorf("Expected 2 missing fields, got %d: %v", len(result.MissingFields), result.MissingFields)
	}
	// Check that both "result" and "timestamp" are in missing fields
	missingMap := make(map[string]bool)
	for _, field := range result.MissingFields {
		missingMap[field] = true
	}
	if !missingMap["result"] || !missingMap["timestamp"] {
		t.Errorf("Expected missing fields to contain 'result' and 'timestamp', got %v", result.MissingFields)
	}
}

// TestSchemaValidator_InvalidEnum tests that invalid enum values fail validation
func TestSchemaValidator_InvalidEnum(t *testing.T) {
	validator := NewSkillSchemaValidator()

	schema := SkillOutputSchema{
		RequiredFields: []string{"status"},
		EnumFields: map[string][]string{
			"status": {"success", "failure", "pending"},
			"level":  {"info", "warning", "error"},
		},
	}
	validator.RegisterSchema("test_skill", schema)

	// Invalid enum value for "status"
	output := json.RawMessage(`{
		"status": "completed",
		"level": "critical"
	}`)

	result := validator.Validate("test_skill", output)

	if result.Valid {
		t.Errorf("Expected Valid=false, got Valid=%v", result.Valid)
	}
	if result.Score != 0 {
		t.Errorf("Expected Score=0, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictFail) {
		t.Errorf("Expected Verdict=%s, got Verdict=%s", models.AssessmentVerdictFail, result.Verdict)
	}
	if len(result.InvalidEnums) != 2 {
		t.Errorf("Expected 2 invalid enums, got %d: %v", len(result.InvalidEnums), result.InvalidEnums)
	}
	// Check format: "fieldname=actualvalue"
	invalidMap := make(map[string]bool)
	for _, invalid := range result.InvalidEnums {
		invalidMap[invalid] = true
	}
	if !invalidMap["status=completed"] || !invalidMap["level=critical"] {
		t.Errorf("Expected invalid enums to contain 'status=completed' and 'level=critical', got %v", result.InvalidEnums)
	}
}

// TestSchemaValidator_InvalidJSON tests that malformed JSON fails with verdict "fail"
func TestSchemaValidator_InvalidJSON(t *testing.T) {
	validator := NewSkillSchemaValidator()

	schema := SkillOutputSchema{
		RequiredFields: []string{"status"},
		EnumFields:     map[string][]string{},
	}
	validator.RegisterSchema("test_skill", schema)

	// Malformed JSON
	output := json.RawMessage(`{invalid json}`)

	result := validator.Validate("test_skill", output)

	if result.Valid {
		t.Errorf("Expected Valid=false, got Valid=%v", result.Valid)
	}
	if result.Score != 0 {
		t.Errorf("Expected Score=0, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictFail) {
		t.Errorf("Expected Verdict=%s, got Verdict=%s", models.AssessmentVerdictFail, result.Verdict)
	}
}

// TestSchemaValidator_UnregisteredSkill tests that unregistered skills pass by default
func TestSchemaValidator_UnregisteredSkill(t *testing.T) {
	validator := NewSkillSchemaValidator()

	// Do not register any schema

	// Any JSON should pass
	output := json.RawMessage(`{"anything": "goes"}`)

	result := validator.Validate("unregistered_skill", output)

	if !result.Valid {
		t.Errorf("Expected Valid=true for unregistered skill, got Valid=%v", result.Valid)
	}
	if result.Score != 100 {
		t.Errorf("Expected Score=100 for unregistered skill, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictPass) {
		t.Errorf("Expected Verdict=%s for unregistered skill, got Verdict=%s", models.AssessmentVerdictPass, result.Verdict)
	}
	if len(result.MissingFields) != 0 {
		t.Errorf("Expected no missing fields for unregistered skill, got %v", result.MissingFields)
	}
	if len(result.InvalidEnums) != 0 {
		t.Errorf("Expected no invalid enums for unregistered skill, got %v", result.InvalidEnums)
	}
}

// TestSchemaValidator_NullFieldValue tests that null values for required fields fail
func TestSchemaValidator_NullFieldValue(t *testing.T) {
	validator := NewSkillSchemaValidator()

	schema := SkillOutputSchema{
		RequiredFields: []string{"status", "result"},
		EnumFields:     map[string][]string{},
	}
	validator.RegisterSchema("test_skill", schema)

	// "result" field is null
	output := json.RawMessage(`{
		"status": "success",
		"result": null
	}`)

	result := validator.Validate("test_skill", output)

	if result.Valid {
		t.Errorf("Expected Valid=false for null required field, got Valid=%v", result.Valid)
	}
	if result.Score != 0 {
		t.Errorf("Expected Score=0, got Score=%d", result.Score)
	}
	if result.Verdict != string(models.AssessmentVerdictFail) {
		t.Errorf("Expected Verdict=%s, got Verdict=%s", models.AssessmentVerdictFail, result.Verdict)
	}
	if len(result.MissingFields) != 1 || result.MissingFields[0] != "result" {
		t.Errorf("Expected missing field 'result', got %v", result.MissingFields)
	}
}

// TestSchemaValidator_ConcurrentAccess tests thread-safe access
func TestSchemaValidator_ConcurrentAccess(t *testing.T) {
	validator := NewSkillSchemaValidator()

	schema := SkillOutputSchema{
		RequiredFields: []string{"status"},
		EnumFields:     map[string][]string{},
	}

	// Register schemas concurrently
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(index int) {
			validator.RegisterSchema("test_skill", schema)
			output := json.RawMessage(`{"status": "success"}`)
			_ = validator.Validate("test_skill", output)
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 10; i++ {
		<-done
	}
}
