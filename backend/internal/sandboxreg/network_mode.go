package sandboxreg

import (
	"strings"

	"iac-platform/internal/keys"
)

const (
	NetworkModeVPC    = "VPC"
	NetworkModePublic = "PUBLIC"

	// ErrNetworkModeNotAllowed is returned when PUBLIC is refused in production.
	ErrNetworkModeNotAllowed = "network_mode_not_allowed"
)

// NormalizeNetworkMode uppercases a known mode; empty stays empty.
func NormalizeNetworkMode(mode string) string {
	m := strings.ToUpper(strings.TrimSpace(mode))
	switch m {
	case NetworkModeVPC, NetworkModePublic:
		return m
	default:
		return strings.TrimSpace(mode)
	}
}

// ValidateNetworkModeForAttach refuses PUBLIC when the platform runs in
// production (ENV=production via keys.IsProduction). Non-production may use
// PUBLIC for 联调. Call before wake / preview / Deploy attach — re-check the
// stored snapshot even if the row was registered while ENV was non-prod.
//
// Live AWS GetAgentRuntime networkMode lookup is intentionally not done here;
// 联调 stub skips AWS until platform AgentCore vault credentials are wired.
func ValidateNetworkModeForAttach(networkMode string) error {
	m := NormalizeNetworkMode(networkMode)
	if m == NetworkModePublic && keys.IsProduction() {
		return &ValidationError{
			Code:    ErrNetworkModeNotAllowed,
			Message: "AgentCore networkMode=PUBLIC is not allowed in production; use VPC",
		}
	}
	return nil
}

// IsPublic reports whether the stored snapshot is PUBLIC (case-insensitive).
func IsPublic(networkMode string) bool {
	return NormalizeNetworkMode(networkMode) == NetworkModePublic
}
