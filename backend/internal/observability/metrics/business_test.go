package metrics

import (
	"testing"

	dto "github.com/prometheus/client_model/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// resetBusinessMetrics clears all package-level business metric variables so
// each test starts from a clean state.
func resetBusinessMetrics() {
	workspaceTasksTotal = nil
	workspaceTaskDuration = nil
	workspaceDriftDetectedTotal = nil
	agentConnections = nil
	agentTasksDispatchedTotal = nil
	agentTasksCompletedTotal = nil
	authLoginsTotal = nil
	authTokensIssuedTotal = nil
	aiTokensTotal = nil
}

// findMetricFamily returns the MetricFamily with the given name from the
// slice, or nil if not found.
func findMetricFamily(mfs []*dto.MetricFamily, name string) *dto.MetricFamily {
	for _, mf := range mfs {
		if mf.GetName() == name {
			return mf
		}
	}
	return nil
}

// allBusinessMetricNames returns the names of all 9 business metrics that
// RegisterBusinessMetrics should register.
func allBusinessMetricNames() []string {
	return []string{
		"iac_workspace_tasks_total",
		"iac_workspace_task_duration_seconds",
		"iac_workspace_drift_detected_total",
		"iac_agent_connections",
		"iac_agent_tasks_dispatched_total",
		"iac_agent_tasks_completed_total",
		"iac_auth_logins_total",
		"iac_auth_tokens_issued_total",
		"iac_ai_tokens_total",
	}
}

// ---------------------------------------------------------------------------
// Test: calling RegisterBusinessMetrics with nil does not panic
// ---------------------------------------------------------------------------

func TestRegisterBusinessMetrics_NilRegistry(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	assert.NotPanics(t, func() {
		RegisterBusinessMetrics(nil)
	}, "RegisterBusinessMetrics(nil) must not panic")

	// Package-level vars should still be nil after a nil registry call.
	assert.Nil(t, workspaceTasksTotal)
	assert.Nil(t, agentConnections)
	assert.Nil(t, authLoginsTotal)
	assert.Nil(t, aiTokensTotal)
}

// ---------------------------------------------------------------------------
// Test: all 9 metric families are registered on a fresh registry
// ---------------------------------------------------------------------------

func TestRegisterBusinessMetrics_RegistersAllMetrics(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	// Force every metric to emit at least one sample so that Gather() returns
	// all families. Counters / histograms only appear after being observed.
	RecordTaskCompleted("plan", "success", 0.1)
	RecordDriftDetected(true)
	IncAgentConnected()
	IncAgentTaskDispatched("default")
	IncAgentTaskCompleted("default", "success")
	IncLoginTotal("local", "success")
	IncTokenIssued("access")
	IncAITokens("openai", "prompt", 10)

	mfs, err := reg.Gather()
	require.NoError(t, err, "Gather() must not return an error")

	for _, name := range allBusinessMetricNames() {
		mf := findMetricFamily(mfs, name)
		assert.NotNilf(t, mf, "Gather() must contain metric family %q", name)
	}
}

// ---------------------------------------------------------------------------
// Test: RecordTaskCompleted increments counter and observes histogram
// ---------------------------------------------------------------------------

func TestRecordTaskCompleted(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	RecordTaskCompleted("plan", "success", 1.5)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	// Verify counter
	mf := findMetricFamily(mfs, "iac_workspace_tasks_total")
	require.NotNil(t, mf, "iac_workspace_tasks_total must be gathered")
	require.Len(t, mf.GetMetric(), 1)
	assert.Equal(t, float64(1), mf.GetMetric()[0].GetCounter().GetValue(),
		"counter must equal 1 after one RecordTaskCompleted call")

	// Verify histogram
	mf = findMetricFamily(mfs, "iac_workspace_task_duration_seconds")
	require.NotNil(t, mf, "iac_workspace_task_duration_seconds must be gathered")
	require.Len(t, mf.GetMetric(), 1)

	h := mf.GetMetric()[0].GetHistogram()
	require.NotNil(t, h)
	assert.Equal(t, uint64(1), h.GetSampleCount(), "histogram sample count must be 1")
	assert.Equal(t, 1.5, h.GetSampleSum(), "histogram sample sum must equal the observed duration")
}

// ---------------------------------------------------------------------------
// Test: RecordDriftDetected with true and false
// ---------------------------------------------------------------------------

func TestRecordDriftDetected(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	RecordDriftDetected(true)
	RecordDriftDetected(false)
	RecordDriftDetected(true)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	mf := findMetricFamily(mfs, "iac_workspace_drift_detected_total")
	require.NotNil(t, mf, "iac_workspace_drift_detected_total must be gathered")

	// Expect two label combinations: has_drift="true" (count 2) and has_drift="false" (count 1).
	metrics := mf.GetMetric()
	require.Len(t, metrics, 2, "must have two label combinations (true and false)")

	counts := make(map[string]float64)
	for _, m := range metrics {
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "has_drift" {
				counts[lp.GetValue()] = m.GetCounter().GetValue()
			}
		}
	}

	assert.Equal(t, float64(2), counts["true"], "has_drift=true counter must be 2")
	assert.Equal(t, float64(1), counts["false"], "has_drift=false counter must be 1")
}

// ---------------------------------------------------------------------------
// Test: Agent metrics (gauge + counters)
// ---------------------------------------------------------------------------

func TestAgentMetrics(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	// Inc twice, Dec once => gauge should be 1.
	IncAgentConnected()
	IncAgentConnected()
	DecAgentConnected()

	IncAgentTaskDispatched("agent")
	IncAgentTaskCompleted("agent", "success")

	mfs, err := reg.Gather()
	require.NoError(t, err)

	// --- agent connections gauge ---
	mf := findMetricFamily(mfs, "iac_agent_connections")
	require.NotNil(t, mf, "iac_agent_connections must be gathered")
	require.Len(t, mf.GetMetric(), 1)
	assert.Equal(t, float64(1), mf.GetMetric()[0].GetGauge().GetValue(),
		"agent connections gauge must be 1 after Inc, Inc, Dec")

	// --- agent tasks dispatched counter ---
	mf = findMetricFamily(mfs, "iac_agent_tasks_dispatched_total")
	require.NotNil(t, mf, "iac_agent_tasks_dispatched_total must be gathered")
	require.Len(t, mf.GetMetric(), 1)
	assert.Equal(t, float64(1), mf.GetMetric()[0].GetCounter().GetValue(),
		"dispatched counter must be 1")

	// Verify label pool_type="agent"
	foundPoolType := false
	for _, lp := range mf.GetMetric()[0].GetLabel() {
		if lp.GetName() == "pool_type" && lp.GetValue() == "agent" {
			foundPoolType = true
		}
	}
	assert.True(t, foundPoolType, "dispatched metric must have pool_type=agent label")

	// --- agent tasks completed counter ---
	mf = findMetricFamily(mfs, "iac_agent_tasks_completed_total")
	require.NotNil(t, mf, "iac_agent_tasks_completed_total must be gathered")
	require.Len(t, mf.GetMetric(), 1)
	assert.Equal(t, float64(1), mf.GetMetric()[0].GetCounter().GetValue(),
		"completed counter must be 1")
}

// ---------------------------------------------------------------------------
// Test: Auth metrics (logins + tokens)
// ---------------------------------------------------------------------------

func TestAuthMetrics(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	IncLoginTotal("local", "success")
	IncLoginTotal("sso", "failure")
	IncTokenIssued("access")

	mfs, err := reg.Gather()
	require.NoError(t, err)

	// --- auth logins ---
	mf := findMetricFamily(mfs, "iac_auth_logins_total")
	require.NotNil(t, mf, "iac_auth_logins_total must be gathered")
	require.Len(t, mf.GetMetric(), 2, "must have two login label combinations")

	loginCounts := make(map[string]float64) // keyed by "method:status"
	for _, m := range mf.GetMetric() {
		var method, status string
		for _, lp := range m.GetLabel() {
			switch lp.GetName() {
			case "method":
				method = lp.GetValue()
			case "status":
				status = lp.GetValue()
			}
		}
		loginCounts[method+":"+status] = m.GetCounter().GetValue()
	}

	assert.Equal(t, float64(1), loginCounts["local:success"], "local:success login counter must be 1")
	assert.Equal(t, float64(1), loginCounts["sso:failure"], "sso:failure login counter must be 1")

	// --- tokens issued ---
	mf = findMetricFamily(mfs, "iac_auth_tokens_issued_total")
	require.NotNil(t, mf, "iac_auth_tokens_issued_total must be gathered")
	require.Len(t, mf.GetMetric(), 1)
	assert.Equal(t, float64(1), mf.GetMetric()[0].GetCounter().GetValue(),
		"tokens issued counter must be 1")

	// Verify label type="access"
	foundType := false
	for _, lp := range mf.GetMetric()[0].GetLabel() {
		if lp.GetName() == "type" && lp.GetValue() == "access" {
			foundType = true
		}
	}
	assert.True(t, foundType, "tokens issued metric must have type=access label")
}

// ---------------------------------------------------------------------------
// Test: AI Tokens metric
// ---------------------------------------------------------------------------

func TestAITokensMetric(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	reg := prometheus.NewRegistry()
	RegisterBusinessMetrics(reg)

	IncAITokens("openai", "prompt", 150)
	IncAITokens("openai", "completion", 50)
	IncAITokens("openai", "prompt", 100)

	mfs, err := reg.Gather()
	require.NoError(t, err)

	mf := findMetricFamily(mfs, "iac_ai_tokens_total")
	require.NotNil(t, mf, "iac_ai_tokens_total must be gathered")
	require.Len(t, mf.GetMetric(), 2, "must have two label combinations (prompt and completion)")

	counts := make(map[string]float64)
	for _, m := range mf.GetMetric() {
		var tokenType string
		for _, lp := range m.GetLabel() {
			if lp.GetName() == "type" {
				tokenType = lp.GetValue()
			}
		}
		counts[tokenType] = m.GetCounter().GetValue()
	}

	assert.Equal(t, float64(250), counts["prompt"],
		"openai:prompt counter must be 250 (150 + 100)")
	assert.Equal(t, float64(50), counts["completion"],
		"openai:completion counter must be 50")
}

// ---------------------------------------------------------------------------
// Test: calling all record functions before registration does not panic
// ---------------------------------------------------------------------------

func TestRecordFunctions_BeforeRegistration(t *testing.T) {
	resetBusinessMetrics()
	defer resetBusinessMetrics()

	// All package-level vars are nil at this point (reset above, no
	// RegisterBusinessMetrics call). Every record function must be safe to
	// call without panicking.
	assert.NotPanics(t, func() {
		RecordTaskCompleted("plan", "success", 1.0)
	}, "RecordTaskCompleted must not panic before registration")

	assert.NotPanics(t, func() {
		RecordDriftDetected(true)
	}, "RecordDriftDetected must not panic before registration")

	assert.NotPanics(t, func() {
		RecordDriftDetected(false)
	}, "RecordDriftDetected(false) must not panic before registration")

	assert.NotPanics(t, func() {
		IncAgentConnected()
	}, "IncAgentConnected must not panic before registration")

	assert.NotPanics(t, func() {
		DecAgentConnected()
	}, "DecAgentConnected must not panic before registration")

	assert.NotPanics(t, func() {
		IncAgentTaskDispatched("pool")
	}, "IncAgentTaskDispatched must not panic before registration")

	assert.NotPanics(t, func() {
		IncAgentTaskCompleted("pool", "ok")
	}, "IncAgentTaskCompleted must not panic before registration")

	assert.NotPanics(t, func() {
		IncLoginTotal("local", "success")
	}, "IncLoginTotal must not panic before registration")

	assert.NotPanics(t, func() {
		IncTokenIssued("access")
	}, "IncTokenIssued must not panic before registration")

	assert.NotPanics(t, func() {
		IncAITokens("openai", "prompt", 100)
	}, "IncAITokens must not panic before registration")
}
