package sandboxreg

// HarnessInvoker wakes an externally registered AgentCore session.
//
// Production data plane MUST call InvokeHarness (Bedrock AgentCore Harness
// invoke / SDK equivalent). NEVER call invoke-agent-runtime / InvokeAgentRuntime.
//
// TODO(step-6): wire AWS SDK InvokeHarness once platform AgentCore vault
// credentials are available. Live AWS calls are skipped in unit tests; source
// ~/.aws/credentials.temp only for optional 联调 (may be absent on the box).
type HarnessInvoker interface {
	// InvokeHarness wakes the harness for sandboxRef / runtimeRef.
	// sessionID isolates workdir and credentials for this session only.
	InvokeHarness(sandboxRef, runtimeRef, sessionID string) error
}

// StubHarnessInvoker records intent without calling AWS.
type StubHarnessInvoker struct {
	Calls []HarnessCall
}

// HarnessCall is one recorded InvokeHarness attempt.
type HarnessCall struct {
	SandboxRef string
	RuntimeRef string
	SessionID  string
}

// InvokeHarness implements HarnessInvoker (stub).
func (s *StubHarnessInvoker) InvokeHarness(sandboxRef, runtimeRef, sessionID string) error {
	if s == nil {
		return nil
	}
	s.Calls = append(s.Calls, HarnessCall{
		SandboxRef: sandboxRef,
		RuntimeRef: runtimeRef,
		SessionID:  sessionID,
	})
	return nil
}

// NewStubHarnessInvoker returns a no-op recorder for tests and registration skeleton.
func NewStubHarnessInvoker() *StubHarnessInvoker {
	return &StubHarnessInvoker{}
}
