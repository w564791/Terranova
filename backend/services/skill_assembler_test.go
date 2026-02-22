package services

import (
	"iac-platform/internal/models"
	"strings"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// ========== EvaluateCondition Tests ==========

func TestEvaluateCondition_EqualsOperator(t *testing.T) {
	a := &SkillAssembler{}
	ctx := &DynamicContext{
		UseCMDB:        true,
		ModuleID:       42,
		WorkspaceID:    "ws-123",
		OrganizationID: "org-456",
	}

	tests := []struct {
		name      string
		condition string
		expected  bool
	}{
		{"use_cmdb true", `use_cmdb == true`, true},
		{"use_cmdb false", `use_cmdb == false`, false},
		{"module_id match", `module_id == 42`, true},
		{"module_id no match", `module_id == 99`, false},
		{"workspace_id match", `workspace_id == ws-123`, true},
		{"organization_id match", `organization_id == org-456`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.EvaluateCondition(tt.condition, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%q): got %v, want %v", tt.condition, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_NotEqualsOperator(t *testing.T) {
	a := &SkillAssembler{}
	ctx := &DynamicContext{UseCMDB: true, WorkspaceID: "ws-123"}

	tests := []struct {
		name      string
		condition string
		expected  bool
	}{
		{"not equals true", `workspace_id != ws-999`, true},
		{"not equals false", `workspace_id != ws-123`, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.EvaluateCondition(tt.condition, ctx)
			if got != tt.expected {
				t.Errorf("EvaluateCondition(%q): got %v, want %v", tt.condition, got, tt.expected)
			}
		})
	}
}

func TestEvaluateCondition_ExtraContext(t *testing.T) {
	a := &SkillAssembler{}
	ctx := &DynamicContext{
		ExtraContext: map[string]interface{}{
			"provider": "aws",
		},
	}

	if !a.EvaluateCondition(`provider == aws`, ctx) {
		t.Error("should read from ExtraContext")
	}
	if a.EvaluateCondition(`provider == gcp`, ctx) {
		t.Error("should not match wrong ExtraContext value")
	}
}

func TestEvaluateCondition_NilContext(t *testing.T) {
	a := &SkillAssembler{}
	if a.EvaluateCondition(`use_cmdb == true`, nil) {
		t.Error("nil context should return false")
	}
}

func TestEvaluateCondition_InvalidFormat(t *testing.T) {
	a := &SkillAssembler{}
	ctx := &DynamicContext{UseCMDB: true}

	tests := []struct {
		name      string
		condition string
	}{
		{"no operator", "use_cmdb"},
		{"empty", ""},
		{"just spaces", "   "},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if a.EvaluateCondition(tt.condition, ctx) {
				t.Errorf("invalid condition %q should return false", tt.condition)
			}
		})
	}
}

func TestEvaluateCondition_QuotedValues(t *testing.T) {
	a := &SkillAssembler{}
	ctx := &DynamicContext{WorkspaceID: "ws-123"}

	// The implementation strips quotes from expected value
	if !a.EvaluateCondition(`workspace_id == "ws-123"`, ctx) {
		t.Error("should handle double-quoted values")
	}
	if !a.EvaluateCondition(`workspace_id == 'ws-123'`, ctx) {
		t.Error("should handle single-quoted values")
	}
}

// ========== fillDynamicContext Tests ==========

func TestFillDynamicContext_AllPlaceholders(t *testing.T) {
	a := &SkillAssembler{}
	template := "User: {user_description}, WS: {workspace_id}, Org: {organization_id}, CMDB: {cmdb_data}, Schema: {schema_data}"
	ctx := &DynamicContext{
		UserDescription: "create a bucket",
		WorkspaceID:     "ws-001",
		OrganizationID:  "org-001",
		CMDBData:        "vpc-123",
		SchemaData:      "{...schema...}",
	}

	result := a.fillDynamicContext(template, ctx)

	if strings.Contains(result, "{user_description}") {
		t.Error("should replace {user_description}")
	}
	if !strings.Contains(result, "create a bucket") {
		t.Error("should contain user description")
	}
	if !strings.Contains(result, "ws-001") {
		t.Error("should contain workspace ID")
	}
	if !strings.Contains(result, "org-001") {
		t.Error("should contain organization ID")
	}
	if !strings.Contains(result, "vpc-123") {
		t.Error("should contain CMDB data")
	}
}

func TestFillDynamicContext_UserRequestAlias(t *testing.T) {
	a := &SkillAssembler{}
	template := "Request: {user_request}"
	ctx := &DynamicContext{UserDescription: "deploy EC2"}

	result := a.fillDynamicContext(template, ctx)
	if !strings.Contains(result, "deploy EC2") {
		t.Error("{user_request} should map to UserDescription")
	}
}

func TestFillDynamicContext_SchemaConstraintsAlias(t *testing.T) {
	a := &SkillAssembler{}
	template := "Constraints: {schema_constraints}"
	ctx := &DynamicContext{SchemaData: "schema-info"}

	result := a.fillDynamicContext(template, ctx)
	if !strings.Contains(result, "schema-info") {
		t.Error("{schema_constraints} should map to SchemaData")
	}
}

func TestFillDynamicContext_ExtraContext(t *testing.T) {
	a := &SkillAssembler{}
	template := "Provider: {provider}, Region: {region}"
	ctx := &DynamicContext{
		ExtraContext: map[string]interface{}{
			"provider": "aws",
			"region":   "us-east-1",
		},
	}

	result := a.fillDynamicContext(template, ctx)
	if !strings.Contains(result, "aws") {
		t.Error("should replace ExtraContext provider")
	}
	if !strings.Contains(result, "us-east-1") {
		t.Error("should replace ExtraContext region")
	}
}

func TestFillDynamicContext_NilContext(t *testing.T) {
	a := &SkillAssembler{}
	template := "unchanged {user_description}"
	result := a.fillDynamicContext(template, nil)
	if result != template {
		t.Error("nil context should return template unchanged")
	}
}

// ========== sortSkills Tests ==========

func TestSortSkills_LayerOrder(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "task1", Name: "task_skill", Layer: models.SkillLayerTask, SourceType: models.SkillSourceManual, Priority: 1, IsActive: true},
		{ID: "found1", Name: "foundation_skill", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Priority: 1, IsActive: true},
		{ID: "domain1", Name: "domain_skill", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Priority: 1, IsActive: true},
	}

	sorted := a.sortSkills(skills)
	if len(sorted) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(sorted))
	}
	if sorted[0].Layer != models.SkillLayerFoundation {
		t.Errorf("first should be foundation, got %s", sorted[0].Layer)
	}
	if sorted[1].Layer != models.SkillLayerDomain {
		t.Errorf("second should be domain, got %s", sorted[1].Layer)
	}
	if sorted[2].Layer != models.SkillLayerTask {
		t.Errorf("third should be task, got %s", sorted[2].Layer)
	}
}

func TestSortSkills_SourceTypeOrder(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "auto1", Name: "auto", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceModuleAuto, Priority: 1, IsActive: true},
		{ID: "manual1", Name: "manual", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Priority: 1, IsActive: true},
		{ID: "hybrid1", Name: "hybrid", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceHybrid, Priority: 1, IsActive: true},
	}

	sorted := a.sortSkills(skills)
	if sorted[0].SourceType != models.SkillSourceManual {
		t.Errorf("first domain skill should be manual, got %s", sorted[0].SourceType)
	}
	if sorted[1].SourceType != models.SkillSourceHybrid {
		t.Errorf("second domain skill should be hybrid, got %s", sorted[1].SourceType)
	}
	if sorted[2].SourceType != models.SkillSourceModuleAuto {
		t.Errorf("third domain skill should be module_auto, got %s", sorted[2].SourceType)
	}
}

func TestSortSkills_Deduplication(t *testing.T) {
	a := &SkillAssembler{}
	skill := &models.Skill{ID: "dup1", Name: "dup", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, IsActive: true}
	skills := []*models.Skill{skill, skill, skill}

	sorted := a.sortSkills(skills)
	if len(sorted) != 1 {
		t.Errorf("expected 1 unique skill, got %d", len(sorted))
	}
}

func TestSortSkills_NilSkipsGracefully(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		nil,
		{ID: "real1", Name: "real", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, IsActive: true},
		nil,
	}

	sorted := a.sortSkills(skills)
	if len(sorted) != 1 {
		t.Errorf("expected 1 skill (nils skipped), got %d", len(sorted))
	}
}

func TestSortSkills_PriorityWithinSameLayerAndSource(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "p3", Name: "low", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Priority: 30, IsActive: true},
		{ID: "p1", Name: "high", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Priority: 10, IsActive: true},
		{ID: "p2", Name: "mid", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Priority: 20, IsActive: true},
	}

	sorted := a.sortSkills(skills)
	if sorted[0].Priority != 10 {
		t.Errorf("first should have priority 10, got %d", sorted[0].Priority)
	}
	if sorted[1].Priority != 20 {
		t.Errorf("second should have priority 20, got %d", sorted[1].Priority)
	}
	if sorted[2].Priority != 30 {
		t.Errorf("third should have priority 30, got %d", sorted[2].Priority)
	}
}

// ========== buildSkillManifest Tests ==========

func TestBuildSkillManifest(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "s1", Name: "placeholder_standard", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Version: "1.0.0", Priority: 1, IsActive: true},
		{ID: "s2", Name: "aws_s3_patterns", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Version: "2.1.0", Priority: 10, IsActive: true},
		{ID: "s3", Name: "inactive_skill", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Version: "1.0.0", Priority: 10, IsActive: false},
	}

	manifest := a.buildSkillManifest(skills)

	if !strings.Contains(manifest, "placeholder_standard") {
		t.Error("manifest should contain active skill name")
	}
	if !strings.Contains(manifest, "aws_s3_patterns") {
		t.Error("manifest should contain second active skill")
	}
	if strings.Contains(manifest, "inactive_skill") {
		t.Error("manifest should not contain inactive skill")
	}
	if !strings.Contains(manifest, "| # |") {
		t.Error("manifest should contain table header")
	}
}

// ========== buildSectionedPrompt Tests ==========

func TestBuildSectionedPrompt(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "s1", Name: "base_rule", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Version: "1.0.0", IsActive: true, Content: "Foundation content"},
		{ID: "s2", Name: "aws_best", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Version: "1.0.0", IsActive: true, Content: "Domain best practice"},
		{ID: "s3", Name: "module_auto", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceModuleAuto, Version: "1.0.0", IsActive: true, Content: "Module constraints"},
		{ID: "s4", Name: "gen_workflow", Layer: models.SkillLayerTask, SourceType: models.SkillSourceManual, Version: "1.0.0", IsActive: true, Content: "Task workflow"},
	}

	result := a.buildSectionedPrompt(skills)

	if !strings.Contains(result, "[Foundation Layer]") {
		t.Error("should contain Foundation Layer section")
	}
	if !strings.Contains(result, "[Domain Layer - Best Practice]") {
		t.Error("should contain Domain Layer - Best Practice section")
	}
	if !strings.Contains(result, "[Domain Layer - Module Constraints]") {
		t.Error("should contain Domain Layer - Module Constraints section")
	}
	if !strings.Contains(result, "[Task Layer]") {
		t.Error("should contain Task Layer section")
	}
	if !strings.Contains(result, "Foundation content") {
		t.Error("should include foundation skill content")
	}
	if !strings.Contains(result, "--- skill: base_rule (v1.0.0) ---") {
		t.Error("should include skill name/version markers")
	}
}

func TestBuildSectionedPrompt_InactiveSkipped(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "s1", Name: "active", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Version: "1.0.0", IsActive: true, Content: "visible"},
		{ID: "s2", Name: "inactive", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Version: "1.0.0", IsActive: false, Content: "hidden"},
	}

	result := a.buildSectionedPrompt(skills)
	if strings.Contains(result, "hidden") {
		t.Error("inactive skill content should not appear")
	}
}

// ========== incrementVersion Tests ==========

func TestIncrementVersion(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"1.0.0", "1.0.1"},
		{"1.0.9", "1.0.10"},
		{"2.3.4", "2.3.5"},
		{"0.0.0", "0.0.1"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := incrementVersion(tt.input)
			if got != tt.expected {
				t.Errorf("incrementVersion(%q): got %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestIncrementVersion_InvalidFormat(t *testing.T) {
	tests := []string{"", "1.0", "abc", "1.0.0.0"}
	for _, input := range tests {
		t.Run(input, func(t *testing.T) {
			got := incrementVersion(input)
			if got != "1.0.1" {
				t.Errorf("invalid format %q should return 1.0.1, got %q", input, got)
			}
		})
	}
}

// ========== truncateString Tests ==========

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxLen   int
		expected string
	}{
		{"short string", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"truncated", "hello world", 5, "hello..."},
		{"empty", "", 5, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxLen)
			if got != tt.expected {
				t.Errorf("truncateString(%q, %d): got %q, want %q", tt.input, tt.maxLen, got, tt.expected)
			}
		})
	}
}

// ========== discoverDomainSkillsFromContent regex Tests ==========
// Note: We test the regex parsing logic. The actual DB loading part
// (loadSkillsByNames) won't be called since we don't set up the DB.
// These tests verify that @require-domain directives are correctly parsed.

// newTestAssemblerWithDB creates a SkillAssembler with in-memory SQLite
func newTestAssemblerWithDB(t *testing.T) *SkillAssembler {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{
		Logger: logger.Discard,
	})
	if err != nil {
		t.Fatalf("failed to open test db: %v", err)
	}
	// Create skills table
	db.Exec(`CREATE TABLE IF NOT EXISTS skills (
		id TEXT PRIMARY KEY,
		name TEXT,
		display_name TEXT,
		layer TEXT,
		content TEXT,
		version TEXT,
		is_active INTEGER,
		priority INTEGER,
		source_type TEXT,
		source_module_id INTEGER,
		metadata TEXT,
		created_at DATETIME,
		updated_at DATETIME
	)`)
	return &SkillAssembler{db: db, skillCache: make(map[string]*models.Skill)}
}

func TestDiscoverDomainSkillsFromContent_SimpleRequire(t *testing.T) {
	a := newTestAssemblerWithDB(t)
	content := `Some task skill content
@require-domain: aws_s3_policy_patterns
@require-domain: aws_policy_core_principles
More content here`

	// DB has no matching skills, so result is empty but no panic
	skills := a.discoverDomainSkillsFromContent(content, nil)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (none in DB), got %d", len(skills))
	}
}

func TestDiscoverDomainSkillsFromContent_ConditionalRequire(t *testing.T) {
	a := newTestAssemblerWithDB(t)
	ctx := &DynamicContext{UseCMDB: true}
	content := `@require-domain-if: use_cmdb == true -> cmdb_helper_skill
@require-domain-if: use_cmdb == false -> no_cmdb_skill`

	// Should parse conditions and evaluate them, no panic
	skills := a.discoverDomainSkillsFromContent(content, ctx)
	if len(skills) != 0 {
		t.Errorf("expected 0 skills (none in DB), got %d", len(skills))
	}
}

// ========== getSectionHeader Tests ==========

func TestGetSectionHeader(t *testing.T) {
	a := &SkillAssembler{}

	tests := []struct {
		name     string
		skill    *models.Skill
		expected string
	}{
		{"foundation", &models.Skill{Layer: models.SkillLayerFoundation}, "[Foundation Layer]"},
		{"domain manual", &models.Skill{Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual}, "[Domain Layer - Best Practice]"},
		{"domain auto", &models.Skill{Layer: models.SkillLayerDomain, SourceType: models.SkillSourceModuleAuto}, "[Domain Layer - Module Constraints]"},
		{"task", &models.Skill{Layer: models.SkillLayerTask}, "[Task Layer]"},
		{"unknown", &models.Skill{Layer: "unknown"}, "[Unknown Layer]"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := a.getSectionHeader(tt.skill)
			if got != tt.expected {
				t.Errorf("getSectionHeader: got %q, want %q", got, tt.expected)
			}
		})
	}
}

// ========== buildMetaRulesPreamble Tests ==========

func TestBuildMetaRulesPreamble_DefaultTemplate(t *testing.T) {
	a := &SkillAssembler{}
	skills := []*models.Skill{
		{ID: "s1", Name: "test_skill", Layer: models.SkillLayerFoundation, SourceType: models.SkillSourceManual, Version: "1.0.0", Priority: 1, IsActive: true},
	}

	result := a.buildMetaRulesPreamble(nil, skills)
	if !strings.Contains(result, "元规则") {
		t.Error("should use default template with 元规则 header")
	}
	if !strings.Contains(result, "test_skill") {
		t.Error("should include skill manifest with skill names")
	}
}

func TestBuildMetaRulesPreamble_CustomTemplate(t *testing.T) {
	a := &SkillAssembler{}
	config := &models.MetaRulesConfig{
		Enabled:  true,
		Template: "Custom rules:\n{skill_manifest}",
	}
	skills := []*models.Skill{
		{ID: "s1", Name: "my_skill", Layer: models.SkillLayerDomain, SourceType: models.SkillSourceManual, Version: "1.0.0", Priority: 1, IsActive: true},
	}

	result := a.buildMetaRulesPreamble(config, skills)
	if !strings.Contains(result, "Custom rules:") {
		t.Error("should use custom template")
	}
	if !strings.Contains(result, "my_skill") {
		t.Error("should fill in skill manifest")
	}
}
