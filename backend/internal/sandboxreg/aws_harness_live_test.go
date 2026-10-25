//go:build agentcore_live

package sandboxreg

import (
	"context"
	"os"
	"testing"
	"time"
)

// Live InvokeHarness against the 联调 harness. Enable with:
//
//	IAC_AGENTCORE_INVOKE_HARNESS=1 AWS_REGION=ap-northeast-1 \
//	  go test -tags agentcore_live ./internal/sandboxreg/ -run Live -count=1
//
// Uses the default AWS SDK credential chain only. Never prints secrets.
func TestLiveInvokeHarness(t *testing.T) {
	if os.Getenv(EnvInvokeHarness) != "1" {
		t.Skip("set IAC_AGENTCORE_INVOKE_HARNESS=1 for live call")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	inv, err := NewAWSHarnessInvoker(ctx)
	if err != nil {
		t.Fatalf("client: %v", err)
	}
	err = inv.InvokeHarness(ctx, InvokeRequest{
		HarnessARN: FixtureHarnessARN,
		RuntimeRef: FixtureRuntimeARN,
		SessionID:  "live-test-550e8400-e29b-41d4-a716-446655440000",
		Message:    "wake",
	})
	if err != nil {
		t.Fatalf("InvokeHarness: %v", err)
	}
}
