package handlers

import "testing"

// Registration stores only known capabilities; an agent that reports none
// (pre-capability build) is stored as [] so a re-registered downgraded agent
// loses its capabilities instead of keeping stale ones.
func TestAgentCapabilitiesJSON(t *testing.T) {
	cases := map[string][]string{
		`[]`:                     nil,
		`["manifest_bundle_v1"]`: {"manifest_bundle_v1", "rm -rf /"},
		`["manifest_bundle_v1","task_data_overrides_v1"]`: {"manifest_bundle_v1", "task_data_overrides_v1"},
	}
	for want, in := range cases {
		if got := *agentCapabilitiesJSON(in); got != want {
			t.Errorf("agentCapabilitiesJSON(%q) = %s, want %s", in, got, want)
		}
	}
}
