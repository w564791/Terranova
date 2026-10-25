package sandboxreg

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
	"unicode"
)

const (
	// DefaultHarnessQualifier is the AgentCore endpoint qualifier (boto3-compatible).
	DefaultHarnessQualifier = "DEFAULT"
	// MinRuntimeSessionIDLen is the AgentCore InvokeHarness minimum (API docs).
	MinRuntimeSessionIDLen = 33
	// MaxRuntimeSessionIDLen is the AgentCore InvokeHarness maximum.
	MaxRuntimeSessionIDLen = 100
	// DefaultWakeMessage is the minimal user message used to wake a harness.
	DefaultWakeMessage = "wake"
)

// sessionIDPattern matches AgentCore runtimeSessionId charset.
var sessionIDPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*$`)

// InvokeRequest is the input to HarnessInvoker.InvokeHarness.
// HarnessARN is the registered sandbox_ref (full harness ARN preferred).
// RuntimeRef is optional warm-pool metadata (not sent as invoke-agent-runtime).
type InvokeRequest struct {
	HarnessARN string
	RuntimeRef string
	SessionID  string // shell_session id; normalized to ≥33 chars
	Message    string // optional; defaults to DefaultWakeMessage
	Qualifier  string // optional; defaults to DEFAULT
}

// HarnessInvoker wakes an externally registered AgentCore harness session.
//
// Production data plane MUST call InvokeHarness (AWS SDK v2
// bedrockagentcore.InvokeHarness / boto3 bedrock-agentcore.invoke_harness).
// NEVER call invoke-agent-runtime / InvokeAgentRuntime / AWS CLI.
type HarnessInvoker interface {
	InvokeHarness(ctx context.Context, req InvokeRequest) error
}

// StubHarnessInvoker records intent without calling AWS (default in tests /
// when IAC_AGENTCORE_INVOKE_HARNESS is unset).
type StubHarnessInvoker struct {
	Calls []InvokeRequest
	Err   error // optional injected failure
}

// InvokeHarness implements HarnessInvoker (stub).
func (s *StubHarnessInvoker) InvokeHarness(ctx context.Context, req InvokeRequest) error {
	if s == nil {
		return nil
	}
	s.Calls = append(s.Calls, req)
	return s.Err
}

// NewStubHarnessInvoker returns a no-op recorder.
func NewStubHarnessInvoker() *StubHarnessInvoker {
	return &StubHarnessInvoker{}
}

// RuntimeSessionID derives a stable AgentCore runtimeSessionId (≥33, ≤100)
// from a shell_session id. UUID-with-hyphens (36) already qualifies; shorter
// ids are padded with a hex hash suffix. Invalid characters are replaced.
func RuntimeSessionID(shellSessionID string) string {
	id := strings.TrimSpace(shellSessionID)
	if id == "" {
		id = "shell"
	}
	var b strings.Builder
	b.Grow(len(id))
	for i, r := range id {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r) || r == '-' || r == '_':
			if i == 0 && !(unicode.IsLetter(r) || unicode.IsDigit(r)) {
				b.WriteByte('s')
			}
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	id = b.String()
	if id == "" || !(unicode.IsLetter(rune(id[0])) || unicode.IsDigit(rune(id[0]))) {
		id = "s" + id
	}
	if len(id) >= MinRuntimeSessionIDLen {
		if len(id) > MaxRuntimeSessionIDLen {
			return id[:MaxRuntimeSessionIDLen]
		}
		return id
	}
	sum := sha256.Sum256([]byte(shellSessionID))
	pad := hex.EncodeToString(sum[:])
	out := id + "-" + pad
	if len(out) > MaxRuntimeSessionIDLen {
		out = out[:MaxRuntimeSessionIDLen]
	}
	for len(out) < MinRuntimeSessionIDLen {
		out += "0"
	}
	return out
}

// ValidateInvokeRequest checks harness ARN and session id shape before AWS call.
func ValidateInvokeRequest(req InvokeRequest) error {
	if strings.TrimSpace(req.HarnessARN) == "" {
		return fmt.Errorf("harness ARN is required")
	}
	if err := ValidateSandboxRef(req.HarnessARN); err != nil {
		return err
	}
	if strings.Contains(req.HarnessARN, "://") {
		return fmt.Errorf("harness ARN must not be a URL")
	}
	sid := RuntimeSessionID(req.SessionID)
	if len(sid) < MinRuntimeSessionIDLen || len(sid) > MaxRuntimeSessionIDLen {
		return fmt.Errorf("runtimeSessionId length %d out of range", len(sid))
	}
	if !sessionIDPattern.MatchString(sid) {
		return fmt.Errorf("runtimeSessionId has invalid characters")
	}
	return nil
}
