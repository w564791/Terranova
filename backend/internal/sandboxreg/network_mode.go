package sandboxreg

import (
	"strings"

	"iac-platform/internal/keys"
)

const (
	NetworkModeVPC    = "VPC"
	NetworkModePublic = "PUBLIC"

	// ErrNetworkModeNotAllowed is production policy refusal on attach/create:
	// PUBLIC, empty, or anything other than VPC while ENV=production.
	ErrNetworkModeNotAllowed = "network_mode_not_allowed"
	// ErrInvalidNetworkMode is a malformed create body value (not VPC|PUBLIC).
	ErrInvalidNetworkMode = "invalid_network_mode"
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

// ParseNetworkModeForCreate validates an optional create-body network_mode.
// Empty / omitted → ("", nil) meaning unknown yet.
// VPC|PUBLIC (any case) → normalized value.
// Anything else → 422 invalid_network_mode (not network_mode_not_allowed).
func ParseNetworkModeForCreate(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	nm := NormalizeNetworkMode(trimmed)
	if nm != NetworkModeVPC && nm != NetworkModePublic {
		return "", &ValidationError{
			Code:    ErrInvalidNetworkMode,
			Message: "network_mode must be VPC or PUBLIC",
		}
	}
	return nm, nil
}

// ValidateNetworkModeForAttach enforces attach/wake/Deploy/preview policy.
//
// Production (ENV=production / keys.IsProduction): stored snapshot must be
// exactly VPC. Empty, PUBLIC, or unknown → 422 network_mode_not_allowed.
// Non-production: empty, PUBLIC, or VPC are all allowed (联调).
//
// Always re-check the stored snapshot before wake — do not rely on
// registration-time ENV. Live AWS Get networkMode is not done here.
func ValidateNetworkModeForAttach(networkMode string) error {
	m := NormalizeNetworkMode(networkMode)
	if !keys.IsProduction() {
		// Non-prod: empty | VPC | PUBLIC OK; unknown non-empty still rejected
		// so typos do not silently attach.
		if m == "" || m == NetworkModeVPC || m == NetworkModePublic {
			return nil
		}
		return &ValidationError{
			Code:    ErrInvalidNetworkMode,
			Message: "network_mode must be VPC or PUBLIC",
		}
	}
	if m != NetworkModeVPC {
		msg := "AgentCore networkMode must be VPC in production"
		switch m {
		case "":
			msg = "AgentCore networkMode snapshot is required in production (must be VPC)"
		case NetworkModePublic:
			msg = "AgentCore networkMode=PUBLIC is not allowed in production; use VPC"
		}
		return &ValidationError{
			Code:    ErrNetworkModeNotAllowed,
			Message: msg,
		}
	}
	return nil
}

// IsPublic reports whether the stored snapshot is PUBLIC (case-insensitive).
func IsPublic(networkMode string) bool {
	return NormalizeNetworkMode(networkMode) == NetworkModePublic
}
