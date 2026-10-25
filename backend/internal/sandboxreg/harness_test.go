package sandboxreg

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/bedrockagentcore"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentcore/types"
)

func TestRuntimeSessionID_MinLength(t *testing.T) {
	short := "abc"
	got := RuntimeSessionID(short)
	if len(got) < MinRuntimeSessionIDLen {
		t.Fatalf("len=%d got=%q", len(got), got)
	}
	uuid := "550e8400-e29b-41d4-a716-446655440000" // 36
	got = RuntimeSessionID(uuid)
	if got != uuid {
		t.Fatalf("uuid should pass through, got %q", got)
	}
	if len(RuntimeSessionID("")) < MinRuntimeSessionIDLen {
		t.Fatal("empty should pad")
	}
}

func TestStubHarnessInvoker_Records(t *testing.T) {
	s := NewStubHarnessInvoker()
	req := InvokeRequest{HarnessARN: FixtureHarnessARN, SessionID: "550e8400-e29b-41d4-a716-446655440000"}
	if err := s.InvokeHarness(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	if len(s.Calls) != 1 || s.Calls[0].HarnessARN != FixtureHarnessARN {
		t.Fatalf("%+v", s.Calls)
	}
}

type mockHarnessAPI struct {
	last *bedrockagentcore.InvokeHarnessInput
	err  error
}

func (m *mockHarnessAPI) InvokeHarness(ctx context.Context, params *bedrockagentcore.InvokeHarnessInput, optFns ...func(*bedrockagentcore.Options)) (*bedrockagentcore.InvokeHarnessOutput, error) {
	m.last = params
	if m.err != nil {
		return nil, m.err
	}
	// nil EventStream → invoker treats HTTP 200 with empty stream as success
	return &bedrockagentcore.InvokeHarnessOutput{}, nil
}

func TestAWSHarnessInvoker_BuildsInvokeHarnessInput(t *testing.T) {
	mock := &mockHarnessAPI{}
	inv := NewAWSHarnessInvokerWithClient(mock)
	sess := "550e8400-e29b-41d4-a716-446655440000"
	err := inv.InvokeHarness(context.Background(), InvokeRequest{
		HarnessARN: FixtureHarnessARN,
		RuntimeRef: FixtureRuntimeARN,
		SessionID:  sess,
	})
	if err != nil {
		t.Fatal(err)
	}
	if mock.last == nil {
		t.Fatal("no call")
	}
	if mock.last.HarnessArn == nil || *mock.last.HarnessArn != FixtureHarnessARN {
		t.Fatalf("harness=%v", mock.last.HarnessArn)
	}
	if mock.last.RuntimeSessionId == nil || *mock.last.RuntimeSessionId != sess {
		t.Fatalf("session=%v", mock.last.RuntimeSessionId)
	}
	if mock.last.Qualifier == nil || *mock.last.Qualifier != DefaultHarnessQualifier {
		t.Fatalf("qualifier=%v", mock.last.Qualifier)
	}
	if len(mock.last.Messages) != 1 || mock.last.Messages[0].Role != types.HarnessConversationRoleUser {
		t.Fatalf("messages=%+v", mock.last.Messages)
	}
	txt, ok := mock.last.Messages[0].Content[0].(*types.HarnessContentBlockMemberText)
	if !ok || txt.Value != DefaultWakeMessage {
		t.Fatalf("content=%T %+v", mock.last.Messages[0].Content[0], mock.last.Messages[0].Content[0])
	}
}

func TestAWSHarnessInvoker_PropagatesError(t *testing.T) {
	mock := &mockHarnessAPI{err: errors.New("boom")}
	inv := NewAWSHarnessInvokerWithClient(mock)
	err := inv.InvokeHarness(context.Background(), InvokeRequest{
		HarnessARN: FixtureHarnessARN,
		SessionID:  "550e8400-e29b-41d4-a716-446655440000",
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("got %v", err)
	}
}

func TestAWSHarnessInvoker_RejectsBadARN(t *testing.T) {
	inv := NewAWSHarnessInvokerWithClient(&mockHarnessAPI{})
	err := inv.InvokeHarness(context.Background(), InvokeRequest{
		HarnessARN: "https://evil.example/h",
		SessionID:  "550e8400-e29b-41d4-a716-446655440000",
	})
	if err == nil {
		t.Fatal("expected reject")
	}
}

func TestHarnessInvokerFromEnv_DefaultStub(t *testing.T) {
	t.Setenv(EnvInvokeHarness, "")
	inv, err := HarnessInvokerFromEnv(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := inv.(*StubHarnessInvoker); !ok {
		t.Fatalf("got %T", inv)
	}
}

func TestSourceHasNoInvokeAgentRuntimeCall(t *testing.T) {
	b, err := os.ReadFile("aws_harness.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if strings.Contains(src, "InvokeAgentRuntime(") || strings.Contains(src, ".InvokeAgentRuntime") {
		t.Fatal("aws_harness.go must not call InvokeAgentRuntime")
	}
	if !strings.Contains(src, "InvokeHarness") {
		t.Fatal("expected InvokeHarness")
	}
}

func TestHarnessInvokerFromEnv_FlagSetConfigFail_NotStub(t *testing.T) {
	t.Setenv(EnvInvokeHarness, "1")
	orig := harnessAWSFactory
	t.Cleanup(func() { harnessAWSFactory = orig })
	harnessAWSFactory = func(ctx context.Context) (HarnessInvoker, error) {
		return nil, errors.New("simulated LoadDefaultConfig failure")
	}
	inv, err := HarnessInvokerFromEnv(context.Background())
	if err == nil {
		t.Fatal("expected construction error")
	}
	if inv == nil {
		t.Fatal("expected failing invoker, got nil")
	}
	if _, ok := inv.(*StubHarnessInvoker); ok {
		t.Fatal("must not return StubHarnessInvoker when flag=1 and config fails")
	}
	fail, ok := inv.(*FailingHarnessInvoker)
	if !ok {
		t.Fatalf("got %T, want *FailingHarnessInvoker", inv)
	}
	callErr := fail.InvokeHarness(context.Background(), InvokeRequest{
		HarnessARN: FixtureHarnessARN,
		SessionID:  "550e8400-e29b-41d4-a716-446655440000",
	})
	if callErr == nil {
		t.Fatal("FailingHarnessInvoker must error on InvokeHarness")
	}
	if !strings.Contains(callErr.Error(), "simulated LoadDefaultConfig failure") {
		t.Fatalf("got %v", callErr)
	}
}

func TestInvokeHarnessTimeout_Documented(t *testing.T) {
	if InvokeHarnessTimeout != 60*time.Second {
		t.Fatalf("InvokeHarnessTimeout=%v, want 60s (match live tests)", InvokeHarnessTimeout)
	}
}
