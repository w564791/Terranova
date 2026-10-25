package sandboxreg

import (
	"strings"
	"testing"
)

func TestValidateSandboxRef_OK(t *testing.T) {
	for _, r := range []string{
		FixtureHarnessID,
		"a1",
		"Arn.Like:with-dashes_ok",
		strings.Repeat("x", 128),
	} {
		if err := ValidateSandboxRef(r); err != nil {
			t.Fatalf("%q: %v", r, err)
		}
	}
}

func TestValidateSandboxRef_Reject(t *testing.T) {
	cases := []string{
		"",
		"https://example.com/x",
		"http://x",
		"s3://bucket/key",
		"arn:aws:bedrock-agentcore:ap-northeast-1:072107099780:harness/harness_l0zfw-Ml59dium3t", // slash
		"-bad",
		".bad",
		"a",
		"has space",
		strings.Repeat("z", 200),
	}
	for _, r := range cases {
		err := ValidateSandboxRef(r)
		if err == nil {
			t.Fatalf("expected error for %q", r)
		}
		ve, ok := err.(*ValidationError)
		if !ok || ve.Code != ErrInvalidSandboxRef {
			t.Fatalf("%q: got %#v", r, err)
		}
	}
}

func TestValidateOptionalRuntimeRef(t *testing.T) {
	if err := ValidateOptionalRuntimeRef(""); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOptionalRuntimeRef(FixtureRuntimeID); err != nil {
		t.Fatal(err)
	}
	if err := ValidateOptionalRuntimeRef("https://x"); err == nil {
		t.Fatal("expected URL reject")
	}
}
