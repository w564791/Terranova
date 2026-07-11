package services

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"iac-platform/internal/models"
)

func TestIsAIAccessError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"bedrock invocation", fmt.Errorf("bedrock invocation failed: AccessDeniedException"), true},
		{"调用 bedrock 失败", fmt.Errorf("调用 Bedrock 失败: ValidationException"), true},
		{"timeout", fmt.Errorf("context deadline exceeded"), true},
		{"401", fmt.Errorf("API 返回错误状态码 401: unauthorized"), true},
		{"model not found", fmt.Errorf("model not found: claude-opus-4-8"), true},
		{"output validation", fmt.Errorf("output validation failed: missing field"), false},
		{"输出格式", fmt.Errorf("输出格式不正确: 缺少 execution_summary"), false},
		{"generic business", fmt.Errorf("AI 未返回有效的 HCL 内容"), false},
		{"wrapped access", fmt.Errorf("AI call failed at step 1: %w", errors.New("bedrock invocation failed: throttle")), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsAIAccessError(tc.err)
			if got != tc.want {
				t.Fatalf("IsAIAccessError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsDefaultAIConfig(t *testing.T) {
	if isDefaultAIConfig(nil) {
		t.Fatal("nil should not be default")
	}
	if isDefaultAIConfig(&models.AIConfig{Enabled: false, Capabilities: models.StringArray{"*"}}) {
		t.Fatal("enabled=false should not be default even with *")
	}
	if !isDefaultAIConfig(&models.AIConfig{Enabled: true, Capabilities: models.StringArray{"*"}}) {
		t.Fatal("enabled=true + * should be default")
	}
	if isDefaultAIConfig(&models.AIConfig{Enabled: true, Capabilities: models.StringArray{"summary"}}) {
		t.Fatal("enabled without * should not be default")
	}
}

type seqCaller struct {
	responses []*AgentAIResponse
	errors    []error
	calls     int
}

func (c *seqCaller) ChatWithTools(ctx context.Context, messages []AgentMessage, tools []AgentToolDef) (*AgentAIResponse, error) {
	i := c.calls
	c.calls++
	if i < len(c.errors) && c.errors[i] != nil {
		return nil, c.errors[i]
	}
	if i < len(c.responses) {
		return c.responses[i], nil
	}
	return &AgentAIResponse{Content: "ok"}, nil
}

func TestFallbackAICaller_RetriesOnAccessError(t *testing.T) {
	primary := &seqCaller{errors: []error{fmt.Errorf("bedrock invocation failed: AccessDenied")}}
	fallback := &seqCaller{responses: []*AgentAIResponse{{Content: "from-default"}}}

	caller := &FallbackAICaller{
		primary:       primary,
		fallback:      fallback,
		capability:    "summary",
		primaryID:     16,
		primaryModel:  "opus",
		fallbackID:    6,
		fallbackModel: "sonnet",
	}

	resp, err := caller.ChatWithTools(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content != "from-default" {
		t.Fatalf("want from-default, got %q", resp.Content)
	}
	if primary.calls != 1 || fallback.calls != 1 {
		t.Fatalf("calls primary=%d fallback=%d", primary.calls, fallback.calls)
	}
}

func TestFallbackAICaller_NoRetryOnBusinessError(t *testing.T) {
	primary := &seqCaller{errors: []error{fmt.Errorf("输出格式不正确: 缺少字段")}}
	fallback := &seqCaller{responses: []*AgentAIResponse{{Content: "should-not-use"}}}

	caller := &FallbackAICaller{
		primary:  primary,
		fallback: fallback,
	}

	_, err := caller.ChatWithTools(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected business error to surface")
	}
	if fallback.calls != 0 {
		t.Fatalf("fallback should not be called, calls=%d", fallback.calls)
	}
}

func TestFallbackAICaller_StickyAfterFallback(t *testing.T) {
	// 第一次 primary 访问失败 → fallback；第二次应直接走 fallback，不再碰 primary
	primary := &seqCaller{errors: []error{fmt.Errorf("bedrock invocation failed: AccessDenied")}}
	fallback := &seqCaller{responses: []*AgentAIResponse{
		{Content: "fb-1"},
		{Content: "fb-2"},
	}}

	caller := &FallbackAICaller{
		primary:  primary,
		fallback: fallback,
	}

	r1, err := caller.ChatWithTools(context.Background(), nil, nil)
	if err != nil || r1.Content != "fb-1" {
		t.Fatalf("first call: resp=%v err=%v", r1, err)
	}
	r2, err := caller.ChatWithTools(context.Background(), nil, nil)
	if err != nil || r2.Content != "fb-2" {
		t.Fatalf("second call: resp=%v err=%v", r2, err)
	}
	if primary.calls != 1 {
		t.Fatalf("primary should only be tried once, calls=%d", primary.calls)
	}
	if fallback.calls != 2 {
		t.Fatalf("fallback calls=%d want 2", fallback.calls)
	}
}

func TestResolveProviderFallback_SkipsEmbedding(t *testing.T) {
	s := &AIConfigService{}
	primary := &models.AIConfig{ID: 11, Enabled: false, Capabilities: models.StringArray{"embedding"}}
	fb, ok := s.ResolveProviderFallback("embedding", primary, fmt.Errorf("bedrock invocation failed"))
	if ok || fb != nil {
		t.Fatal("embedding must not fallback")
	}
}
