package services

import (
	"encoding/json"
	"iac-platform/internal/models"
	"testing"
	"time"
)

func TestBuildBedrockToolCallingRequest(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "anthropic.claude-3-5-sonnet-20241022-v2:0", cacheEnabled: true}

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

	// Verify system prompt extracted to top level (array with cache_control)
	systemBlocks := body["system"].([]interface{})
	if len(systemBlocks) != 1 {
		t.Fatalf("expected 1 system block, got %d", len(systemBlocks))
	}
	systemBlock := systemBlocks[0].(map[string]interface{})
	if systemBlock["text"] != "You are an assistant" {
		t.Errorf("system text mismatch: %v", systemBlock["text"])
	}
	if _, ok := systemBlock["cache_control"]; !ok {
		t.Error("expected cache_control on system block when cacheEnabled=true")
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

func TestBuildBedrockRequestWithCacheEnabled(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "test-model", cacheEnabled: true}

	messages := []AgentMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Hello"},
	}

	body := caller.buildBedrockRequest(messages, nil)
	systemBlocks := body["system"].([]interface{})
	block := systemBlocks[0].(map[string]interface{})

	if _, ok := block["cache_control"]; !ok {
		t.Error("expected cache_control when cacheEnabled=true")
	}
	cc := block["cache_control"].(map[string]interface{})
	if cc["type"] != "ephemeral" {
		t.Errorf("cache_control type should be ephemeral, got %v", cc["type"])
	}
}

func TestBuildBedrockRequestWithCacheDisabled(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "test-model", cacheEnabled: false}

	messages := []AgentMessage{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Hello"},
	}

	body := caller.buildBedrockRequest(messages, nil)
	systemBlocks := body["system"].([]interface{})
	block := systemBlocks[0].(map[string]interface{})

	if _, ok := block["cache_control"]; ok {
		t.Error("cache_control should NOT be present when cacheEnabled=false")
	}
	if block["text"] != "System prompt" {
		t.Errorf("system text should still be present, got %v", block["text"])
	}
}

func TestBuildBedrockRequestNoSystemPrompt(t *testing.T) {
	caller := &BedrockCaller{region: "us-west-2", modelID: "test-model", cacheEnabled: true}

	messages := []AgentMessage{
		{Role: "user", Content: "Hello"},
	}

	body := caller.buildBedrockRequest(messages, nil)
	if _, ok := body["system"]; ok {
		t.Error("system block should not exist when there's no system message")
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
		{"us-east-1", true, "anthropic.claude-3", "anthropic.claude-3"},
		{"ap-southeast-1", true, "anthropic.claude-3", "anthropic.claude-3"},
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

func TestBuildBedrockRequestWithThinking(t *testing.T) {
	caller := &BedrockCaller{
		region:               "us-west-2",
		modelID:              "anthropic.claude-opus-4-6-v1",
		thinkingEnabled:      true,
		thinkingBudgetTokens: 5000,
	}

	messages := []AgentMessage{
		{Role: "system", Content: "You are an assistant"},
		{Role: "user", Content: "Analyze this"},
	}

	body := caller.buildBedrockRequest(messages, nil)

	thinking, ok := body["thinking"].(map[string]interface{})
	if !ok {
		t.Fatal("thinking block not found in request")
	}
	if thinking["type"] != "enabled" {
		t.Errorf("thinking type should be 'enabled', got %v", thinking["type"])
	}
	if thinking["budget_tokens"] != 5000 {
		t.Errorf("budget_tokens should be 5000, got %v", thinking["budget_tokens"])
	}

	// temperature must NOT be set when thinking is enabled
	if _, hasTemp := body["temperature"]; hasTemp {
		t.Error("temperature must not be set when thinking is enabled")
	}
}

func TestBuildBedrockRequestWithoutThinking(t *testing.T) {
	caller := &BedrockCaller{
		region:          "us-west-2",
		modelID:         "anthropic.claude-sonnet-4-6",
		thinkingEnabled: false,
	}

	messages := []AgentMessage{
		{Role: "user", Content: "Hello"},
	}

	body := caller.buildBedrockRequest(messages, nil)

	if _, ok := body["thinking"]; ok {
		t.Error("thinking block should not exist when thinking is disabled")
	}
}

func TestBuildBedrockRequestThinkingBudgetTooSmall(t *testing.T) {
	caller := &BedrockCaller{
		region:               "us-west-2",
		modelID:              "anthropic.claude-opus-4-6-v1",
		thinkingEnabled:      true,
		thinkingBudgetTokens: 500, // below 1024 minimum
	}

	body := caller.buildBedrockRequest([]AgentMessage{{Role: "user", Content: "test"}}, nil)

	if _, ok := body["thinking"]; ok {
		t.Error("thinking block should not exist when budget < 1024")
	}
}

func TestParseBedrockThinkingResponse(t *testing.T) {
	caller := &BedrockCaller{}

	responseJSON := `{
		"content": [
			{"type": "thinking", "thinking": "Let me analyze this step by step..."},
			{"type": "text", "text": "Here is my analysis"}
		],
		"usage": {"input_tokens": 100, "output_tokens": 200},
		"stop_reason": "end_turn"
	}`

	result, err := caller.parseBedrockResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.Content != "Here is my analysis" {
		t.Errorf("wrong content: %s", result.Content)
	}
	if result.ThinkingContent != "Let me analyze this step by step..." {
		t.Errorf("wrong thinking content: %s", result.ThinkingContent)
	}
}

func TestParseBedrockResponseWithoutThinking(t *testing.T) {
	caller := &BedrockCaller{}

	responseJSON := `{
		"content": [
			{"type": "text", "text": "Normal response"}
		],
		"usage": {"input_tokens": 50, "output_tokens": 30},
		"stop_reason": "end_turn"
	}`

	result, err := caller.parseBedrockResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.ThinkingContent != "" {
		t.Errorf("thinking content should be empty, got: %s", result.ThinkingContent)
	}
}

func TestNewAICallerFromConfig_ThinkingPassthrough(t *testing.T) {
	cfg := &models.AIConfig{
		ServiceType:          "bedrock",
		AWSRegion:            "us-west-2",
		ModelID:              "anthropic.claude-opus-4-6-v1",
		ThinkingEnabled:      true,
		ThinkingBudgetTokens: 8000,
		CacheEnabled:         true,
	}
	caller := NewAICallerFromConfig(cfg)
	bc, ok := caller.(*BedrockCaller)
	if !ok {
		t.Fatal("expected BedrockCaller")
	}
	if !bc.thinkingEnabled {
		t.Error("thinkingEnabled should be true")
	}
	if bc.thinkingBudgetTokens != 8000 {
		t.Errorf("thinkingBudgetTokens should be 8000, got %d", bc.thinkingBudgetTokens)
	}
	if !bc.cacheEnabled {
		t.Error("cacheEnabled should be true")
	}
}

func TestNewAICallerFromConfig_CacheDisabled(t *testing.T) {
	cfg := &models.AIConfig{
		ServiceType:  "bedrock",
		AWSRegion:    "us-west-2",
		ModelID:      "anthropic.claude-3-5-sonnet-20241022-v2:0",
		CacheEnabled: false,
	}
	caller := NewAICallerFromConfig(cfg)
	bc, ok := caller.(*BedrockCaller)
	if !ok {
		t.Fatal("expected BedrockCaller")
	}
	if bc.cacheEnabled {
		t.Error("cacheEnabled should be false")
	}
}

// ========== Qwen Caller Tests ==========

func TestBuildQwenRequestWithThinking(t *testing.T) {
	caller := &QwenCaller{
		OpenAICaller: OpenAICaller{modelID: "qwen3.5-plus"},
		thinkingEnabled:      true,
		thinkingBudgetTokens: 10240,
	}

	body := caller.buildQwenRequest([]AgentMessage{{Role: "user", Content: "test"}}, nil)

	if body["enable_thinking"] != true {
		t.Error("enable_thinking should be true")
	}
	if _, hasTemp := body["temperature"]; hasTemp {
		t.Error("temperature must not be set when thinking is enabled")
	}
}

func TestBuildQwenRequestWithoutThinking(t *testing.T) {
	caller := &QwenCaller{
		OpenAICaller: OpenAICaller{modelID: "qwen3.5-plus"},
		thinkingEnabled: false,
	}

	body := caller.buildQwenRequest([]AgentMessage{{Role: "user", Content: "test"}}, nil)

	if _, ok := body["enable_thinking"]; ok {
		t.Error("enable_thinking should not exist when thinking is disabled")
	}
	if body["temperature"] != 0.7 {
		t.Errorf("temperature should be 0.7, got %v", body["temperature"])
	}
	// 非 thinking 模式应自动加 json_object
	rf, ok := body["response_format"].(map[string]interface{})
	if !ok {
		t.Fatal("response_format should be set when thinking is disabled")
	}
	if rf["type"] != "json_object" {
		t.Errorf("response_format type should be json_object, got %v", rf["type"])
	}
}

func TestBuildQwenRequestWithThinkingNoJsonMode(t *testing.T) {
	caller := &QwenCaller{
		OpenAICaller:         OpenAICaller{modelID: "qwen3.5-plus"},
		thinkingEnabled:      true,
		thinkingBudgetTokens: 10240,
	}

	body := caller.buildQwenRequest([]AgentMessage{{Role: "user", Content: "test"}}, nil)

	// thinking 模式不能加 response_format
	if _, ok := body["response_format"]; ok {
		t.Error("response_format should not be set when thinking is enabled")
	}
}

func TestParseQwenThinkingResponse(t *testing.T) {
	caller := &QwenCaller{}

	responseJSON := `{
		"choices": [{
			"message": {
				"reasoning_content": "Let me think step by step...",
				"content": "Here is my answer"
			}
		}],
		"usage": {"prompt_tokens": 100, "completion_tokens": 200}
	}`

	result, err := caller.parseQwenResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Content != "Here is my answer" {
		t.Errorf("wrong content: %s", result.Content)
	}
	if result.ThinkingContent != "Let me think step by step..." {
		t.Errorf("wrong thinking content: %s", result.ThinkingContent)
	}
}

func TestParseQwenResponseWithoutThinking(t *testing.T) {
	caller := &QwenCaller{}

	responseJSON := `{
		"choices": [{
			"message": {
				"content": "Normal response"
			}
		}],
		"usage": {"prompt_tokens": 50, "completion_tokens": 30}
	}`

	result, err := caller.parseQwenResponse([]byte(responseJSON))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ThinkingContent != "" {
		t.Errorf("thinking content should be empty, got: %s", result.ThinkingContent)
	}
}

func TestNewAICallerFromConfig_QwenType(t *testing.T) {
	cfg := &models.AIConfig{
		ServiceType:          "qwen",
		BaseURL:              "https://dashscope-intl.aliyuncs.com/compatible-mode/v1",
		ModelID:              "qwen3.5-plus",
		ThinkingEnabled:      true,
		ThinkingBudgetTokens: 10240,
	}
	caller := NewAICallerFromConfig(cfg)
	qc, ok := caller.(*QwenCaller)
	if !ok {
		t.Fatal("expected QwenCaller for qwen service type")
	}
	if !qc.thinkingEnabled {
		t.Error("thinkingEnabled should be true")
	}
	if qc.thinkingBudgetTokens != 10240 {
		t.Errorf("thinkingBudgetTokens should be 10240, got %d", qc.thinkingBudgetTokens)
	}
}

// suppress unused import warning
var _ = json.Marshal

func TestNormalizeGrokReasoningEffort(t *testing.T) {
	cases := map[string]string{
		"":       GrokEffortHigh,
		"LOW":    GrokEffortLow,
		"medium": GrokEffortMedium,
		"high":   GrokEffortHigh,
		"xhigh":  GrokEffortHigh, // 不支持 xhigh，回落 high
		"none":   GrokEffortHigh,
	}
	for in, want := range cases {
		if got := NormalizeGrokReasoningEffort(in); got != want {
			t.Errorf("NormalizeGrokReasoningEffort(%q)=%q want %q", in, got, want)
		}
	}
}

func TestNewAICallerFromConfig_GrokType(t *testing.T) {
	cfg := &models.AIConfig{
		ServiceType:         "grok",
		BaseURL:             "",
		APIKey:              "xai-test",
		ModelID:             "grok-4.5",
		GrokReasoningEffort: "medium",
	}
	caller := NewAICallerFromConfig(cfg)
	gc, ok := caller.(*GrokCaller)
	if !ok {
		t.Fatalf("expected *GrokCaller, got %T", caller)
	}
	if gc.baseURL != DefaultGrokBaseURL {
		t.Errorf("baseURL=%q want default %q", gc.baseURL, DefaultGrokBaseURL)
	}
	if gc.reasoningEffort != GrokEffortMedium {
		t.Errorf("effort=%q want medium", gc.reasoningEffort)
	}
	if gc.modelID != "grok-4.5" {
		t.Errorf("model=%q", gc.modelID)
	}
}

func TestGrokCaller_buildGrokRequest(t *testing.T) {
	c := &GrokCaller{
		OpenAICaller: OpenAICaller{
			baseURL:     DefaultGrokBaseURL,
			apiKey:      "k",
			modelID:     "grok-4.5",
			serviceType: "grok",
		},
		reasoningEffort: GrokEffortLow,
	}
	body := c.buildGrokRequest([]AgentMessage{{Role: "user", Content: "hi"}}, nil)
	if body["reasoning_effort"] != GrokEffortLow {
		t.Errorf("reasoning_effort=%v", body["reasoning_effort"])
	}
	if _, hasTemp := body["temperature"]; hasTemp {
		t.Error("temperature should be removed for reasoning models")
	}
	if body["model"] != "grok-4.5" {
		t.Errorf("model=%v", body["model"])
	}
}

func TestGrokTimeoutForEffort(t *testing.T) {
	if GrokTimeoutForEffort(GrokEffortLow) != 120*time.Second {
		t.Fatal("low timeout")
	}
	if GrokTimeoutForEffort(GrokEffortMedium) != 300*time.Second {
		t.Fatal("medium timeout")
	}
	if GrokTimeoutForEffort(GrokEffortHigh) != 600*time.Second {
		t.Fatal("high timeout")
	}
}

func TestResolveGrokBaseURL(t *testing.T) {
	if ResolveGrokBaseURL("") != DefaultGrokBaseURL {
		t.Fatal("empty should default")
	}
	if ResolveGrokBaseURL("https://custom.example/v1") != "https://custom.example/v1" {
		t.Fatal("custom should keep")
	}
}

func TestApplyGrokReasoningToBody(t *testing.T) {
	body := map[string]interface{}{"temperature": 0.7, "model": "grok-4.5"}
	applyGrokReasoningToBody(body, "medium")
	if body["reasoning_effort"] != GrokEffortMedium {
		t.Fatalf("effort=%v", body["reasoning_effort"])
	}
	if _, ok := body["temperature"]; ok {
		t.Fatal("temperature should be deleted")
	}
}
