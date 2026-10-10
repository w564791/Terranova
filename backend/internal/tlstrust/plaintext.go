package tlstrust

import (
	"fmt"
	"os"
	"strings"

	"iac-platform/internal/keys"
)

// Exact value that opts a production agent into plaintext HTTP/WS
// (cluster-internal traffic only). Any other non-empty value is a startup error.
const AllowPlaintextClusterInternal = "cluster-internal"

const (
	EnvAgentProtocol       = "IAC_AGENT_PROTOCOL"
	EnvAgentAllowPlaintext = "IAC_AGENT_ALLOW_PLAINTEXT"
)

// ResolveAgentProtocol returns the effective agent API protocol (http|https).
// Empty IAC_AGENT_PROTOCOL defaults to http (historical default).
func ResolveAgentProtocol(raw string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(raw))
	if p == "" {
		p = "http"
	}
	if p != "http" && p != "https" {
		return "", fmt.Errorf("invalid %s: %s (must be 'http' or 'https')", EnvAgentProtocol, raw)
	}
	return p, nil
}

// WebSocketScheme maps the agent API protocol to the C&C WebSocket scheme.
func WebSocketScheme(protocol string) string {
	if protocol == "https" {
		return "wss"
	}
	return "ws"
}

// CheckAgentPlaintext validates plaintext transport for the agent API
// (http) and C&C WebSocket (ws). In production (ENV=production):
//   - https/wss: ok
//   - http/ws with IAC_AGENT_ALLOW_PLAINTEXT=cluster-internal: ok with a warning
//   - http/ws otherwise (including the default): refuse to start
//   - any other IAC_AGENT_ALLOW_PLAINTEXT value: refuse to start
//
// Outside production, http/ws is allowed with a warning when used.
func CheckAgentPlaintext(protocol string) (warnings []string, err error) {
	allow := strings.TrimSpace(os.Getenv(EnvAgentAllowPlaintext))
	if allow != "" && allow != AllowPlaintextClusterInternal {
		return nil, fmt.Errorf("%s=%q is invalid (only %q is accepted)", EnvAgentAllowPlaintext, allow, AllowPlaintextClusterInternal)
	}

	plaintext := protocol == "http"
	ws := WebSocketScheme(protocol)
	if !plaintext {
		return nil, nil
	}

	msg := fmt.Sprintf("%s=%s (C&C %s): agent token, variables and state travel unencrypted", EnvAgentProtocol, protocol, ws)
	if keys.IsProduction() {
		if allow == AllowPlaintextClusterInternal {
			warnings = append(warnings, fmt.Sprintf("!!! production agent using plaintext %s with %s=%s; intended for cluster-internal traffic only !!!", msg, EnvAgentAllowPlaintext, AllowPlaintextClusterInternal))
			return warnings, nil
		}
		return nil, fmt.Errorf("refusing to start in production with plaintext %s; set %s=https or %s=%s", msg, EnvAgentProtocol, EnvAgentAllowPlaintext, AllowPlaintextClusterInternal)
	}
	warnings = append(warnings, fmt.Sprintf("!!! %s. Allowed only because ENV is not production; set %s=https (or %s=%s in production for cluster-internal agents) !!!", msg, EnvAgentProtocol, EnvAgentAllowPlaintext, AllowPlaintextClusterInternal))
	return warnings, nil
}
