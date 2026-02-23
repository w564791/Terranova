package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"iac-platform/internal/models"
	"iac-platform/internal/observability/metrics"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
	"gorm.io/gorm"
)

// AIAnalysisService AI 分析服务
type AIAnalysisService struct {
	db            *gorm.DB
	configService *AIConfigService
}

// NewAIAnalysisService 创建 AI 分析服务实例
func NewAIAnalysisService(db *gorm.DB) *AIAnalysisService {
	return &AIAnalysisService{
		db:            db,
		configService: NewAIConfigService(db),
	}
}

// CheckRateLimit 检查速率限制（使用默认 10 秒）
func (s *AIAnalysisService) CheckRateLimit(userID string) (allowed bool, retryAfter int) {
	return s.CheckRateLimitWithConfig(userID, 10)
}

// CheckRateLimitWithConfig 使用配置的频率限制检查
func (s *AIAnalysisService) CheckRateLimitWithConfig(userID string, limitSeconds int) (allowed bool, retryAfter int) {
	var rateLimit models.AIAnalysisRateLimit
	result := s.db.Where("user_id = ?", userID).First(&rateLimit)

	// 如果没有记录，允许分析
	if result.Error != nil {
		return true, 0
	}

	now := time.Now()
	lastAnalysis := rateLimit.LastAnalysisAt

	// 计算距离上次分析的时间（秒）
	elapsedSeconds := int(now.Sub(lastAnalysis).Seconds())

	// 如果 elapsed 是负数（时区问题导致），直接允许
	if elapsedSeconds < 0 {
		return true, 0
	}

	// 检查是否超过限制时间
	if elapsedSeconds < limitSeconds {
		retryAfter = limitSeconds - elapsedSeconds
		return false, retryAfter
	}

	return true, 0
}

// UpdateRateLimit 更新速率限制记录
func (s *AIAnalysisService) UpdateRateLimit(userID string) error {
	now := time.Now()

	// 使用 ON CONFLICT 实现真正的 UPSERT
	return s.db.Exec(`
		INSERT INTO ai_analysis_rate_limits (user_id, last_analysis_at)
		VALUES (?, ?)
		ON CONFLICT (user_id)
		DO UPDATE SET last_analysis_at = EXCLUDED.last_analysis_at
	`, userID, now).Error
}

// BuildPrompt 构建分析 prompt
func (s *AIAnalysisService) BuildPrompt(taskType, tfVersion, errorMessage, customPrompt string) string {
	return s.BuildPromptWithCapability(taskType, tfVersion, errorMessage, customPrompt, "")
}

// BuildPromptWithCapability 构建分析 prompt（支持自定义能力场景 prompt）
func (s *AIAnalysisService) BuildPromptWithCapability(taskType, tfVersion, errorMessage, customPrompt, capabilityPrompt string) string {
	var prompt string

	// 如果有自定义的能力场景 prompt，使用它
	if capabilityPrompt != "" {
		// 替换变量占位符
		prompt = capabilityPrompt
		prompt = strings.ReplaceAll(prompt, "{task_type}", taskType)
		prompt = strings.ReplaceAll(prompt, "{terraform_version}", tfVersion)
		prompt = strings.ReplaceAll(prompt, "{error_message}", errorMessage)
	} else {
		// 使用默认 prompt
		defaultPrompt := `你是一个专业的 Terraform 和云基础设施专家。

【重要规则 - 必须严格遵守】
1. 这是 Terraform %s 执行过程中的报错，请基于 Terraform 和云服务的专业知识进行分析
2. 输出必须精简，但要让人看得懂
3. 每个解决方案需要包含具体的修复建议，可以包含简短的代码示例
4. 根本原因不超过 50 字
5. 每个解决方案不超过 100 字（可包含代码）
6. 预防措施不超过 50 字
7. 必须返回有效的 JSON 格式，不要有任何额外的文字说明或 markdown 标记

【执行环境】
- 执行阶段：%s（plan 表示规划阶段，apply 表示应用阶段）
- Terraform 版本：%s
- 错误来源：Terraform 执行输出

【错误信息】
%s

【输出格式 - 必须严格遵守】
{
  "error_type": "错误类型（从以下选择：配置错误/权限错误/资源冲突/网络错误/语法错误/依赖错误/其他）",
  "root_cause": "根本原因（简洁明了，不超过50字）",
  "solutions": [
    "解决方案1：具体的修复步骤和建议，可包含代码示例（不超过100字）",
    "解决方案2：具体的修复步骤和建议，可包含代码示例（不超过100字）",
    "解决方案3：具体的修复步骤和建议，可包含代码示例（不超过100字）"
  ],
  "prevention": "预防措施（不超过50字）",
  "severity": "严重程度（从以下选择：low/medium/high/critical）"
}

请立即分析并返回纯 JSON 结果，不要有任何额外的解释、说明或 markdown 标记。`

		prompt = fmt.Sprintf(defaultPrompt, taskType, taskType, tfVersion, errorMessage)
	}

	// 如果有自定义 prompt（追加内容），追加到末尾
	if customPrompt != "" {
		prompt += fmt.Sprintf("\n\n【用户补充要求】\n%s", customPrompt)
	}

	return prompt
}

// AnalyzeErrorByTaskID 根据任务ID分析错误（安全版本）
// 安全说明：从数据库获取任务信息，不信任客户端输入，防止 prompt injection 攻击
func (s *AIAnalysisService) AnalyzeErrorByTaskID(taskID uint, userID string) (*AnalysisResult, int, error) {
	// 从数据库获取任务信息
	var task models.WorkspaceTask
	if err := s.db.First(&task, taskID).Error; err != nil {
		return nil, 0, fmt.Errorf("任务不存在")
	}

	// 获取工作空间信息以获取 Terraform 版本
	var workspace models.Workspace
	if err := s.db.Where("workspace_id = ?", task.WorkspaceID).First(&workspace).Error; err != nil {
		return nil, 0, fmt.Errorf("工作空间不存在")
	}

	// 构建错误信息：优先使用 ErrorMessage，如果为空则尝试从 PlanOutput 或 ApplyOutput 中提取
	errorMessage := task.ErrorMessage
	if errorMessage == "" {
		// 如果没有明确的错误信息，尝试从输出中提取
		if task.Status == models.TaskStatusFailed {
			if task.ApplyOutput != "" {
				errorMessage = task.ApplyOutput
			} else if task.PlanOutput != "" {
				errorMessage = task.PlanOutput
			}
		}
	}

	// 如果仍然没有错误信息，返回错误
	if errorMessage == "" {
		return nil, 0, fmt.Errorf("任务没有错误信息，无需分析")
	}

	// 确定任务类型
	taskType := string(task.TaskType)
	if taskType == "" {
		taskType = "plan_and_apply"
	}

	// 获取 Terraform 版本
	tfVersion := workspace.TerraformVersion
	if tfVersion == "" {
		tfVersion = "latest"
	}

	// 调用原有的分析方法
	return s.AnalyzeError(fmt.Sprintf("%d", taskID), userID, errorMessage, taskType, tfVersion)
}

// AnalyzeError 分析错误（内部方法）
// 注意：此方法仅供内部调用，外部应使用 AnalyzeErrorByTaskID
func (s *AIAnalysisService) AnalyzeError(taskID, userID string, errorMessage, taskType, tfVersion string) (*AnalysisResult, int, error) {
	startTime := time.Now()

	// 获取支持错误分析的 AI 配置
	cfg, err := s.configService.GetConfigForCapability("error_analysis")
	if err != nil {
		return nil, 0, fmt.Errorf("无法获取 AI 配置: %w", err)
	}

	if cfg == nil {
		return nil, 0, fmt.Errorf("AI 分析服务未启用")
	}

	// 使用配置中的频率限制检查
	allowed, retryAfter := s.CheckRateLimitWithConfig(userID, cfg.RateLimitSeconds)
	if !allowed {
		return nil, retryAfter, fmt.Errorf("请求过于频繁，请在 %d 秒后重试", retryAfter)
	}

	// 获取自定义的能力场景 prompt（如果有）
	capabilityPrompt := ""
	if cfg.CapabilityPrompts != nil {
		if prompt, ok := cfg.CapabilityPrompts["error_analysis"]; ok && prompt != "" {
			capabilityPrompt = prompt
		}
	}

	// 构建 prompt（支持自定义能力场景 prompt）
	prompt := s.BuildPromptWithCapability(taskType, tfVersion, errorMessage, cfg.CustomPrompt, capabilityPrompt)

	// 根据服务类型调用不同的 API
	var result *AnalysisResult
	switch cfg.ServiceType {
	case "bedrock":
		result, err = s.callBedrock(cfg.AWSRegion, cfg.ModelID, prompt, cfg.UseInferenceProfile)
	case "openai", "azure_openai", "ollama":
		result, err = s.callOpenAICompatible(cfg.BaseURL, cfg.APIKey, cfg.ModelID, prompt)
	default:
		return nil, 0, fmt.Errorf("不支持的服务类型: %s", cfg.ServiceType)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("AI 分析失败: %w", err)
	}

	// 计算耗时
	duration := int(time.Since(startTime).Milliseconds())

	// 保存分析结果
	if err := s.configService.SaveAnalysis(taskID, userID, errorMessage, result, duration); err != nil {
		return nil, 0, fmt.Errorf("保存分析结果失败: %w", err)
	}

	// 更新速率限制
	if err := s.UpdateRateLimit(userID); err != nil {
		return nil, 0, fmt.Errorf("更新速率限制失败: %w", err)
	}

	return result, duration, nil
}

// callBedrock 调用 Bedrock API
func (s *AIAnalysisService) callBedrock(region, modelID, prompt string, useInferenceProfile bool) (*AnalysisResult, error) {
	// 加载 AWS 配置
	cfg, err := config.LoadDefaultConfig(context.TODO(),
		config.WithRegion(region),
	)
	if err != nil {
		return nil, fmt.Errorf("无法加载 AWS 配置: %w", err)
	}

	// 创建 Bedrock Runtime 客户端
	// 禁用自动重试，避免过多请求
	cfg.RetryMaxAttempts = 1
	client := bedrockruntime.NewFromConfig(cfg)

	// 构建请求体（Claude 格式）
	requestBody := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        4000, // 增加 token 限制，避免响应被截断
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("无法序列化请求: %w", err)
	}

	// 根据配置决定使用哪个 model ID
	finalModelID := modelID
	if useInferenceProfile {
		// 如果配置了使用 inference profile，直接使用 cross-region inference profile
		if region == "us-east-1" || region == "us-west-2" {
			finalModelID = fmt.Sprintf("us.%s", modelID)
		} else if region == "eu-west-1" || region == "eu-central-1" {
			finalModelID = fmt.Sprintf("eu.%s", modelID)
		} else if region == "ap-southeast-1" || region == "ap-northeast-1" {
			finalModelID = fmt.Sprintf("apac.%s", modelID)
		}
	}

	// 调用模型（只调用一次，不重试）
	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestBodyJSON,
	}

	output, err := client.InvokeModel(context.TODO(), input)
	if err != nil {
		return nil, fmt.Errorf("调用 Bedrock 失败: %w", err)
	}

	// 解析响应
	var response struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(output.Body, &response); err != nil {
		return nil, fmt.Errorf("无法解析响应: %w", err)
	}

	// 记录 token 用量指标
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		metrics.IncAITokens("bedrock", "prompt", float64(response.Usage.InputTokens))
		metrics.IncAITokens("bedrock", "completion", float64(response.Usage.OutputTokens))
	}

	if len(response.Content) == 0 {
		return nil, fmt.Errorf("响应内容为空")
	}

	// 解析 AI 返回的 JSON
	text := response.Content[0].Text

	// 如果返回的是 markdown 代码块，提取 JSON 内容
	if contains(text, "```json") {
		// 提取 ```json 和 ``` 之间的内容
		start := findSubstring(text, "```json")
		if start {
			text = text[len("```json"):]
			end := findSubstring(text, "```")
			if end {
				endIdx := 0
				for i := 0; i <= len(text)-3; i++ {
					if text[i:i+3] == "```" {
						endIdx = i
						break
					}
				}
				if endIdx > 0 {
					text = text[:endIdx]
				}
			}
		}
	} else if contains(text, "```") {
		// 提取 ``` 和 ``` 之间的内容
		start := 0
		for i := 0; i <= len(text)-3; i++ {
			if text[i:i+3] == "```" {
				start = i + 3
				break
			}
		}
		if start > 0 {
			text = text[start:]
			end := 0
			for i := 0; i <= len(text)-3; i++ {
				if text[i:i+3] == "```" {
					end = i
					break
				}
			}
			if end > 0 {
				text = text[:end]
			}
		}
	}

	// 去除首尾空白
	text = trimSpace(text)

	// 尝试解析 JSON
	var result AnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// 如果解析失败，尝试修复常见问题
		// 1. 检查是否被截断（缺少结尾的 } 或 ]）
		if !contains(text, "}") || !contains(text, "]") {
			return nil, fmt.Errorf("AI 返回的 JSON 不完整，可能被截断。请增加 max_tokens 或简化 prompt")
		}

		// 2. 尝试修复不完整的 JSON（添加缺失的结尾）
		fixedText := text
		openBraces := 0
		openBrackets := 0
		for _, ch := range text {
			if ch == '{' {
				openBraces++
			} else if ch == '}' {
				openBraces--
			} else if ch == '[' {
				openBrackets++
			} else if ch == ']' {
				openBrackets--
			}
		}

		// 添加缺失的结尾
		for i := 0; i < openBrackets; i++ {
			fixedText += "]"
		}
		for i := 0; i < openBraces; i++ {
			fixedText += "}"
		}

		// 尝试解析修复后的 JSON
		if err2 := json.Unmarshal([]byte(fixedText), &result); err2 != nil {
			return nil, fmt.Errorf("无法解析 AI 返回的 JSON: %w (原始内容: %s)", err, text)
		}
	}

	return &result, nil
}

// contains 检查字符串是否包含子串
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

// findSubstring 在字符串中查找子串
func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// callOpenAICompatible 调用 OpenAI Compatible API
func (s *AIAnalysisService) callOpenAICompatible(baseURL, apiKey, modelID, prompt string) (*AnalysisResult, error) {
	// 构建请求体（OpenAI 格式）
	requestBody := map[string]interface{}{
		"model": modelID,
		"messages": []map[string]string{
			{
				"role":    "user",
				"content": prompt,
			},
		},
		"max_tokens":  4000,
		"temperature": 0.7,
	}

	requestBodyJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("无法序列化请求: %w", err)
	}

	// 构建完整的 URL
	url := baseURL
	if url[len(url)-1] != '/' {
		url += "/"
	}
	url += "chat/completions"

	// 创建 HTTP 请求
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(requestBodyJSON))
	if err != nil {
		return nil, fmt.Errorf("无法创建请求: %w", err)
	}

	// 设置请求头
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	// 发送请求
	client := &http.Client{
		Timeout: 60 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("无法读取响应: %w", err)
	}

	// 检查状态码
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API 返回错误状态码 %d: %s", resp.StatusCode, string(body))
	}

	// 解析响应
	var response struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("无法解析响应: %w", err)
	}

	// 记录 token 用量指标
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		metrics.IncAITokens("openai", "prompt", float64(response.Usage.PromptTokens))
		metrics.IncAITokens("openai", "completion", float64(response.Usage.CompletionTokens))
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("响应内容为空")
	}

	// 解析 AI 返回的 JSON
	text := response.Choices[0].Message.Content

	// 记录原始响应元数据
	log.Printf("[AIAnalysis] Received response (length: %d)", len(text))

	// 提取 JSON 内容（处理 markdown 代码块）
	text = extractJSON(text)

	log.Printf("[AIAnalysis] Extracted JSON content (length: %d)", len(text))

	// 尝试解析 JSON
	var result AnalysisResult
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		// 检查是否完全不是 JSON 格式
		if !contains(text, "{") || !contains(text, "}") {
			maxLen := 200
			if len(text) < maxLen {
				maxLen = len(text)
			}
			return nil, fmt.Errorf("AI 返回的内容不是 JSON 格式，请检查 prompt 设置或模型配置。原始内容: %s", text[:maxLen])
		}

		// 尝试修复不完整的 JSON
		fixedText := fixIncompleteJSON(text)
		if err2 := json.Unmarshal([]byte(fixedText), &result); err2 != nil {
			maxLen := 500
			if len(text) < maxLen {
				maxLen = len(text)
			}
			return nil, fmt.Errorf("无法解析 AI 返回的 JSON: %w (原始内容: %s)", err, text[:maxLen])
		}
	}

	return &result, nil
}

// cleanInvalidChars 清理无效的控制字符
// 保留 \t (0x09), \n (0x0A), \r (0x0D)，移除其他控制字符
// 注意：使用 rune 遍历，正确处理多字节 UTF-8 字符（如中文）
func cleanInvalidChars(text string) string {
	log.Printf("[AIAnalysis] cleanInvalidChars input length: %d", len(text))

	var cleaned strings.Builder
	cleaned.Grow(len(text))
	filteredCount := 0
	for _, r := range text {
		// rune 是 Unicode 码点，中文字符的码点远大于 0x9F
		// 只需要过滤 ASCII 控制字符范围 (0x00-0x1F, 0x7F)
		// 以及 C1 控制字符范围 (0x80-0x9F)
		if r >= 0x20 && r < 0x7F {
			// 可打印 ASCII 字符
			cleaned.WriteRune(r)
		} else if r == '\t' || r == '\n' || r == '\r' {
			// 允许的空白字符
			cleaned.WriteRune(r)
		} else if r > 0x9F {
			// 非 ASCII 字符（包括中文、日文等），保留
			cleaned.WriteRune(r)
		} else {
			// 被过滤的字符
			filteredCount++
		}
	}

	result := cleaned.String()
	if filteredCount > 0 {
		log.Printf("[AIAnalysis] cleanInvalidChars filtered %d characters, output length: %d", filteredCount, len(result))
	}

	return result
}

// extractJSON 从文本中提取 JSON 内容（处理 markdown 代码块）
// 版本: 2026-01-31-v3 - 修复了 markdown 代码块提取时的字节索引问题
func extractJSON(text string) string {
	// 首先清理无效的控制字符
	// 注意：cleanInvalidChars 使用 rune 遍历，正确处理多字节 UTF-8 字符
	text = cleanInvalidChars(text)

	// 使用 strings.Index 来正确查找子串位置
	// 如果返回的是 markdown 代码块，提取 JSON 内容
	if idx := strings.Index(text, "```json"); idx >= 0 {
		// 从 "```json" 之后开始
		text = text[idx+len("```json"):]
		// 查找结束的 ```
		if endIdx := strings.Index(text, "```"); endIdx >= 0 {
			text = text[:endIdx]
		}
	} else if idx := strings.Index(text, "```"); idx >= 0 {
		// 从第一个 ``` 之后开始
		text = text[idx+3:]
		// 查找结束的 ```
		if endIdx := strings.Index(text, "```"); endIdx >= 0 {
			text = text[:endIdx]
		}
	}

	return trimSpace(text)
}

// fixIncompleteJSON 修复不完整的 JSON
func fixIncompleteJSON(text string) string {
	openBraces := 0
	openBrackets := 0
	for _, ch := range text {
		if ch == '{' {
			openBraces++
		} else if ch == '}' {
			openBraces--
		} else if ch == '[' {
			openBrackets++
		} else if ch == ']' {
			openBrackets--
		}
	}

	// 添加缺失的结尾
	fixedText := text
	for i := 0; i < openBrackets; i++ {
		fixedText += "]"
	}
	for i := 0; i < openBraces; i++ {
		fixedText += "}"
	}

	return fixedText
}

// trimSpace 去除字符串首尾空白
// 使用 strings.TrimSpace 来正确处理 UTF-8 字符串
func trimSpace(s string) string {
	return strings.TrimSpace(s)
}
