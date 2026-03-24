package services

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

// ========== AI Agent Loop Framework ==========
// 通用 AI Agent 循环框架，类似 n8n AI Agent 节点
// 框架只负责：注册工具 → 转发 AI 的 tool call → 把结果喂回 AI
// AI 完全控制流程走向

// AgentTool AI 可调用的工具接口
type AgentTool interface {
	Name() string
	Description() string
	InputSchema() map[string]interface{}
	Execute(ctx context.Context, params map[string]interface{}) (interface{}, error)
}

// AICaller AI 调用抽象，屏蔽 bedrock/openai/ollama 差异
type AICaller interface {
	ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error)
}

// AgentMessage 对话消息
type AgentMessage struct {
	Role       string          `json:"role"`                  // system / user / assistant / tool
	Content    string          `json:"content"`               // 文本内容
	ToolCalls  []AgentToolCall `json:"tool_calls,omitempty"`  // assistant 角色的工具调用
	ToolCallID string          `json:"tool_call_id,omitempty"` // tool 角色时对应的 tool call ID
}

// AgentToolCall AI 发起的工具调用
type AgentToolCall struct {
	ID     string                 `json:"id"`
	Name   string                 `json:"name"`
	Params map[string]interface{} `json:"params"`
}

// AgentAIResponse AI 响应
type AgentAIResponse struct {
	Content   string          `json:"content"`              // 纯文本（AI 决定结束时）
	ToolCalls []AgentToolCall `json:"tool_calls,omitempty"` // 工具调用（AI 想继续时）
}

// AgentToolDef 传给 AI 的工具定义
type AgentToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// AgentToolCallRecord 工具调用审计记录
type AgentToolCallRecord struct {
	ToolName   string                 `json:"tool_name"`
	Params     map[string]interface{} `json:"params"`
	Result     interface{}            `json:"result,omitempty"`
	Error      string                 `json:"error,omitempty"`
	DurationMs int64                  `json:"duration_ms"`
}

// AgentLoopResult 循环结果
type AgentLoopResult struct {
	FinalOutput string                `json:"final_output"`
	ToolCalls   []AgentToolCallRecord `json:"tool_calls"`
	TotalSteps  int                   `json:"total_steps"`
	Completed   bool                  `json:"completed"` // true=AI 主动结束, false=达到上限
}

// AIAgentLoop 通用 AI Agent 循环
// OutputValidator 输出验证器 — 验证 AI 最终输出是否符合预期
// 返回 nil 表示通过，返回 error 表示需要 AI 重试（error message 会反馈给 AI）
type OutputValidator func(output string) error

// AIAgentLoop 通用 AI Agent 循环
type AIAgentLoop struct {
	aiCaller        AICaller
	tools           map[string]AgentTool
	maxIterations   int
	outputValidator OutputValidator
	maxRetries      int // 输出验证失败时的最大重试次数
}

// NewAIAgentLoop 创建 Agent Loop
func NewAIAgentLoop(caller AICaller, maxIterations int) *AIAgentLoop {
	if maxIterations <= 0 {
		maxIterations = 10
	}
	return &AIAgentLoop{
		aiCaller:      caller,
		tools:         make(map[string]AgentTool),
		maxIterations: maxIterations,
		maxRetries:    2, // 默认最多重试 2 次
	}
}

// SetOutputValidator 设置输出验证器
func (loop *AIAgentLoop) SetOutputValidator(validator OutputValidator) {
	loop.outputValidator = validator
}

// SetMaxRetries 设置输出验证失败时的最大重试次数
func (loop *AIAgentLoop) SetMaxRetries(n int) {
	if n > 0 && n <= 5 {
		loop.maxRetries = n
	}
}

// RegisterTool 注册工具
func (loop *AIAgentLoop) RegisterTool(tool AgentTool) {
	loop.tools[tool.Name()] = tool
}

// Run 核心循环 — AI 完全控制流程
func (loop *AIAgentLoop) Run(ctx context.Context, systemPrompt, userPrompt string) (*AgentLoopResult, error) {
	messages := []AgentMessage{
		{Role: "system", Content: systemPrompt},
		{Role: "user", Content: userPrompt},
	}

	toolDefs := loop.buildToolDefs()
	var allToolCalls []AgentToolCallRecord
	retryCount := 0

	for i := 0; i < loop.maxIterations; i++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		response, err := loop.aiCaller.ChatWithTools(ctx, messages, toolDefs)
		if err != nil {
			return nil, fmt.Errorf("AI call failed at step %d: %w", i+1, err)
		}

		// AI 返回纯文本 = 决定结束
		if len(response.ToolCalls) == 0 {
			// 如果设置了输出验证器，验证输出格式
			if loop.outputValidator != nil {
				if validationErr := loop.outputValidator(response.Content); validationErr != nil {
					retryCount++
					if retryCount <= loop.maxRetries {
						log.Printf("[AIAgentLoop] Output validation failed (retry %d/%d): %v", retryCount, loop.maxRetries, validationErr)
						// 把验证错误反馈给 AI，让它修正
						messages = append(messages, AgentMessage{
							Role:    "assistant",
							Content: response.Content,
						})
						messages = append(messages, AgentMessage{
							Role:    "user",
							Content: fmt.Sprintf("你的输出格式不正确: %s\n请严格按照要求的 JSON 格式重新输出。", validationErr.Error()),
						})
						continue
					}
					log.Printf("[AIAgentLoop] Output validation still failing after %d retries, accepting as-is", loop.maxRetries)
				}
			}

			return &AgentLoopResult{
				FinalOutput: response.Content,
				ToolCalls:   allToolCalls,
				TotalSteps:  i + 1,
				Completed:   true,
			}, nil
		}

		// AI 要调用工具
		messages = append(messages, AgentMessage{
			Role:      "assistant",
			Content:   response.Content,
			ToolCalls: response.ToolCalls,
		})

		// 并发执行所有 tool calls
		type toolResult struct {
			index   int
			record  AgentToolCallRecord
			message AgentMessage
		}

		results := make([]toolResult, len(response.ToolCalls))
		var wg sync.WaitGroup

		for idx, tc := range response.ToolCalls {
			wg.Add(1)
			go func(i int, tc AgentToolCall) {
				defer wg.Done()

				record := AgentToolCallRecord{
					ToolName: tc.Name,
					Params:   tc.Params,
				}

				startTime := time.Now()

				tool, exists := loop.tools[tc.Name]
				if !exists {
					record.Error = fmt.Sprintf("tool not found: %s", tc.Name)
					record.DurationMs = time.Since(startTime).Milliseconds()
					results[i] = toolResult{
						index:  i,
						record: record,
						message: AgentMessage{
							Role:       "tool",
							ToolCallID: tc.ID,
							Content:    fmt.Sprintf("Error: tool '%s' not found", tc.Name),
						},
					}
					return
				}

				result, execErr := tool.Execute(ctx, tc.Params)
				record.DurationMs = time.Since(startTime).Milliseconds()

				if execErr != nil {
					record.Error = execErr.Error()
					results[i] = toolResult{
						index:  i,
						record: record,
						message: AgentMessage{
							Role:       "tool",
							ToolCallID: tc.ID,
							Content:    fmt.Sprintf("Error: %s", execErr.Error()),
						},
					}
					return
				}

				record.Result = result
				resultStr := loop.serializeResult(result)
				results[i] = toolResult{
					index:  i,
					record: record,
					message: AgentMessage{
						Role:       "tool",
						ToolCallID: tc.ID,
						Content:    resultStr,
					},
				}
			}(idx, tc)
		}

		wg.Wait()

		// 按原始顺序追加结果
		for _, r := range results {
			allToolCalls = append(allToolCalls, r.record)
			messages = append(messages, r.message)
		}
	}

	// 达到最大迭代次数 — 强制让 AI 总结
	log.Printf("[AIAgentLoop] 达到最大迭代次数 %d，强制生成最终输出", loop.maxIterations)
	messages = append(messages, AgentMessage{
		Role:    "user",
		Content: "已达到最大迭代次数，请基于当前已有信息生成最终结果。",
	})

	finalResponse, err := loop.aiCaller.ChatWithTools(ctx, messages, nil)
	if err != nil {
		return nil, fmt.Errorf("final AI call failed: %w", err)
	}

	return &AgentLoopResult{
		FinalOutput: finalResponse.Content,
		ToolCalls:   allToolCalls,
		TotalSteps:  loop.maxIterations,
		Completed:   false,
	}, nil
}

// buildToolDefs 从注册的工具构建定义列表
func (loop *AIAgentLoop) buildToolDefs() []AgentToolDef {
	defs := make([]AgentToolDef, 0, len(loop.tools))
	for _, tool := range loop.tools {
		defs = append(defs, AgentToolDef{
			Name:        tool.Name(),
			Description: tool.Description(),
			InputSchema: tool.InputSchema(),
		})
	}
	return defs
}

// serializeResult 序列化工具结果，超长截断
func (loop *AIAgentLoop) serializeResult(result interface{}) string {
	const maxLen = 10000

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Sprintf("%v", result)
	}

	str := string(data)
	if len(str) > maxLen {
		return str[:maxLen] + "\n[... truncated, showing first 10000 chars ...]"
	}
	return str
}
