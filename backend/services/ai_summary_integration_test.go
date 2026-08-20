package services

import (
	"context"
	"encoding/json"
	"iac-platform/internal/models"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sequentialMockAICaller returns predefined responses in sequence
type sequentialMockAICaller struct {
	callIndex int
	responses []*AgentAIResponse
}

func (m *sequentialMockAICaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	idx := m.callIndex
	m.callIndex++
	if idx >= len(m.responses) {
		return &AgentAIResponse{Content: "no more responses"}, nil
	}
	return m.responses[idx], nil
}

func setupIntegrationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared&_busy_timeout=5000"), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}

	db.AutoMigrate(
		&models.ResourceIndex{},
		&models.AIPlanSummary{},
		&models.AIApplySummary{},
	)
	return db
}

func TestAIAgentLoopWithRealTools(t *testing.T) {
	db := setupIntegrationTestDB(t)

	// Seed resource_index with module resources
	resources := []models.ResourceIndex{
		{WorkspaceID: "ws-test", TerraformAddress: "module.vpc.aws_vpc.main", ResourceType: "aws_vpc", ResourceName: "main", ResourceMode: "managed", ModulePath: "module.vpc", CloudResourceID: "vpc-123"},
		{WorkspaceID: "ws-test", TerraformAddress: "module.vpc.aws_subnet.public", ResourceType: "aws_subnet", ResourceName: "public", ResourceMode: "managed", ModulePath: "module.vpc", CloudResourceID: "subnet-456"},
		{WorkspaceID: "ws-test", TerraformAddress: "module.vpc.aws_security_group.main", ResourceType: "aws_security_group", ResourceName: "main", ResourceMode: "managed", ModulePath: "module.vpc", CloudResourceID: "sg-789"},
		{WorkspaceID: "ws-foreign", TerraformAddress: "module.vpc.aws_vpc.foreign", ResourceType: "aws_vpc", ResourceName: "foreign", ResourceMode: "managed", ModulePath: "module.vpc", CloudResourceID: "vpc-foreign"},
	}
	for _, r := range resources {
		db.Create(&r)
	}

	// Mock AI: 1st call queries module resources, 2nd call returns final answer
	mockCaller := &sequentialMockAICaller{
		responses: []*AgentAIResponse{
			{
				Content: "Let me check the module resources",
				ToolCalls: []AgentToolCall{
					{ID: "tc_1", Name: "query_module_resources", Params: map[string]interface{}{
						// The model may try to inject another workspace, but the
						// registered task scope must ignore it.
						"workspace_id": "ws-foreign",
						"module_path":  "module.vpc",
					}},
				},
			},
			{Content: `{"changes_overview": "VPC module changes", "risk_level": "medium"}`},
		},
	}

	loop := NewAIAgentLoop(mockCaller, 10)
	agentScope := NewAIAgentTaskScope("ws-test", 1)
	loop.RegisterTool(NewQueryModuleResourcesTool(db, agentScope))
	loop.RegisterTool(NewQueryCMDBDependenciesTool(db, agentScope))
	loop.RegisterTool(NewQueryResourceAttributesTool(db, agentScope))
	loop.RegisterTool(NewQueryStateResourcesTool(db, agentScope))

	result, err := loop.Run(context.Background(), "You are an analyst", "Analyze changes to module.vpc")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify loop ran correctly
	if !result.Completed {
		t.Error("expected loop to complete normally")
	}
	if result.TotalSteps != 2 {
		t.Errorf("expected 2 steps, got %d", result.TotalSteps)
	}

	// Verify tool was called
	if len(result.ToolCalls) != 1 {
		t.Fatalf("expected 1 tool call, got %d", len(result.ToolCalls))
	}
	tc := result.ToolCalls[0]
	if tc.ToolName != "query_module_resources" {
		t.Errorf("expected query_module_resources, got %s", tc.ToolName)
	}
	if tc.Error != "" {
		t.Errorf("tool call should have no error, got: %s", tc.Error)
	}

	// Verify tool returned actual data from DB
	resultJSON, _ := json.Marshal(tc.Result)
	if !strings.Contains(string(resultJSON), "vpc-123") {
		t.Error("tool result should contain seeded resource data")
	}
	if strings.Contains(string(resultJSON), "vpc-foreign") {
		t.Error("model-supplied workspace_id must not escape the task workspace")
	}

	// Verify final output
	if !strings.Contains(result.FinalOutput, "changes_overview") {
		t.Error("final output should contain analysis JSON")
	}
}

func TestAgentLoopWithMultipleToolCalls(t *testing.T) {
	db := setupIntegrationTestDB(t)

	// Seed data
	db.Create(&models.ResourceIndex{
		WorkspaceID: "ws-test", TerraformAddress: "aws_instance.web",
		ResourceType: "aws_instance", ResourceName: "web", ResourceMode: "managed",
		CloudResourceID: "i-123",
		Attributes:      json.RawMessage(`{"vpc_id": "vpc-abc", "security_group_ids": ["sg-789"]}`),
	})

	// AI calls two tools then finishes
	mockCaller := &sequentialMockAICaller{
		responses: []*AgentAIResponse{
			{ToolCalls: []AgentToolCall{
				{ID: "tc_1", Name: "query_resource_attributes", Params: map[string]interface{}{
					"cloud_resource_id": "i-123",
				}},
			}},
			{ToolCalls: []AgentToolCall{
				{ID: "tc_2", Name: "query_state_resources", Params: map[string]interface{}{
					"workspace_id": "ws-test",
				}},
			}},
			{Content: "Analysis complete"},
		},
	}

	loop := NewAIAgentLoop(mockCaller, 10)
	agentScope := NewAIAgentTaskScope("ws-test", 1)
	loop.RegisterTool(NewQueryResourceAttributesTool(db, agentScope))
	loop.RegisterTool(NewQueryStateResourcesTool(db, agentScope))

	result, err := loop.Run(context.Background(), "system", "analyze")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result.TotalSteps != 3 {
		t.Errorf("expected 3 steps, got %d", result.TotalSteps)
	}
	if len(result.ToolCalls) != 2 {
		t.Errorf("expected 2 tool call records, got %d", len(result.ToolCalls))
	}
	if result.ToolCalls[0].ToolName != "query_resource_attributes" {
		t.Errorf("1st tool should be query_resource_attributes, got %s", result.ToolCalls[0].ToolName)
	}
	if result.ToolCalls[1].ToolName != "query_state_resources" {
		t.Errorf("2nd tool should be query_state_resources, got %s", result.ToolCalls[1].ToolName)
	}
}
