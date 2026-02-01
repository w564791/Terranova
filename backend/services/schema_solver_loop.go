package services

import (
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"log"
	"strings"

	"gorm.io/gorm"
)

// ========== AI 反馈循环 ==========

// AIFeedbackLoop AI 反馈循环
type AIFeedbackLoop struct {
	db            *gorm.DB
	solver        *SchemaSolver
	aiService     *AIFormService
	configService *AIConfigService
	maxRetries    int
}

// NewAIFeedbackLoop 创建 AI 反馈循环
func NewAIFeedbackLoop(db *gorm.DB, moduleID uint) *AIFeedbackLoop {
	return &AIFeedbackLoop{
		db:            db,
		solver:        NewSchemaSolver(db, moduleID),
		aiService:     NewAIFormService(db),
		configService: NewAIConfigService(db),
		maxRetries:    3, // 最多重试 3 次
	}
}

// IterationResult 迭代结果
type IterationResult struct {
	Iteration   int                    `json:"iteration"`
	Input       map[string]interface{} `json:"input"`
	Output      *SolverResult          `json:"output"`
	AIResponse  interface{}            `json:"ai_response,omitempty"`
	AIReasoning string                 `json:"ai_reasoning,omitempty"`
}

// FeedbackLoopResult 反馈循环结果
type FeedbackLoopResult struct {
	Success      bool                   `json:"success"`
	FinalParams  map[string]interface{} `json:"final_params"`
	Iterations   []*IterationResult     `json:"iterations"`
	TotalRetries int                    `json:"total_retries"`
	Error        string                 `json:"error,omitempty"`
}

// ExecuteWithRetry 执行带重试的组装
func (loop *AIFeedbackLoop) ExecuteWithRetry(
	userRequest string,
	initialParams map[string]interface{},
	aiConfig *models.AIConfig,
) (*FeedbackLoopResult, error) {
	iterations := make([]*IterationResult, 0)
	currentParams := initialParams

	for i := 0; i < loop.maxRetries; i++ {
		log.Printf("[AIFeedbackLoop] 迭代 %d: 开始验证参数", i+1)

		// 执行组装
		result := loop.solver.Solve(currentParams)

		iteration := &IterationResult{
			Iteration: i + 1,
			Input:     currentParams,
			Output:    result,
		}
		iterations = append(iterations, iteration)

		// 如果成功，返回结果
		if result.Success {
			log.Printf("[AIFeedbackLoop] 迭代 %d: 验证成功", i+1)
			return &FeedbackLoopResult{
				Success:      true,
				FinalParams:  result.Params,
				Iterations:   iterations,
				TotalRetries: i,
			}, nil
		}

		// 如果不需要 AI 修复（比如是配置问题），直接返回
		if !result.NeedAIFix {
			log.Printf("[AIFeedbackLoop] 迭代 %d: 验证失败但无法通过 AI 修复", i+1)
			return &FeedbackLoopResult{
				Success:      false,
				FinalParams:  result.Params,
				Iterations:   iterations,
				TotalRetries: i,
				Error:        "验证失败但无法通过 AI 修复",
			}, nil
		}

		// 如果是最后一次迭代，不再调用 AI
		if i == loop.maxRetries-1 {
			log.Printf("[AIFeedbackLoop] 达到最大重试次数 %d", loop.maxRetries)
			break
		}

		// 构建 AI 提示
		prompt := loop.buildAIPrompt(userRequest, currentParams, result, i+1)

		// 调用 AI 重新生成参数
		log.Printf("[AIFeedbackLoop] 迭代 %d: 调用 AI 修正参数", i+1)
		aiResponse, err := loop.aiService.callAI(aiConfig, prompt)
		if err != nil {
			log.Printf("[AIFeedbackLoop] AI 调用失败: %v", err)
			return &FeedbackLoopResult{
				Success:      false,
				FinalParams:  result.Params,
				Iterations:   iterations,
				TotalRetries: i + 1,
				Error:        fmt.Sprintf("AI 调用失败: %v", err),
			}, nil
		}

		// 解析 AI 响应
		correctedParams, reasoning, err := loop.parseAIResponse(aiResponse)
		if err != nil {
			log.Printf("[AIFeedbackLoop] 解析 AI 响应失败: %v", err)
			return &FeedbackLoopResult{
				Success:      false,
				FinalParams:  result.Params,
				Iterations:   iterations,
				TotalRetries: i + 1,
				Error:        fmt.Sprintf("解析 AI 响应失败: %v", err),
			}, nil
		}

		iteration.AIResponse = correctedParams
		iteration.AIReasoning = reasoning

		// 使用修正后的参数进行下一次迭代
		currentParams = correctedParams
		log.Printf("[AIFeedbackLoop] 迭代 %d: AI 修正了参数，准备下一次验证", i+1)
	}

	// 达到最大重试次数
	lastIteration := iterations[len(iterations)-1]
	return &FeedbackLoopResult{
		Success:      false,
		FinalParams:  lastIteration.Output.Params,
		Iterations:   iterations,
		TotalRetries: loop.maxRetries,
		Error:        fmt.Sprintf("达到最大重试次数 (%d)，验证仍然失败", loop.maxRetries),
	}, nil
}

// buildAIPrompt 构建 AI 提示
func (loop *AIFeedbackLoop) buildAIPrompt(
	userRequest string,
	currentParams map[string]interface{},
	result *SolverResult,
	iteration int,
) string {
	paramsJSON, _ := json.MarshalIndent(currentParams, "", "  ")

	var sb strings.Builder

	sb.WriteString("你正在帮助生成有效的基础设施配置参数。\n\n")

	sb.WriteString("【原始用户请求】\n")
	sb.WriteString(userRequest)
	sb.WriteString("\n\n")

	sb.WriteString(fmt.Sprintf("【迭代次数】%d\n\n", iteration))

	sb.WriteString("【你之前的尝试】\n")
	sb.WriteString(string(paramsJSON))
	sb.WriteString("\n\n")

	sb.WriteString("【验证结果】\n")
	sb.WriteString(loop.formatValidationResults(result))
	sb.WriteString("\n\n")

	sb.WriteString("【需要修复的问题】\n")
	sb.WriteString(result.AIInstructions)
	sb.WriteString("\n\n")

	sb.WriteString(`【重要提醒】
- 保持对原始用户请求的忠实
- 遵守所有 Schema 约束
- 为你的选择提供清晰的理由
- 如果需要做权衡，请解释

请以 JSON 格式提供修正后的参数和解释。

输出格式：
{
  "corrected_params": { ... },
  "changes": [
    {
      "field": "字段名",
      "action": "你做了什么",
      "reason": "为什么这样做"
    }
  ]
}
`)

	return sb.String()
}

// formatValidationResults 格式化验证结果
func (loop *AIFeedbackLoop) formatValidationResults(result *SolverResult) string {
	var sb strings.Builder

	if len(result.AppliedRules) > 0 {
		sb.WriteString("✅ 已应用的规则：\n")
		for _, rule := range result.AppliedRules {
			sb.WriteString(fmt.Sprintf("  - %s\n", rule))
		}
		sb.WriteString("\n")
	}

	if len(result.Warnings) > 0 {
		sb.WriteString("⚠️ 警告：\n")
		for _, warning := range result.Warnings {
			sb.WriteString(fmt.Sprintf("  - %s\n", warning))
		}
		sb.WriteString("\n")
	}

	errorCount := 0
	for _, feedback := range result.Feedbacks {
		if feedback.Type == FeedbackTypeError {
			errorCount++
		}
	}

	if errorCount > 0 {
		sb.WriteString(fmt.Sprintf("❌ 错误: 发现 %d 个问题\n", errorCount))
	}

	return sb.String()
}

// parseAIResponse 解析 AI 响应
func (loop *AIFeedbackLoop) parseAIResponse(response string) (map[string]interface{}, string, error) {
	// 提取 JSON
	jsonStr := extractJSON(response)
	if jsonStr == "" {
		return nil, "", fmt.Errorf("无法从 AI 响应中提取 JSON")
	}

	var result map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &result); err != nil {
		// 尝试修复不完整的 JSON
		fixedJSON := fixIncompleteJSON(jsonStr)
		if err2 := json.Unmarshal([]byte(fixedJSON), &result); err2 != nil {
			return nil, "", fmt.Errorf("无法解析 AI 响应: %w", err)
		}
	}

	// 提取 corrected_params
	correctedParams, ok := result["corrected_params"].(map[string]interface{})
	if !ok {
		// 如果没有 corrected_params，尝试直接使用整个结果作为参数
		// 这是为了兼容 AI 可能直接返回参数的情况
		if _, hasConfig := result["config"]; hasConfig {
			if config, ok := result["config"].(map[string]interface{}); ok {
				correctedParams = config
			}
		} else {
			// 检查是否有其他可能的参数字段
			for key, value := range result {
				if key != "changes" && key != "reasoning" && key != "status" && key != "message" {
					if params, ok := value.(map[string]interface{}); ok {
						correctedParams = params
						break
					}
				}
			}
		}

		if correctedParams == nil {
			return nil, "", fmt.Errorf("AI 响应缺少 'corrected_params' 字段")
		}
	}

	// 提取 reasoning
	reasoning := ""
	if r, ok := result["reasoning"].(string); ok {
		reasoning = r
	} else if changes, ok := result["changes"].([]interface{}); ok {
		// 从 changes 构建 reasoning
		var reasoningParts []string
		for _, change := range changes {
			if changeMap, ok := change.(map[string]interface{}); ok {
				field, _ := changeMap["field"].(string)
				action, _ := changeMap["action"].(string)
				reason, _ := changeMap["reason"].(string)
				reasoningParts = append(reasoningParts,
					fmt.Sprintf("- %s: %s (%s)", field, action, reason))
			}
		}
		if len(reasoningParts) > 0 {
			reasoning = "所做的更改：\n" + strings.Join(reasoningParts, "\n")
		}
	}

	return correctedParams, reasoning, nil
}

// SetMaxRetries 设置最大重试次数
func (loop *AIFeedbackLoop) SetMaxRetries(maxRetries int) {
	if maxRetries > 0 && maxRetries <= 5 {
		loop.maxRetries = maxRetries
	}
}

// GetSolver 获取 SchemaSolver
func (loop *AIFeedbackLoop) GetSolver() *SchemaSolver {
	return loop.solver
}
