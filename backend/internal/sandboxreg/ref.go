// Package sandboxreg validates and registers externally managed sandboxes
// (AgentCore harness/runtime). The platform only registers refs; it does not
// create or destroy AgentCore containers (docs/manifest/manifest-sandbox-spec.md §10.2).
package sandboxreg

import (
	"fmt"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	// MaxRefLen is the max length of sandbox_ref / runtime_ref (AWS ARNs fit).
	MaxRefLen = 256
)

// RefPattern allows short ids and full AWS ARNs (including '/').
// Rejects URLs via a separate :// check — not via this charset alone.
var RefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{1,255}$`)

const (
	// ErrInvalidSandboxRef is returned for malformed sandbox_ref / runtime_ref.
	ErrInvalidSandboxRef = "invalid_sandbox_ref"
	// ErrInvalidRuntimeRef is the same shape check for runtime_ref.
	ErrInvalidRuntimeRef = "invalid_runtime_ref"
)

// ValidationError is a stable API error with a machine code.
type ValidationError struct {
	Code    string
	Message string
}

func (e *ValidationError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return e.Code
}

// ValidateRef checks sandbox_ref / runtime_ref format.
// Rejects empty, :// (URL/SSRF), and charset/length violations.
func ValidateRef(ref string, kind string) error {
	code := ErrInvalidSandboxRef
	if kind == "runtime" {
		code = ErrInvalidRuntimeRef
	}
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return &ValidationError{Code: code, Message: kind + " ref is required"}
	}
	// Reject URLs / schemes with ://; bare ':' and '/' are allowed (ARN paths).
	if strings.Contains(ref, "://") {
		return &ValidationError{Code: code, Message: kind + " ref must not be a URL"}
	}
	if utf8.RuneCountInString(ref) > MaxRefLen {
		return &ValidationError{Code: code, Message: fmt.Sprintf("%s ref exceeds %d characters", kind, MaxRefLen)}
	}
	if !RefPattern.MatchString(ref) {
		return &ValidationError{Code: code, Message: fmt.Sprintf("%s ref must match %s", kind, RefPattern.String())}
	}
	return nil
}

// ValidateSandboxRef validates a required sandbox_ref.
func ValidateSandboxRef(ref string) error {
	return ValidateRef(ref, "sandbox")
}

// ValidateOptionalRuntimeRef validates runtime_ref when non-empty.
func ValidateOptionalRuntimeRef(ref string) error {
	if strings.TrimSpace(ref) == "" {
		return nil
	}
	return ValidateRef(ref, "runtime")
}

// 联调 fixtures — full ARNs (networkMode=PUBLIC today; production attach rejects).
// Region ap-northeast-1, account 072107099780.
const (
	FixtureHarnessARN = "arn:aws:bedrock-agentcore:ap-northeast-1:072107099780:harness/harness_l0zfw-Ml59dium3t"
	FixtureRuntimeARN = "arn:aws:bedrock-agentcore:ap-northeast-1:072107099780:runtime/harness_harness_l0zfw-GlZZB92yvC"
	// Short ids kept for tests that do not need ARN shape.
	FixtureHarnessID = "harness_l0zfw-Ml59dium3t"
	FixtureRuntimeID = "harness_harness_l0zfw-GlZZB92yvC"
)
