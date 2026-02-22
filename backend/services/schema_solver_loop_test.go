package services

import (
	"strings"
	"testing"
)

// ========== SetMaxRetries Tests ==========

func TestSetMaxRetries_ValidRange(t *testing.T) {
	loop := &AIFeedbackLoop{maxRetries: 3}

	tests := []struct {
		name     string
		input    int
		expected int
	}{
		{"set to 1 (min)", 1, 1},
		{"set to 2", 2, 2},
		{"set to 5 (max)", 5, 5},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop.maxRetries = 3 // reset
			loop.SetMaxRetries(tt.input)
			if loop.maxRetries != tt.expected {
				t.Errorf("SetMaxRetries(%d): got %d, want %d", tt.input, loop.maxRetries, tt.expected)
			}
		})
	}
}

func TestSetMaxRetries_OutOfRange(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int // should remain at initial value
	}{
		{"zero", 0, 3},
		{"negative", -1, 3},
		{"too high", 6, 3},
		{"way too high", 100, 3},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loop := &AIFeedbackLoop{maxRetries: 3}
			loop.SetMaxRetries(tt.input)
			if loop.maxRetries != tt.expected {
				t.Errorf("SetMaxRetries(%d): got %d, want unchanged %d", tt.input, loop.maxRetries, tt.expected)
			}
		})
	}
}

// ========== parseAIResponse Tests ==========

func TestParseAIResponse_StandardFormat(t *testing.T) {
	loop := &AIFeedbackLoop{}
	response := `{
		"corrected_params": {
			"bucket_name": "my-bucket",
			"region": "us-east-1"
		},
		"changes": [
			{
				"field": "region",
				"action": "changed to us-east-1",
				"reason": "closest region"
			}
		]
	}`

	params, reasoning, err := loop.parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["bucket_name"] != "my-bucket" {
		t.Errorf("bucket_name: got %v, want my-bucket", params["bucket_name"])
	}
	if params["region"] != "us-east-1" {
		t.Errorf("region: got %v, want us-east-1", params["region"])
	}
	if reasoning == "" {
		t.Error("expected reasoning to be populated from changes")
	}
}

func TestParseAIResponse_MarkdownCodeBlock(t *testing.T) {
	loop := &AIFeedbackLoop{}
	response := "Here's the fix:\n```json\n" + `{
		"corrected_params": {"name": "test"},
		"changes": []
	}` + "\n```\nDone."

	params, _, err := loop.parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["name"] != "test" {
		t.Errorf("name: got %v, want test", params["name"])
	}
}

func TestParseAIResponse_ConfigFallback(t *testing.T) {
	// When AI returns "config" instead of "corrected_params"
	loop := &AIFeedbackLoop{}
	response := `{
		"config": {"instance_type": "t3.micro"},
		"reasoning": "chose smallest instance"
	}`

	params, reasoning, err := loop.parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["instance_type"] != "t3.micro" {
		t.Errorf("instance_type: got %v, want t3.micro", params["instance_type"])
	}
	if reasoning != "chose smallest instance" {
		t.Errorf("reasoning: got %q, want 'chose smallest instance'", reasoning)
	}
}

func TestParseAIResponse_ArbitraryKeyFallback(t *testing.T) {
	// When AI returns a map under an arbitrary key
	loop := &AIFeedbackLoop{}
	response := `{
		"parameters": {"vpc_id": "vpc-123"},
		"status": "ok"
	}`

	params, _, err := loop.parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if params["vpc_id"] != "vpc-123" {
		t.Errorf("vpc_id: got %v, want vpc-123", params["vpc_id"])
	}
}

func TestParseAIResponse_NoParamsField(t *testing.T) {
	loop := &AIFeedbackLoop{}
	response := `{"status": "ok", "message": "done"}`

	_, _, err := loop.parseAIResponse(response)
	if err == nil {
		t.Fatal("expected error for missing corrected_params")
	}
	if !strings.Contains(err.Error(), "corrected_params") {
		t.Errorf("error should mention corrected_params: %v", err)
	}
}

func TestParseAIResponse_InvalidJSON(t *testing.T) {
	loop := &AIFeedbackLoop{}
	response := "this is not json at all"

	_, _, err := loop.parseAIResponse(response)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseAIResponse_EmptyResponse(t *testing.T) {
	loop := &AIFeedbackLoop{}

	_, _, err := loop.parseAIResponse("")
	if err == nil {
		t.Fatal("expected error for empty response")
	}
}

func TestParseAIResponse_ChangesBuildsReasoning(t *testing.T) {
	loop := &AIFeedbackLoop{}
	response := `{
		"corrected_params": {"name": "test"},
		"changes": [
			{"field": "name", "action": "set", "reason": "default"},
			{"field": "region", "action": "added", "reason": "required"}
		]
	}`

	_, reasoning, err := loop.parseAIResponse(response)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(reasoning, "name") {
		t.Error("reasoning should mention 'name' field")
	}
	if !strings.Contains(reasoning, "region") {
		t.Error("reasoning should mention 'region' field")
	}
}

// ========== formatValidationResults Tests ==========

func TestFormatValidationResults_AllSections(t *testing.T) {
	loop := &AIFeedbackLoop{}
	result := &SolverResult{
		AppliedRules: []string{"rule1", "rule2"},
		Warnings:     []string{"warn1"},
		Feedbacks: []*SolverFeedback{
			{Type: FeedbackTypeError, Field: "f1"},
			{Type: FeedbackTypeError, Field: "f2"},
			{Type: FeedbackTypeWarning, Field: "f3"},
		},
	}

	output := loop.formatValidationResults(result)

	if !strings.Contains(output, "rule1") {
		t.Error("should contain applied rule")
	}
	if !strings.Contains(output, "warn1") {
		t.Error("should contain warning")
	}
	if !strings.Contains(output, "2") {
		t.Error("should show error count of 2")
	}
}

func TestFormatValidationResults_Empty(t *testing.T) {
	loop := &AIFeedbackLoop{}
	result := &SolverResult{
		AppliedRules: []string{},
		Warnings:     []string{},
		Feedbacks:    []*SolverFeedback{},
	}

	output := loop.formatValidationResults(result)
	if output != "" {
		t.Errorf("expected empty output for empty result, got %q", output)
	}
}

func TestFormatValidationResults_OnlyWarnings(t *testing.T) {
	loop := &AIFeedbackLoop{}
	result := &SolverResult{
		Warnings:  []string{"watch out"},
		Feedbacks: []*SolverFeedback{},
	}

	output := loop.formatValidationResults(result)
	if !strings.Contains(output, "watch out") {
		t.Error("should contain warning message")
	}
	if strings.Contains(output, "错误") {
		t.Error("should not contain error section")
	}
}

// ========== buildAIPrompt Tests ==========

func TestBuildAIPrompt_ContainsAllSections(t *testing.T) {
	loop := &AIFeedbackLoop{}
	result := &SolverResult{
		AIInstructions: "fix the region field",
		AppliedRules:   []string{"type_check"},
		Feedbacks: []*SolverFeedback{
			{Type: FeedbackTypeError, Field: "region"},
		},
	}

	prompt := loop.buildAIPrompt(
		"create an S3 bucket",
		map[string]interface{}{"bucket_name": "test"},
		result,
		1,
	)

	if !strings.Contains(prompt, "create an S3 bucket") {
		t.Error("prompt should contain user request")
	}
	if !strings.Contains(prompt, "bucket_name") {
		t.Error("prompt should contain current params")
	}
	if !strings.Contains(prompt, "fix the region field") {
		t.Error("prompt should contain AI instructions")
	}
	if !strings.Contains(prompt, "1") {
		t.Error("prompt should contain iteration number")
	}
	if !strings.Contains(prompt, "corrected_params") {
		t.Error("prompt should contain output format instruction")
	}
}

// ========== extractJSON Tests ==========

func TestExtractJSON_Plain(t *testing.T) {
	input := `{"key": "value"}`
	result := extractJSON(input)
	if result != `{"key": "value"}` {
		t.Errorf("got %q", result)
	}
}

func TestExtractJSON_MarkdownCodeBlock(t *testing.T) {
	input := "Some text\n```json\n{\"key\": \"value\"}\n```\nMore text"
	result := extractJSON(input)
	if !strings.Contains(result, `"key"`) {
		t.Errorf("should extract JSON from code block, got %q", result)
	}
}

func TestExtractJSON_GenericCodeBlock(t *testing.T) {
	input := "Text\n```\n{\"k\": 1}\n```\nEnd"
	result := extractJSON(input)
	if !strings.Contains(result, `"k"`) {
		t.Errorf("should extract JSON from generic code block, got %q", result)
	}
}

func TestExtractJSON_EmptyString(t *testing.T) {
	result := extractJSON("")
	if result != "" {
		t.Errorf("expected empty, got %q", result)
	}
}

// ========== fixIncompleteJSON Tests ==========

func TestFixIncompleteJSON_MissingBraces(t *testing.T) {
	input := `{"key": {"nested": "value"`
	result := fixIncompleteJSON(input)
	if !strings.HasSuffix(result, "}}") {
		t.Errorf("should add missing braces, got %q", result)
	}
}

func TestFixIncompleteJSON_MissingBrackets(t *testing.T) {
	input := `[1, 2, [3, 4`
	result := fixIncompleteJSON(input)
	if !strings.HasSuffix(result, "]]") {
		t.Errorf("should add missing brackets, got %q", result)
	}
}

func TestFixIncompleteJSON_MixedMissing(t *testing.T) {
	input := `{"items": [1, 2`
	result := fixIncompleteJSON(input)
	// should close bracket first, then brace
	if !strings.HasSuffix(result, "]}") {
		t.Errorf("should close brackets then braces, got %q", result)
	}
}

func TestFixIncompleteJSON_AlreadyComplete(t *testing.T) {
	input := `{"key": "value"}`
	result := fixIncompleteJSON(input)
	if result != input {
		t.Errorf("should not modify complete JSON, got %q", result)
	}
}
