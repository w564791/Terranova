package services

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
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
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// ========== AICaller Factory ==========

// NewAICallerFromConfig 根据 AIConfig 创建对应的 AICaller
func NewAICallerFromConfig(cfg *models.AIConfig) AICaller {
	switch cfg.ServiceType {
	case "bedrock":
		return &BedrockCaller{
			region:               cfg.AWSRegion,
			modelID:              cfg.ModelID,
			useInferenceProfile:  cfg.UseInferenceProfile,
			thinkingEnabled:      cfg.ThinkingEnabled,
			thinkingBudgetTokens: cfg.ThinkingBudgetTokens,
			cacheEnabled:         cfg.CacheEnabled,
		}
	case "qwen":
		return &QwenCaller{
			OpenAICaller: OpenAICaller{
				baseURL:     cfg.BaseURL,
				apiKey:      cfg.APIKey,
				modelID:     cfg.ModelID,
				serviceType: cfg.ServiceType,
			},
			thinkingEnabled:      cfg.ThinkingEnabled,
			thinkingBudgetTokens: cfg.ThinkingBudgetTokens,
		}
	case "openai", "azure_openai", "ollama":
		return &OpenAICaller{
			baseURL:     cfg.BaseURL,
			apiKey:      cfg.APIKey,
			modelID:     cfg.ModelID,
			serviceType: cfg.ServiceType,
		}
	default:
		return &BedrockCaller{
			region:               cfg.AWSRegion,
			modelID:              cfg.ModelID,
			thinkingEnabled:      cfg.ThinkingEnabled,
			thinkingBudgetTokens: cfg.ThinkingBudgetTokens,
			cacheEnabled:         cfg.CacheEnabled,
		}
	}
}

// ========== Bedrock Caller (Claude tool_use format) ==========

// BedrockCaller Bedrock Claude tool calling 实现
type BedrockCaller struct {
	region               string
	modelID              string
	useInferenceProfile  bool
	thinkingEnabled      bool
	thinkingBudgetTokens int
	cacheEnabled         bool
}

// isGLMModel 判断是否为 GLM 模型（Z.AI on Bedrock）
func (c *BedrockCaller) isGLMModel() bool {
	return strings.HasPrefix(c.modelID, "zai.")
}

// ChatWithTools 调用 Bedrock（自动识别 Claude / GLM 格式）
func (c *BedrockCaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(c.region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	awsCfg.RetryMaxAttempts = 1
	client := bedrockruntime.NewFromConfig(awsCfg)

	// 根据模型类型构建不同格式的请求体
	var requestBody map[string]interface{}
	if c.isGLMModel() {
		requestBody = c.buildGLMRequest(messages, tools)
	} else {
		requestBody = c.buildBedrockRequest(messages, tools)
	}

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 确定 model ID（GLM 不使用 inference profile）
	finalModelID := c.resolveModelID()
	logBedrockRequestFingerprint(finalModelID, messages, tools)

	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestJSON,
	}

	output, err := client.InvokeModel(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock invocation failed: %w", err)
	}

	if c.isGLMModel() {
		return c.parseGLMResponse(output.Body)
	}
	return c.parseBedrockResponse(output.Body)
}

func logBedrockRequestFingerprint(modelID string, messages []AgentMessage, tools []AgentToolDef) {
	systemPrompt := ""
	roleCounts := make(map[string]int)
	for _, msg := range messages {
		roleCounts[msg.Role]++
		if msg.Role == "system" && systemPrompt == "" {
			systemPrompt = msg.Content
		}
	}
	toolsJSON, _ := json.Marshal(tools)
	log.Printf("[AICaller/Bedrock] request: model=%s system_hash=%s system_len=%d tools_hash=%s tools=%d messages=%d roles=%v",
		modelID,
		shortSHA256(systemPrompt),
		len(systemPrompt),
		shortSHA256(string(toolsJSON)),
		len(tools),
		len(messages),
		roleCounts,
	)
}

func shortSHA256(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])[:12]
}

// buildBedrockRequest 构建 Bedrock Claude 请求体（支持 tool_use）
func (c *BedrockCaller) buildBedrockRequest(messages []AgentMessage, tools []AgentToolDef) map[string]interface{} {
	maxTokens := 8000

	// Extended thinking: inject thinking block (max_tokens must > budget_tokens, temperature must not be set)
	if c.thinkingEnabled && c.thinkingBudgetTokens >= 1024 {
		if maxTokens <= c.thinkingBudgetTokens {
			maxTokens = c.thinkingBudgetTokens + 4000
		}
	}

	body := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        maxTokens,
	}

	if c.thinkingEnabled && c.thinkingBudgetTokens >= 1024 {
		body["thinking"] = map[string]interface{}{
			"type":          "enabled",
			"budget_tokens": c.thinkingBudgetTokens,
		}
	}

	// 构建 messages（提取 system 到顶层）
	var systemPrompt string
	var claudeMessages []interface{}

	for _, msg := range messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}

		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			// assistant 消息带 tool_use（回传 thinking 保持推理连贯性）
			content := make([]interface{}, 0)
			if msg.ThinkingContent != "" {
				thinkingBlock := map[string]interface{}{
					"type":     "thinking",
					"thinking": msg.ThinkingContent,
				}
				if msg.ThinkingSignature != "" {
					thinkingBlock["signature"] = msg.ThinkingSignature
				}
				content = append(content, thinkingBlock)
			}
			if msg.Content != "" {
				content = append(content, map[string]interface{}{
					"type": "text",
					"text": msg.Content,
				})
			}
			for _, tc := range msg.ToolCalls {
				content = append(content, map[string]interface{}{
					"type":  "tool_use",
					"id":    tc.ID,
					"name":  tc.Name,
					"input": tc.Params,
				})
			}
			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role":    "assistant",
				"content": content,
			})
		} else if msg.Role == "tool" {
			// tool_result
			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role": "user",
				"content": []interface{}{
					map[string]interface{}{
						"type":        "tool_result",
						"tool_use_id": msg.ToolCallID,
						"content":     msg.Content,
					},
				},
			})
		} else {
			// user / assistant（纯文本）
			claudeMessages = append(claudeMessages, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}

	if systemPrompt != "" {
		if c.cacheEnabled {
			body["system"] = []interface{}{
				map[string]interface{}{
					"type":          "text",
					"text":          systemPrompt,
					"cache_control": map[string]interface{}{"type": "ephemeral"},
				},
			}
		} else {
			body["system"] = []interface{}{
				map[string]interface{}{
					"type": "text",
					"text": systemPrompt,
				},
			}
		}
	}
	body["messages"] = claudeMessages

	// 工具定义
	if len(tools) > 0 {
		var toolDefs []interface{}
		for _, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			// 确保 schema 有 type 字段
			if _, ok := schema["type"]; !ok {
				schema = map[string]interface{}{
					"type":       "object",
					"properties": schema,
				}
			}
			toolDefs = append(toolDefs, map[string]interface{}{
				"name":         t.Name,
				"description":  t.Description,
				"input_schema": schema,
			})
		}
		body["tools"] = toolDefs
	}

	return body
}

// resolveModelID 返回最终的 model ID
// use_inference_profile=true 时，modelID 已是用户选择的完整 inference profile ID，直接返回
// use_inference_profile=false 时，modelID 是基础模型 ID，直接返回
func (c *BedrockCaller) resolveModelID() string {
	return c.modelID
}

// parseBedrockResponse 解析 Bedrock 响应
func (c *BedrockCaller) parseBedrockResponse(body []byte) (*AgentAIResponse, error) {
	var response struct {
		Content []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text,omitempty"`
			Thinking  string          `json:"thinking,omitempty"`  // extended thinking content
			Signature string          `json:"signature,omitempty"` // thinking signature（回传用）
			ID        string          `json:"id,omitempty"`
			Name      string          `json:"name,omitempty"`
			Input     json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens              int `json:"input_tokens"`
			OutputTokens             int `json:"output_tokens"`
			CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
			CacheReadInputTokens     int `json:"cache_read_input_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse bedrock response: %w", err)
	}

	// 记录 token 指标(含 cache 指标)
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		metrics.IncAITokens("bedrock", "prompt", float64(response.Usage.InputTokens))
		metrics.IncAITokens("bedrock", "completion", float64(response.Usage.OutputTokens))
		log.Printf("[AICaller/Bedrock] tokens: input=%d, output=%d, cache_create=%d, cache_read=%d",
			response.Usage.InputTokens, response.Usage.OutputTokens,
			response.Usage.CacheCreationInputTokens, response.Usage.CacheReadInputTokens)
	}

	result := &AgentAIResponse{}
	var textParts []string
	var blockTypes []string

	for _, block := range response.Content {
		blockTypes = append(blockTypes, block.Type)
		switch block.Type {
		case "thinking":
			result.ThinkingContent = block.Thinking
			result.ThinkingSignature = block.Signature
		case "text":
			textParts = append(textParts, block.Text)
		case "tool_use":
			var params map[string]interface{}
			if len(block.Input) > 0 {
				if err := json.Unmarshal(block.Input, &params); err != nil {
					log.Printf("[AICaller/Bedrock] failed to parse tool input for %s: %v", block.Name, err)
				}
			}
			result.ToolCalls = append(result.ToolCalls, AgentToolCall{
				ID:     block.ID,
				Name:   block.Name,
				Params: params,
			})
		}
	}

	result.Content = strings.Join(textParts, "\n")
	log.Printf("[AICaller/Bedrock] Response block types: %v, thinking=%d chars", blockTypes, len(result.ThinkingContent))
	return result, nil
}

// ========== Bedrock GLM (Z.AI) — OpenAI 兼容格式通过 Bedrock InvokeModel ==========

// buildGLMRequest 构建 GLM 请求体（OpenAI chat completion 格式）
func (c *BedrockCaller) buildGLMRequest(messages []AgentMessage, tools []AgentToolDef) map[string]interface{} {
	body := map[string]interface{}{
		"model":       c.modelID,
		"max_tokens":  4096,
		"temperature": 0.7,
	}

	// 构建 messages（OpenAI 格式，system 作为 message role）
	var glmMessages []interface{}
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCalls := make([]interface{}, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				paramsJSON, _ := json.Marshal(tc.Params)
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(paramsJSON),
					},
				})
			}
			glmMessages = append(glmMessages, map[string]interface{}{
				"role":       "assistant",
				"content":    msg.Content,
				"tool_calls": toolCalls,
			})
		} else if msg.Role == "tool" {
			glmMessages = append(glmMessages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": msg.ToolCallID,
				"content":      msg.Content,
			})
		} else {
			glmMessages = append(glmMessages, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}
	body["messages"] = glmMessages

	// 工具定义（OpenAI function calling 格式）
	if len(tools) > 0 {
		var toolDefs []interface{}
		for _, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			if _, ok := schema["type"]; !ok {
				schema = map[string]interface{}{
					"type":       "object",
					"properties": schema,
				}
			}
			toolDefs = append(toolDefs, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  schema,
				},
			})
		}
		body["tools"] = toolDefs
		body["tool_choice"] = "auto"
	}

	return body
}

// parseGLMResponse 解析 GLM 响应（OpenAI choices 格式）
func (c *BedrockCaller) parseGLMResponse(body []byte) (*AgentAIResponse, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse GLM response: %w", err)
	}

	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		metrics.IncAITokens("bedrock-glm", "prompt", float64(response.Usage.PromptTokens))
		metrics.IncAITokens("bedrock-glm", "completion", float64(response.Usage.CompletionTokens))
		log.Printf("[AICaller/BedrockGLM] tokens: prompt=%d, completion=%d", response.Usage.PromptTokens, response.Usage.CompletionTokens)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("empty response from GLM API")
	}

	msg := response.Choices[0].Message
	result := &AgentAIResponse{Content: msg.Content}

	for _, tc := range msg.ToolCalls {
		var params map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				log.Printf("[AICaller/BedrockGLM] failed to parse tool arguments for %s: %v", tc.Function.Name, err)
			}
		}
		result.ToolCalls = append(result.ToolCalls, AgentToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: params,
		})
	}

	log.Printf("[AICaller/BedrockGLM] Response: content=%d chars, tool_calls=%d", len(result.Content), len(result.ToolCalls))
	return result, nil
}

// ========== OpenAI Compatible Caller (function calling format) ==========

// OpenAICaller OpenAI / Azure OpenAI / Ollama 实现
type OpenAICaller struct {
	baseURL     string
	apiKey      string
	modelID     string
	serviceType string
}

// ChatWithTools 调用 OpenAI Compatible API（支持 function calling）
func (c *OpenAICaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	requestBody := c.buildOpenAIRequest(messages, tools)

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 构建 URL
	url := c.baseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return c.parseOpenAIResponse(body)
}

// buildOpenAIRequest 构建 OpenAI 格式请求体
func (c *OpenAICaller) buildOpenAIRequest(messages []AgentMessage, tools []AgentToolDef) map[string]interface{} {
	body := map[string]interface{}{
		"model":      c.modelID,
		"max_tokens": 8000,
	}

	body["temperature"] = 0.7

	// 构建 messages
	var openaiMessages []interface{}
	for _, msg := range messages {
		if msg.Role == "assistant" && len(msg.ToolCalls) > 0 {
			toolCalls := make([]interface{}, 0, len(msg.ToolCalls))
			for _, tc := range msg.ToolCalls {
				paramsJSON, _ := json.Marshal(tc.Params)
				toolCalls = append(toolCalls, map[string]interface{}{
					"id":   tc.ID,
					"type": "function",
					"function": map[string]interface{}{
						"name":      tc.Name,
						"arguments": string(paramsJSON),
					},
				})
			}
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":       "assistant",
				"content":    msg.Content,
				"tool_calls": toolCalls,
			})
		} else if msg.Role == "tool" {
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":         "tool",
				"tool_call_id": msg.ToolCallID,
				"content":      msg.Content,
			})
		} else {
			openaiMessages = append(openaiMessages, map[string]interface{}{
				"role":    msg.Role,
				"content": msg.Content,
			})
		}
	}
	body["messages"] = openaiMessages

	// 工具定义
	if len(tools) > 0 {
		var toolDefs []interface{}
		for _, t := range tools {
			schema := t.InputSchema
			if schema == nil {
				schema = map[string]interface{}{"type": "object", "properties": map[string]interface{}{}}
			}
			if _, ok := schema["type"]; !ok {
				schema = map[string]interface{}{
					"type":       "object",
					"properties": schema,
				}
			}
			toolDefs = append(toolDefs, map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":        t.Name,
					"description": t.Description,
					"parameters":  schema,
				},
			})
		}
		body["tools"] = toolDefs
	}

	return body
}

// parseOpenAIResponse 解析 OpenAI 格式响应
func (c *OpenAICaller) parseOpenAIResponse(body []byte) (*AgentAIResponse, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse openai response: %w", err)
	}

	// 记录 token 指标
	metricsType := c.serviceType
	if metricsType == "azure_openai" {
		metricsType = "openai"
	}
	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		metrics.IncAITokens(metricsType, "prompt", float64(response.Usage.PromptTokens))
		metrics.IncAITokens(metricsType, "completion", float64(response.Usage.CompletionTokens))
		log.Printf("[AICaller/OpenAI] tokens: prompt=%d, completion=%d", response.Usage.PromptTokens, response.Usage.CompletionTokens)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("empty response from API")
	}

	msg := response.Choices[0].Message
	result := &AgentAIResponse{Content: msg.Content}

	for _, tc := range msg.ToolCalls {
		var params map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				log.Printf("[AICaller/OpenAI] failed to parse tool arguments for %s: %v", tc.Function.Name, err)
			}
		}
		result.ToolCalls = append(result.ToolCalls, AgentToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: params,
		})
	}

	return result, nil
}

// ========== Qwen / DashScope Caller (OpenAI compatible + thinking) ==========

// QwenCaller Qwen/DashScope 实现，继承 OpenAICaller 并添加 thinking 支持
type QwenCaller struct {
	OpenAICaller
	thinkingEnabled      bool
	thinkingBudgetTokens int
}

// ChatWithTools 调用 Qwen API（覆写请求构建和响应解析）
func (c *QwenCaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	requestBody := c.buildQwenRequest(messages, tools)

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := c.baseURL
	if !strings.HasSuffix(url, "/") {
		url += "/"
	}
	url += "chat/completions"

	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(requestJSON))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.apiKey))
	}

	client := &http.Client{Timeout: 120 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	return c.parseQwenResponse(body)
}

// buildQwenRequest 构建 Qwen 请求体（基于 OpenAI 格式 + enable_thinking + reasoning_content 回传）
func (c *QwenCaller) buildQwenRequest(messages []AgentMessage, tools []AgentToolDef) map[string]interface{} {
	body := c.OpenAICaller.buildOpenAIRequest(messages, tools)

	if c.thinkingEnabled && c.thinkingBudgetTokens >= 1024 {
		body["enable_thinking"] = true
		delete(body, "temperature")

		// 回传 reasoning_content 到 assistant messages，保持多轮推理连贯性
		if msgs, ok := body["messages"].([]interface{}); ok {
			for i, rawMsg := range msgs {
				if msgMap, ok := rawMsg.(map[string]interface{}); ok {
					if msgMap["role"] == "assistant" && i < len(messages) {
						for _, orig := range messages {
							if orig.Role == "assistant" && orig.ThinkingContent != "" && orig.Content == msgMap["content"] {
								msgMap["reasoning_content"] = orig.ThinkingContent
								break
							}
						}
					}
				}
			}
		}
	} else {
		// 非 thinking 模式：强制 JSON 输出（减少格式错误）
		body["response_format"] = map[string]interface{}{
			"type": "json_object",
		}
	}

	return body
}

// parseQwenResponse 解析 Qwen 响应（支持 reasoning_content 字段）
func (c *QwenCaller) parseQwenResponse(body []byte) (*AgentAIResponse, error) {
	var response struct {
		Choices []struct {
			Message struct {
				Content          string `json:"content"`
				ReasoningContent string `json:"reasoning_content,omitempty"`
				ToolCalls        []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
		} `json:"usage"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse qwen response: %w", err)
	}

	if response.Usage.PromptTokens > 0 || response.Usage.CompletionTokens > 0 {
		metrics.IncAITokens("qwen", "prompt", float64(response.Usage.PromptTokens))
		metrics.IncAITokens("qwen", "completion", float64(response.Usage.CompletionTokens))
		log.Printf("[AICaller/Qwen] tokens: prompt=%d, completion=%d", response.Usage.PromptTokens, response.Usage.CompletionTokens)
	}

	if len(response.Choices) == 0 {
		return nil, fmt.Errorf("empty response from Qwen API")
	}

	msg := response.Choices[0].Message
	result := &AgentAIResponse{
		Content:         msg.Content,
		ThinkingContent: msg.ReasoningContent,
	}

	if result.ThinkingContent != "" {
		log.Printf("[AICaller/Qwen] Thinking content received (%d chars)", len(result.ThinkingContent))
	}

	for _, tc := range msg.ToolCalls {
		var params map[string]interface{}
		if tc.Function.Arguments != "" {
			if err := json.Unmarshal([]byte(tc.Function.Arguments), &params); err != nil {
				log.Printf("[AICaller/Qwen] failed to parse tool arguments for %s: %v", tc.Function.Name, err)
			}
		}
		result.ToolCalls = append(result.ToolCalls, AgentToolCall{
			ID:     tc.ID,
			Name:   tc.Function.Name,
			Params: params,
		})
	}

	return result, nil
}
