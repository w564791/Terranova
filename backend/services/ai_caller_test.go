package services

import (
	"encoding/json"
	"iac-platform/internal/models"
	"testing"
)

func TestBuildBedrockToolCallingRequest(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "anthropic.claude-3-5-sonnet-20241022-v2:0"}

	messages := []AgentMessage{
		{Role: "system", Content: "You are an assistant"},
		{Role: "user", Content: "Analyze this change"},
	}
	tools := []AgentToolDef{
		{Name: "query_cmdb", Description: "Query CMDB", InputSchema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"resource_id": map[string]interface{}{"type": "string"},
			},
		}},
	}

	body := caller.buildBedrockRequest(messages, tools)

	// Verify anthropic_version
	if body["anthropic_version"] != "bedrock-2023-05-31" {
		t.Errorf("wrong anthropic_version: %v", body["anthropic_version"])
	}

	// Verify system prompt extracted to top level
	if body["system"] != "You are an assistant" {
		t.Errorf("system prompt not extracted: %v", body["system"])
	}

	// Verify messages don't contain system
	msgs := body["messages"].([]interface{})
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message (user only), got %d", len(msgs))
	}
	firstMsg := msgs[0].(map[string]interface{})
	if firstMsg["role"] != "user" {
		t.Errorf("expected user role, got %v", firstMsg["role"])
	}

	// Verify tools
	toolsList := body["tools"].([]interface{})
	if len(toolsList) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(toolsList))
	}
	toolDef := toolsList[0].(map[string]interface{})
	if toolDef["name"] != "query_cmdb" {
		t.Errorf("wrong tool name: %v", toolDef["name"])
	}
}

func TestBuildBedrockRequestWithToolResults(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "test-model"}

	messages := []AgentMessage{
		{Role: "system", Content: "system"},
		{Role: "user", Content: "analyze"},
		{Role: "assistant", ToolCalls: []AgentToolCall{
			{ID: "tc_1", Name: "search", Params: map[string]interface{}{"q": "vpc"}},
		}},
		{Role: "tool", ToolCallID: "tc_1", Content: `{"result": "found"}`},
	}

	body := caller.buildBedrockRequest(messages, nil)
	msgs := body["messages"].([]interface{})

	// user + assistant(tool_use) + user(tool_result) = 3
	if len(msgs) != 3 {
		t.Fatalf("expected 3 messages, got %d", len(msgs))
	}

	// Check assistant message has tool_use content
	assistantMsg := msgs[1].(map[string]interface{})
	content := assistantMsg["content"].([]interface{})
	toolUse := content[0].(map[string]interface{})
	if toolUse["type"] != "tool_use" {
		t.Errorf("expected tool_use type, got %v", toolUse["type"])
	}

	// Check tool_result message
	toolResultMsg := msgs[2].(map[string]interface{})
	if toolResultMsg["role"] != "user" {
		t.Errorf("tool_result should be under user role for Claude, got %v", toolResultMsg["role"])
	}
}

func TestBuildOpenAIToolCallingRequest(t *testing.T) {
	caller := &OpenAICaller{baseURL: "http://localhost:11434/v1", modelID: "gpt-4"}

	messages := []AgentMessage{
		{Role: "system", Content: "You are an assistant"},
		{Role: "user", Content: "Analyze"},
	}
	tools := []AgentToolDef{
		{Name: "search", Description: "Search resources", InputSchema: map[string]interface{}{
			"type":       "object",
			"properties": map[string]interface{}{"query": map[string]interface{}{"type": "string"}},
		}},
	}

	body := caller.buildOpenAIRequest(messages, tools)

	// System message stays in messages array for OpenAI
	msgs := body["messages"].([]interface{})
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}

	// Verify tools use function format
	toolsList := body["tools"].([]interface{})
	toolDef := toolsList[0].(map[string]interface{})
	if toolDef["type"] != "function" {
		t.Errorf("expected function type, got %v", toolDef["type"])
	}
	fn := toolDef["function"].(map[string]interface{})
	if fn["name"] != "search" {
		t.Errorf("wrong function name: %v", fn["name"])
	}
}

func TestParseBedrockToolCallResponse(t *testing.T) {
	caller := &BedrockCaller{}

	responseJSON := `{
		"content": [
			{"type": "text", "text": "I need to query the CMDB"},
			{"type": "tool_use", "id": "toolu_01", "name": "query_cmdb", "input": {"resource_id": "sg-123"}}
		],
		"usage": {"input_tokens": 100, "output_tokens": 50},
		"stop_reason": "tool_use"
	}`

	result, err := caller.parseBedrockResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "I need to query the CMDB" {
		t.Errorf("wrong content: %s", result.Content)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ID != "toolu_01" || tc.Name != "query_cmdb" {
		t.Errorf("wrong tool call: id=%s name=%s", tc.ID, tc.Name)
	}
	if tc.Params["resource_id"] != "sg-123" {
		t.Errorf("wrong params: %v", tc.Params)
	}
}

func TestParseBedrockTextResponse(t *testing.T) {
	caller := &BedrockCaller{}

	responseJSON := `{
		"content": [
			{"type": "text", "text": "Here is the final analysis"}
		],
		"usage": {"input_tokens": 200, "output_tokens": 100},
		"stop_reason": "end_turn"
	}`

	result, err := caller.parseBedrockResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "Here is the final analysis" {
		t.Errorf("wrong content: %s", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(result.ToolCalls))
	}
}

func TestParseOpenAIToolCallResponse(t *testing.T) {
	caller := &OpenAICaller{serviceType: "openai"}

	responseJSON := `{
		"choices": [{
			"message": {
				"content": null,
				"tool_calls": [{
					"id": "call_123",
					"type": "function",
					"function": {
						"name": "query_cmdb",
						"arguments": "{\"resource_id\": \"sg-456\"}"
					}
				}]
			}
		}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 30}
	}`

	result, err := caller.parseOpenAIResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.Name != "query_cmdb" {
		t.Errorf("wrong tool name: %s", tc.Name)
	}
	if tc.Params["resource_id"] != "sg-456" {
		t.Errorf("wrong params: %v", tc.Params)
	}
}

func TestParseOpenAITextResponse(t *testing.T) {
	caller := &OpenAICaller{serviceType: "openai"}

	responseJSON := `{
		"choices": [{
			"message": {
				"content": "Final answer here"
			}
		}],
		"usage": {"prompt_tokens": 50, "completion_tokens": 20}
	}`

	result, err := caller.parseOpenAIResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "Final answer here" {
		t.Errorf("wrong content: %s", result.Content)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(result.ToolCalls))
	}
}

func TestResolveModelID(t *testing.T) {
	tests := []struct {
		region   string
		useIP    bool
		modelID  string
		expected string
	}{
		{"us-east-1", true, "anthropic.claude-3", "us.anthropic.claude-3"},
		{"ap-southeast-1", true, "anthropic.claude-3", "apac.anthropic.claude-3"},
		{"us-east-1", false, "anthropic.claude-3", "anthropic.claude-3"},
	}

	for _, tt := range tests {
		caller := &BedrockCaller{region: tt.region, modelID: tt.modelID, useInferenceProfile: tt.useIP}
		got := caller.resolveModelID()
		if got != tt.expected {
			t.Errorf("resolveModelID(%s, %v) = %s, want %s", tt.region, tt.useIP, got, tt.expected)
		}
	}
}

func TestNewAICallerFromConfig(t *testing.T) {
	bedrockCfg := &models.AIConfig{ServiceType: "bedrock", AWSRegion: "us-west-2", ModelID: "claude-3"}
	caller := NewAICallerFromConfig(bedrockCfg)
	if _, ok := caller.(*BedrockCaller); !ok {
		t.Error("expected BedrockCaller for bedrock service type")
	}

	openaiCfg := &models.AIConfig{ServiceType: "openai", BaseURL: "https://api.openai.com/v1", ModelID: "gpt-4"}
	caller = NewAICallerFromConfig(openaiCfg)
	if _, ok := caller.(*OpenAICaller); !ok {
		t.Error("expected OpenAICaller for openai service type")
	}

	ollamaCfg := &models.AIConfig{ServiceType: "ollama", BaseURL: "http://localhost:11434/v1", ModelID: "llama3"}
	caller = NewAICallerFromConfig(ollamaCfg)
	if _, ok := caller.(*OpenAICaller); !ok {
		t.Error("expected OpenAICaller for ollama service type")
	}
}

// suppress unused import warning
var _ = json.Marshal
