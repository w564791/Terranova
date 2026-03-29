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

// SummaryLLMEvaluator performs L2/L3 LLM evaluation for resource summaries
type SummaryLLMEvaluator struct {
	db            *gorm.DB
	configService *AIConfigService
}

func NewSummaryLLMEvaluator(db *gorm.DB) *SummaryLLMEvaluator {
	return &SummaryLLMEvaluator{
		db:            db,
		configService: NewAIConfigService(db),
	}
}

// EvaluateRule performs L2 evaluation: prompt compliance check
func (e *SummaryLLMEvaluator) EvaluateRule(ctx context.Context, summary, resourceType string, attributes json.RawMessage) (*LLMEvalResult, error) {
	aiConfig, err := e.configService.GetConfigForCapability("summary_rule_evaluation")
	if err != nil {
		return nil, fmt.Errorf("no AI config for summary_rule_evaluation: %w", err)
	}

	promptRules := extractSummaryPromptRules()
	prompt := fmt.Sprintf(`你是 CMDB 资源摘要质量审核员。评估以下摘要是否严格遵守了生成规则。

## 生成规则（摘要必须遵守的）

%s

## 待评估内容

资源类型: %s
原始属性: %s
生成的摘要: %s

## 评估要求

逐条检查每条规则的遵守情况，输出 JSON：
{"verdict": "pass或warn或fail", "score": 0-100, "rule_violations": [{"rule": "规则描述", "detail": "违反详情", "severity": "fail或warn"}], "assessment_confidence": "high或medium或low"}`,
		promptRules, resourceType, truncateForEval(string(attributes)), summary)

	return e.callLLM(ctx, aiConfig, prompt)
}

// EvaluateSemantic performs L3 evaluation: content accuracy and completeness
func (e *SummaryLLMEvaluator) EvaluateSemantic(ctx context.Context, summary, resourceType string, attributes json.RawMessage) (*LLMEvalResult, error) {
	aiConfig, err := e.configService.GetConfigForCapability("summary_semantic_evaluation")
	if err != nil {
		return nil, fmt.Errorf("no AI config for summary_semantic_evaluation: %w", err)
	}

	prompt := fmt.Sprintf(`你是 CMDB 资源摘要质量审核员。评估摘要的内容准确性和完整性。

## 待评估内容

资源类型: %s
原始属性（完整）: %s
生成的摘要: %s

## 评估维度

1. 准确性：摘要中的每个事实是否都能在原始属性中找到依据？
2. 完整性：重要的安全/网络/规格信息是否被遗漏？
3. 幻觉检测：是否包含原始属性中不存在的信息？

输出 JSON：
{"verdict": "pass或warn或fail", "score": 0-100, "quality_issues": [{"type": "hallucination或omission或inaccuracy", "detail": "具体问题", "severity": "fail或warn"}], "assessment_confidence": "high或medium或low"}`,
		resourceType, truncateForEval(string(attributes)), summary)

	return e.callLLM(ctx, aiConfig, prompt)
}

// callLLM sends the prompt to the LLM and parses the response
func (e *SummaryLLMEvaluator) callLLM(ctx context.Context, aiConfig *models.AIConfig, prompt string) (*LLMEvalResult, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	caller := NewAICallerFromConfig(aiConfig)
	messages := []AgentMessage{
		{Role: "user", Content: prompt},
	}

	startTime := time.Now()
	resp, err := caller.ChatWithTools(ctx, messages, nil)
	latencyMs := int(time.Since(startTime).Milliseconds())

	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	result := &LLMEvalResult{
		RawOutput: resp.Content,
		LatencyMs: latencyMs,
		Model:     aiConfig.ModelID,
	}

	jsonStr := extractJSON(resp.Content)
	if err := json.Unmarshal([]byte(jsonStr), result); err != nil {
		log.Printf("[SummaryLLMEvaluator] failed to parse LLM response: %v", err)
		result.Verdict = "warn"
		result.Score = 50
		result.Confidence = "low"
	}

	result.RawOutput = resp.Content
	result.LatencyMs = latencyMs
	result.Model = aiConfig.ModelID

	return result, nil
}

// extractSummaryPromptRules returns the rules section from the default summary prompt
func extractSummaryPromptRules() string {
	lines := strings.Split(defaultResourceSummaryPrompt, "\n")
	var rules []string
	inRules := false
	for _, line := range lines {
		if strings.Contains(line, "严格规则") {
			inRules = true
			continue
		}
		if inRules {
			if strings.Contains(line, "提取维度") {
				break
			}
			rules = append(rules, line)
		}
	}
	return strings.Join(rules, "\n")
}

// truncateForEval truncates attributes for evaluation prompt (max 4000 runes)
func truncateForEval(s string) string {
	const maxLen = 4000
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	return string(runes[:maxLen]) + "...(truncated)"
}
