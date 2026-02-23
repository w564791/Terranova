package metrics

import (
	"fmt"
	"log"

	"github.com/prometheus/client_golang/prometheus"
)

// ---------------------------------------------------------------------------
// Package-level metric variables. These are set by RegisterBusinessMetrics and
// referenced by the record/increment helpers below. When nil (i.e. before
// RegisterBusinessMetrics is called), callers simply skip recording.
// ---------------------------------------------------------------------------

// Workspace Task metrics
var (
	workspaceTasksTotal        *prometheus.CounterVec
	workspaceTaskDuration      *prometheus.HistogramVec
	workspaceDriftDetectedTotal *prometheus.CounterVec
)

// Agent metrics
var (
	agentConnections         prometheus.Gauge
	agentTasksDispatchedTotal *prometheus.CounterVec
	agentTasksCompletedTotal  *prometheus.CounterVec
)

// Auth metrics
var (
	authLoginsTotal      *prometheus.CounterVec
	authTokensIssuedTotal *prometheus.CounterVec
)

// AI Token metrics
var (
	aiTokensTotal *prometheus.CounterVec
)

// RegisterBusinessMetrics registers all business-related Prometheus metrics on
// the provided registry. If reg is nil the call is a no-op.
func RegisterBusinessMetrics(reg *prometheus.Registry) {
	if reg == nil {
		return
	}

	// --- 1.1 Workspace Task Metrics ---

	workspaceTasksTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_workspace_tasks_total",
			Help: "Total number of workspace tasks executed.",
		},
		[]string{"type", "status"},
	)

	workspaceTaskDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iac_workspace_task_duration_seconds",
			Help:    "Duration of workspace tasks in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"type"},
	)

	workspaceDriftDetectedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_workspace_drift_detected_total",
			Help: "Total number of drift detection runs.",
		},
		[]string{"has_drift"},
	)

	// --- 1.2 Agent Metrics ---

	agentConnections = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iac_agent_connections",
		Help: "Current number of connected agents.",
	})

	agentTasksDispatchedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_agent_tasks_dispatched_total",
			Help: "Total number of tasks dispatched to agents.",
		},
		[]string{"pool_type"},
	)

	agentTasksCompletedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_agent_tasks_completed_total",
			Help: "Total number of tasks completed by agents.",
		},
		[]string{"pool_type", "status"},
	)

	// --- 1.3 Auth Metrics ---

	authLoginsTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_auth_logins_total",
			Help: "Total number of login attempts.",
		},
		[]string{"method", "status"},
	)

	authTokensIssuedTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_auth_tokens_issued_total",
			Help: "Total number of tokens issued.",
		},
		[]string{"type"},
	)

	// --- 1.4 AI Token Metrics ---

	aiTokensTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_ai_tokens_total",
			Help: "Total number of AI tokens consumed.",
		},
		[]string{"provider", "type"},
	)

	reg.MustRegister(
		workspaceTasksTotal,
		workspaceTaskDuration,
		workspaceDriftDetectedTotal,
		agentConnections,
		agentTasksDispatchedTotal,
		agentTasksCompletedTotal,
		authLoginsTotal,
		authTokensIssuedTotal,
		aiTokensTotal,
	)
}

// ---------------------------------------------------------------------------
// Record / Increment helpers
// ---------------------------------------------------------------------------

// RecordTaskCompleted increments the workspace task counter and observes
// the task duration histogram for the given task type and status.
func RecordTaskCompleted(taskType, status string, durationSeconds float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in RecordTaskCompleted: %v", r)
		}
	}()

	if workspaceTasksTotal != nil {
		workspaceTasksTotal.WithLabelValues(taskType, status).Inc()
	}
	if workspaceTaskDuration != nil {
		workspaceTaskDuration.WithLabelValues(taskType).Observe(durationSeconds)
	}
}

// RecordDriftDetected increments the drift detection counter. The hasDrift
// parameter is converted to a "true"/"false" label value.
func RecordDriftDetected(hasDrift bool) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in RecordDriftDetected: %v", r)
		}
	}()

	if workspaceDriftDetectedTotal != nil {
		workspaceDriftDetectedTotal.WithLabelValues(fmt.Sprintf("%t", hasDrift)).Inc()
	}
}

// IncAgentConnected increments the agent connections gauge by 1.
func IncAgentConnected() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncAgentConnected: %v", r)
		}
	}()

	if agentConnections != nil {
		agentConnections.Inc()
	}
}

// DecAgentConnected decrements the agent connections gauge by 1.
func DecAgentConnected() {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in DecAgentConnected: %v", r)
		}
	}()

	if agentConnections != nil {
		agentConnections.Dec()
	}
}

// IncAgentTaskDispatched increments the dispatched tasks counter for the
// given pool type.
func IncAgentTaskDispatched(poolType string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncAgentTaskDispatched: %v", r)
		}
	}()

	if agentTasksDispatchedTotal != nil {
		agentTasksDispatchedTotal.WithLabelValues(poolType).Inc()
	}
}

// IncAgentTaskCompleted increments the completed tasks counter for the given
// pool type and status.
func IncAgentTaskCompleted(poolType, status string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncAgentTaskCompleted: %v", r)
		}
	}()

	if agentTasksCompletedTotal != nil {
		agentTasksCompletedTotal.WithLabelValues(poolType, status).Inc()
	}
}

// IncLoginTotal increments the login attempts counter for the given method
// and status.
func IncLoginTotal(method, status string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncLoginTotal: %v", r)
		}
	}()

	if authLoginsTotal != nil {
		authLoginsTotal.WithLabelValues(method, status).Inc()
	}
}

// IncTokenIssued increments the tokens-issued counter for the given token type.
func IncTokenIssued(tokenType string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncTokenIssued: %v", r)
		}
	}()

	if authTokensIssuedTotal != nil {
		authTokensIssuedTotal.WithLabelValues(tokenType).Inc()
	}
}

// IncAITokens adds the given count to the AI tokens counter for the given
// provider and token type.
func IncAITokens(provider, tokenType string, count float64) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[metrics] recovered from panic in IncAITokens: %v", r)
		}
	}()

	if aiTokensTotal != nil {
		aiTokensTotal.WithLabelValues(provider, tokenType).Add(count)
	}
}
