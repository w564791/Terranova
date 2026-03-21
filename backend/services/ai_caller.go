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
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockruntime"
)

// ========== AICaller Factory ==========

// NewAICallerFromConfig 根据 AIConfig 创建对应的 AICaller
func NewAICallerFromConfig(cfg *models.AIConfig) AICaller {
	switch cfg.ServiceType {
	case "bedrock":
		return &BedrockCaller{
			region:              cfg.AWSRegion,
			modelID:             cfg.ModelID,
			useInferenceProfile: cfg.UseInferenceProfile,
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
			region:  cfg.AWSRegion,
			modelID: cfg.ModelID,
		}
	}
}

// ========== Bedrock Caller (Claude tool_use format) ==========

// BedrockCaller Bedrock Claude tool calling 实现
type BedrockCaller struct {
	region              string
	modelID             string
	useInferenceProfile bool
}

// ChatWithTools 调用 Bedrock Claude（支持 tool_use）
func (c *BedrockCaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(c.region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	awsCfg.RetryMaxAttempts = 1
	client := bedrockruntime.NewFromConfig(awsCfg)

	// 构建请求体
	requestBody := c.buildBedrockRequest(messages, tools)

	requestJSON, err := json.Marshal(requestBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// 确定 model ID
	finalModelID := c.resolveModelID()

	input := &bedrockruntime.InvokeModelInput{
		ModelId:     aws.String(finalModelID),
		ContentType: aws.String("application/json"),
		Body:        requestJSON,
	}

	output, err := client.InvokeModel(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("bedrock invocation failed: %w", err)
	}

	return c.parseBedrockResponse(output.Body)
}

// buildBedrockRequest 构建 Bedrock Claude 请求体（支持 tool_use）
func (c *BedrockCaller) buildBedrockRequest(messages []AgentMessage, tools []AgentToolDef) map[string]interface{} {
	body := map[string]interface{}{
		"anthropic_version": "bedrock-2023-05-31",
		"max_tokens":        8000,
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
			// assistant 消息带 tool_use
			content := make([]interface{}, 0)
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
		body["system"] = systemPrompt
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

// resolveModelID 处理 inference profile
func (c *BedrockCaller) resolveModelID() string {
	if !c.useInferenceProfile {
		return c.modelID
	}
	switch {
	case c.region == "us-east-1" || c.region == "us-west-2":
		return fmt.Sprintf("us.%s", c.modelID)
	case c.region == "eu-west-1" || c.region == "eu-central-1":
		return fmt.Sprintf("eu.%s", c.modelID)
	case c.region == "ap-southeast-1" || c.region == "ap-northeast-1":
		return fmt.Sprintf("apac.%s", c.modelID)
	default:
		return c.modelID
	}
}

// parseBedrockResponse 解析 Bedrock 响应
func (c *BedrockCaller) parseBedrockResponse(body []byte) (*AgentAIResponse, error) {
	var response struct {
		Content []struct {
			Type  string          `json:"type"`
			Text  string          `json:"text,omitempty"`
			ID    string          `json:"id,omitempty"`
			Name  string          `json:"name,omitempty"`
			Input json.RawMessage `json:"input,omitempty"`
		} `json:"content"`
		Usage struct {
			InputTokens  int `json:"input_tokens"`
			OutputTokens int `json:"output_tokens"`
		} `json:"usage"`
		StopReason string `json:"stop_reason"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to parse bedrock response: %w", err)
	}

	// 记录 token 指标
	if response.Usage.InputTokens > 0 || response.Usage.OutputTokens > 0 {
		metrics.IncAITokens("bedrock", "prompt", float64(response.Usage.InputTokens))
		metrics.IncAITokens("bedrock", "completion", float64(response.Usage.OutputTokens))
		log.Printf("[AICaller/Bedrock] tokens: input=%d, output=%d", response.Usage.InputTokens, response.Usage.OutputTokens)
	}

	result := &AgentAIResponse{}
	var textParts []string

	for _, block := range response.Content {
		switch block.Type {
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
		"model":       c.modelID,
		"max_tokens":  8000,
		"temperature": 0.7,
	}

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
