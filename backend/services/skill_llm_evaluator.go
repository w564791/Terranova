package services

import (
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"
	"time"

	"gorm.io/gorm"
)

// SkillLLMEvaluator performs LLM-based evaluation for skill quality assessment.
// It handles Layer 2 (rule) and Layer 3 (semantic) evaluations.
type SkillLLMEvaluator struct {
	db             *gorm.DB
	configService  *AIConfigService
	skillAssembler *SkillAssembler
}

// NewSkillLLMEvaluator creates a new SkillLLMEvaluator instance.
func NewSkillLLMEvaluator(db *gorm.DB) *SkillLLMEvaluator {
	return &SkillLLMEvaluator{
		db:             db,
		configService:  NewAIConfigService(db),
		skillAssembler: NewSkillAssembler(db),
	}
}

// LLMEvalResult holds the result of an LLM evaluation.
type LLMEvalResult struct {
	Verdict        string          `json:"verdict"`
	Score          int             `json:"score"`
	Confidence     string          `json:"assessment_confidence"`
	RuleViolations json.RawMessage `json:"rule_violations,omitempty"`
	QualityIssues  json.RawMessage `json:"quality_issues,omitempty"`
	RawOutput      string          `json:"-"`
	LatencyMs      int             `json:"-"`
	Model          string          `json:"-"`
}

// EvaluateRule performs Layer 2 (rule-based) evaluation using the LLM.
// Uses AI Config capability "skill_rule_evaluation", task_skill points to the evaluation prompt Skill.
func (e *SkillLLMEvaluator) EvaluateRule(ctx context.Context, usageLog *models.SkillUsageLog) (*LLMEvalResult, error) {
	// 1. Get AI config
	aiConfig, err := e.configService.GetConfigForCapability("skill_rule_evaluation")
	if err != nil {
		return nil, fmt.Errorf("no AI config for skill_rule_evaluation: %w", err)
	}

	// 2. Load the rule evaluation skill from AI Config skill_composition.task_skill
	evalSkillName := aiConfig.SkillComposition.TaskSkill
	if evalSkillName == "" {
		evalSkillName = "skill_quality_rule_evaluation"
	}
	evalSkill, err := e.skillAssembler.GetSkillByName(evalSkillName)
	if err != nil {
		return nil, fmt.Errorf("eval skill not found: %s: %w", evalSkillName, err)
	}

	// 3. Get the skill content snapshot for this usage log
	skillContent := e.getSkillContentForLog(usageLog)

	// 4. Extract rules section from skill content
	rulesSection := extractRulesSection(skillContent)

	// 5. Build prompt by replacing variables in the eval skill content
	prompt := evalSkill.Content
	prompt = strings.ReplaceAll(prompt, "{skill_rules_section}", rulesSection)
	prompt = strings.ReplaceAll(prompt, "{input}", snapshotToString(usageLog.InputSnapshot))
	prompt = strings.ReplaceAll(prompt, "{output}", snapshotToString(usageLog.OutputSnapshot))

	// 6. Call LLM
	result, err := e.callLLM(ctx, aiConfig, prompt, "skill_rule_evaluation")
	if err != nil {
		return nil, err
	}

	return result, nil
}

// EvaluateSemantic performs Layer 3 (semantic) evaluation using the LLM.
// It assesses the overall quality and coherence of the skill output.
func (e *SkillLLMEvaluator) EvaluateSemantic(ctx context.Context, usageLog *models.SkillUsageLog) (*LLMEvalResult, error) {
	// 1. Get AI config
	aiConfig, err := e.configService.GetConfigForCapability("skill_semantic_evaluation")
	if err != nil {
		return nil, fmt.Errorf("no AI config for skill_semantic_evaluation: %w", err)
	}

	// 2. Load the semantic evaluation skill from AI Config skill_composition.task_skill
	evalSkillName := aiConfig.SkillComposition.TaskSkill
	if evalSkillName == "" {
		evalSkillName = "skill_quality_semantic_evaluation"
	}
	evalSkill, err := e.skillAssembler.GetSkillByName(evalSkillName)
	if err != nil {
		return nil, fmt.Errorf("eval skill not found: %s: %w", evalSkillName, err)
	}

	// 3. Get the full skill content snapshot for this usage log
	skillContent := e.getSkillContentForLog(usageLog)

	// 4. Build prompt by replacing variables in the eval skill content
	prompt := evalSkill.Content
	prompt = strings.ReplaceAll(prompt, "{skill_md}", skillContent)
	prompt = strings.ReplaceAll(prompt, "{input}", snapshotToString(usageLog.InputSnapshot))
	prompt = strings.ReplaceAll(prompt, "{output}", snapshotToString(usageLog.OutputSnapshot))

	// 5. Call LLM
	result, err := e.callLLM(ctx, aiConfig, prompt, "skill_semantic_evaluation")
	if err != nil {
		return nil, err
	}

	return result, nil
}

// callLLM sends the prompt to the LLM and parses the response into LLMEvalResult.
func (e *SkillLLMEvaluator) callLLM(ctx context.Context, aiConfig *models.AIConfig, prompt string, capability string) (*LLMEvalResult, error) {
	// Apply 30-second timeout
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	caller := e.configService.NewCallerWithFallback(aiConfig, capability)
	messages := []AgentMessage{
		{Role: "user", Content: prompt},
	}

	startTime := time.Now()
	resp, err := caller.ChatWithTools(ctx, messages, nil)
	latencyMs := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response JSON into LLMEvalResult
	result := &LLMEvalResult{
		RawOutput: resp.Content,
		LatencyMs: latencyMs,
		Model:     aiConfig.ModelID,
	}

	// Try to extract JSON from the response (may be wrapped in markdown code blocks)
	jsonStr := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		log.Printf("[SkillLLMEvaluator] failed to parse LLM response as JSON: %v, raw: %s", err, truncateString(resp.Content, 500))
		// Return a fallback result with the raw output preserved
		result.Verdict = "warn"
		result.Score = 50
		result.Confidence = "low"
	}

	// Ensure metadata fields are preserved (json.Unmarshal zeroes them via `json:"-"`)
	result.RawOutput = resp.Content
	result.LatencyMs = latencyMs
	result.Model = aiConfig.ModelID

	return result, nil
}

// getSkillContentForLog retrieves the skill content snapshot associated with the usage log.
// It first checks for a non-null skill_content_snapshot with matching content_hash,
// then falls back to a placeholder.
func (e *SkillLLMEvaluator) getSkillContentForLog(usageLog *models.SkillUsageLog) string {
	// If the usage log itself has a snapshot, use it
	if usageLog.SkillContentSnapshot != nil && *usageLog.SkillContentSnapshot != "" {
		return *usageLog.SkillContentSnapshot
	}

	// Query for first non-null skill_content_snapshot with matching hash
	if usageLog.SkillContentHash != "" {
		var log models.SkillUsageLog
		err := e.db.Where("skill_content_hash = ? AND skill_content_snapshot IS NOT NULL", usageLog.SkillContentHash).
			First(&log).Error
		if err == nil && log.SkillContentSnapshot != nil && *log.SkillContentSnapshot != "" {
			return *log.SkillContentSnapshot
		}
	}

	return "(skill content not available)"
}

// extractRulesSection extracts the rules section from skill content
// delimited by <!-- rules-begin --> and <!-- rules-end --> markers.
// If markers are not found, the full content is returned as fallback.
func extractRulesSection(content string) string {
	beginMarker := "<!-- rules-begin -->"
	endMarker := "<!-- rules-end -->"
	beginIdx := strings.Index(content, beginMarker)
	endIdx := strings.Index(content, endMarker)
	if beginIdx == -1 || endIdx == -1 || endIdx <= beginIdx {
		return content // fallback: use full content
	}
	return content[beginIdx+len(beginMarker) : endIdx]
}

// snapshotToString converts a JSON RawMessage pointer to a string representation.
func snapshotToString(snapshot *json.RawMessage) string {
	if snapshot == nil {
		return "(no data)"
	}
	return string(*snapshot)
}

