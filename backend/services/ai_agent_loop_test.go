package services

import (
	"context"
	"testing"
)

// ========== Mock implementations ==========

type mockAICaller struct {
	calls     int
	responses []*AgentAIResponse
}

func (m *mockAICaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	idx := m.calls
	m.calls++
	if idx >= len(m.responses) {
		return &AgentAIResponse{Content: "fallback response"}, nil
	}
	return m.responses[idx], nil
}

type mockTool struct {
	name       string
	callCount  int
	lastParams map[string]interface{}
}

func (t *mockTool) Name() string        { return t.name }
func (t *mockTool) Description() string  { return "mock tool for testing" }
func (t *mockTool) InputSchema() map[string]interface{} {
	return map[string]interface{}{"query": map[string]interface{}{"type": "string"}}
}
func (t *mockTool) Execute(ctx context.Context, params map[string]interface{}) (interface{}, error) {
	t.callCount++
	t.lastParams = params
	return map[string]string{"result": "mock_data"}, nil
}

// ========== Tests ==========

func TestAgentLoopFinishesOnTextResponse(t *testing.T) {
	caller := &mockAICaller{
		responses: []*AgentAIResponse{
			{Content: "This is the final answer"},
		},
	}

	loop := NewAIAgentLoop(caller, 10)
	result, err := loop.Run(context.Background(), "system prompt", "user prompt")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if result.TotalSteps != 1 {
		t.Errorf("expected TotalSteps=1, got %d", result.TotalSteps)
	}
	if result.FinalOutput != "This is the final answer" {
		t.Errorf("unexpected FinalOutput: %s", result.FinalOutput)
	}
	if len(result.ToolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(result.ToolCalls))
	}
}

func TestAgentLoopExecutesToolCalls(t *testing.T) {
	caller := &mockAICaller{
		responses: []*AgentAIResponse{
			{
				ToolCalls: []AgentToolCall{
					{ID: "tc_1", Name: "search", Params: map[string]interface{}{"query": "vpc"}},
				},
			},
			{Content: "Found VPC info, here is the summary"},
		},
	}

	tool := &mockTool{name: "search"}
	loop := NewAIAgentLoop(caller, 10)
	loop.RegisterTool(tool)

	result, err := loop.Run(context.Background(), "system", "user")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Completed {
		t.Error("expected Completed=true")
	}
	if result.TotalSteps != 2 {
		t.Errorf("expected TotalSteps=2, got %d", result.TotalSteps)
	}
	if tool.callCount != 1 {
		t.Errorf("expected tool called once, got %d", tool.callCount)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ToolName != "search" {
		t.Errorf("expected tool name 'search', got '%s'", result.ToolCalls[0].ToolName)
	}
	if result.ToolCalls[0].Error != "" {
		t.Errorf("expected no error, got '%s'", result.ToolCalls[0].Error)
	}
}

func TestAgentLoopRespectsMaxIterations(t *testing.T) {
	// First 3 calls return tool calls (hitting maxIterations=3), 4th call is the forced final
	caller := &mockAICaller{
		responses: []*AgentAIResponse{
			{ToolCalls: []AgentToolCall{{ID: "tc_0", Name: "search", Params: map[string]interface{}{"query": "loop"}}}},
			{ToolCalls: []AgentToolCall{{ID: "tc_1", Name: "search", Params: map[string]interface{}{"query": "loop"}}}},
			{ToolCalls: []AgentToolCall{{ID: "tc_2", Name: "search", Params: map[string]interface{}{"query": "loop"}}}},
			{Content: "forced summary"}, // forced final call after max iterations
		},
	}

	tool := &mockTool{name: "search"}
	loop := NewAIAgentLoop(caller, 3)
	loop.RegisterTool(tool)

	result, err := loop.Run(context.Background(), "system", "user")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Completed {
		t.Error("expected Completed=false (hit max iterations)")
	}
	if result.TotalSteps != 3 {
		t.Errorf("expected TotalSteps=3, got %d", result.TotalSteps)
	}
	if tool.callCount != 3 {
		t.Errorf("expected tool called 3 times, got %d", tool.callCount)
	}
	if result.FinalOutput != "forced summary" {
		t.Errorf("expected forced summary output, got '%s'", result.FinalOutput)
	}
}

func TestAgentLoopHandlesUnknownTool(t *testing.T) {
	caller := &mockAICaller{
		responses: []*AgentAIResponse{
			{
				ToolCalls: []AgentToolCall{
					{ID: "tc_1", Name: "nonexistent_tool", Params: nil},
				},
			},
			{Content: "final answer after error"},
		},
	}

	loop := NewAIAgentLoop(caller, 10)
	// Don't register any tools

	result, err := loop.Run(context.Background(), "system", "user")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call record, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].Error == "" {
		t.Error("expected error for unknown tool")
	}
}

func TestAgentLoopDefaultMaxIterations(t *testing.T) {
	loop := NewAIAgentLoop(&mockAICaller{}, 0)
	if loop.maxIterations != 10 {
		t.Errorf("expected default maxIterations=10, got %d", loop.maxIterations)
	}

	loop2 := NewAIAgentLoop(&mockAICaller{}, -5)
	if loop2.maxIterations != 10 {
		t.Errorf("expected default maxIterations=10, got %d", loop2.maxIterations)
	}
}
