package sandboxreg

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentcore"
	"github.com/aws/aws-sdk-go-v2/service/bedrockagentcore/types"
)

// EnvInvokeHarness enables the live AWS InvokeHarness client when set to "1"
// (or "true"/"yes"). Default remains StubHarnessInvoker so unit tests and
// deploys without AWS credentials keep working.
const EnvInvokeHarness = "IAC_AGENTCORE_INVOKE_HARNESS"

// invokeHarnessAPI is the subset of bedrockagentcore.Client used for wake.
// NEVER add InvokeAgentRuntime here.
type invokeHarnessAPI interface {
	InvokeHarness(ctx context.Context, params *bedrockagentcore.InvokeHarnessInput, optFns ...func(*bedrockagentcore.Options)) (*bedrockagentcore.InvokeHarnessOutput, error)
}

// AWSHarnessInvoker calls bedrockagentcore.InvokeHarness via the default AWS
// credential chain (env / shared config / IAM role). It does not shell out to
// the AWS CLI and never calls InvokeAgentRuntime.
//
// Live 联调: export IAC_AGENTCORE_INVOKE_HARNESS=1 and configure the SDK chain
// (e.g. AWS_REGION=ap-northeast-1 and credentials). Do not log secrets.
type AWSHarnessInvoker struct {
	client invokeHarnessAPI
}

// NewAWSHarnessInvoker builds a client with config.LoadDefaultConfig.
func NewAWSHarnessInvoker(ctx context.Context, optFns ...func(*config.LoadOptions) error) (*AWSHarnessInvoker, error) {
	cfg, err := config.LoadDefaultConfig(ctx, optFns...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config for InvokeHarness: %w", err)
	}
	return &AWSHarnessInvoker{client: bedrockagentcore.NewFromConfig(cfg)}, nil
}

// NewAWSHarnessInvokerWithClient injects a mock or prebuilt client (tests).
func NewAWSHarnessInvokerWithClient(client invokeHarnessAPI) *AWSHarnessInvoker {
	return &AWSHarnessInvoker{client: client}
}

// InvokeHarness implements HarnessInvoker using the official SDK operation
// InvokeHarness only (boto3-compatible: harnessArn + runtimeSessionId +
// messages + qualifier=DEFAULT → streaming response).
func (a *AWSHarnessInvoker) InvokeHarness(ctx context.Context, req InvokeRequest) error {
	if a == nil || a.client == nil {
		return fmt.Errorf("InvokeHarness client not configured")
	}
	if err := ValidateInvokeRequest(req); err != nil {
		return err
	}
	msg := strings.TrimSpace(req.Message)
	if msg == "" {
		msg = DefaultWakeMessage
	}
	qual := strings.TrimSpace(req.Qualifier)
	if qual == "" {
		qual = DefaultHarnessQualifier
	}
	sid := RuntimeSessionID(req.SessionID)
	in := &bedrockagentcore.InvokeHarnessInput{
		HarnessArn:       aws.String(req.HarnessARN),
		RuntimeSessionId: aws.String(sid),
		Qualifier:        aws.String(qual),
		Messages: []types.HarnessMessage{{
			Role: types.HarnessConversationRoleUser,
			Content: []types.HarnessContentBlock{
				&types.HarnessContentBlockMemberText{Value: msg},
			},
		}},
	}
	// runtimeRef is warm-pool metadata only; InvokeHarness takes harness ARN.
	// Do NOT call InvokeAgentRuntime with runtimeRef.
	_ = req.RuntimeRef

	out, err := a.client.InvokeHarness(ctx, in)
	if err != nil {
		// Error strings from AWS SDK must not be logged with credentials;
		// return as-is (SDK errors do not embed AK/SK).
		return fmt.Errorf("InvokeHarness: %w", err)
	}
	if out == nil {
		return fmt.Errorf("InvokeHarness: empty response")
	}
	stream := out.GetStream()
	if stream == nil {
		return nil
	}
	defer stream.Close()
	// Drain until the stream ends; wake succeeds on HTTP 200 + readable stream.
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case ev, ok := <-stream.Events():
			if !ok {
				if err := stream.Err(); err != nil && err != io.EOF {
					return fmt.Errorf("InvokeHarness stream: %w", err)
				}
				return nil
			}
			_ = ev
		}
	}
}

// InvokeHarnessTimeout bounds InvokeHarness + event-stream drain on the
// shell-create handler path. Matches the live test budget in
// aws_harness_live_test.go (60s). Do not rely solely on an unbounded request
// context while draining the stream.
const InvokeHarnessTimeout = 60 * time.Second

// FailingHarnessInvoker always returns Err from InvokeHarness.
// Used when IAC_AGENTCORE_INVOKE_HARNESS=1 but AWS config / client construction
// failed, so operators get 503 harness_invoke_failed instead of a silent stub
// that never called AWS.
type FailingHarnessInvoker struct {
	Err error
}

// InvokeHarness implements HarnessInvoker (always errors).
func (f *FailingHarnessInvoker) InvokeHarness(ctx context.Context, req InvokeRequest) error {
	if f == nil || f.Err == nil {
		return fmt.Errorf("InvokeHarness: AWS client not configured")
	}
	return f.Err
}

// harnessAWSFactory builds the live AWS invoker. Overridden in unit tests to
// simulate LoadDefaultConfig / client construction failure without touching
// real credentials.
var harnessAWSFactory = func(ctx context.Context) (HarnessInvoker, error) {
	return NewAWSHarnessInvoker(ctx)
}

// HarnessInvokerFromEnv returns AWSHarnessInvoker when EnvInvokeHarness is set,
// otherwise StubHarnessInvoker.
//
// When the live flag is set and AWS config/client construction fails, this
// NEVER falls back to StubHarnessInvoker. It returns a FailingHarnessInvoker
// (every InvokeHarness errors) plus the construction error so callers may fail
// startup loudly or keep serving 503 on shell-create paths.
func HarnessInvokerFromEnv(ctx context.Context) (HarnessInvoker, error) {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(EnvInvokeHarness)))
	if v != "1" && v != "true" && v != "yes" {
		return NewStubHarnessInvoker(), nil
	}
	inv, err := harnessAWSFactory(ctx)
	if err != nil {
		fail := &FailingHarnessInvoker{Err: fmt.Errorf("AWS config for InvokeHarness: %w", err)}
		return fail, err
	}
	return inv, nil
}
