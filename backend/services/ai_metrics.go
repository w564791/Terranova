package services

import (
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ---------- private registry & metric vars ----------

var (
	aiRegistry     *prometheus.Registry
	aiRegistryOnce sync.Once

	// Histograms
	aiCallDuration          *prometheus.HistogramVec
	vectorSearchDuration    *prometheus.HistogramVec
	skillAssemblyDuration   *prometheus.HistogramVec
	parallelExecutionDur    *prometheus.HistogramVec
	domainSkillSelectionDur *prometheus.HistogramVec
	cmdbAssessmentDur       *prometheus.HistogramVec

	// Counters
	aiCallTotal        *prometheus.CounterVec
	vectorSearchTotal  *prometheus.CounterVec
	cmdbQueryTotal     *prometheus.CounterVec

	// Gauges
	activeParallelTasks *prometheus.GaugeVec
)

// defaultBuckets bucket 边界（毫秒）
var defaultBuckets = []float64{10, 50, 100, 250, 500, 1000, 2500, 5000, 10000, 30000, 60000}

// initAIMetrics lazily creates the private registry and registers all metrics.
func initAIMetrics() {
	aiRegistryOnce.Do(func() {
		aiRegistry = prometheus.NewRegistry()

		// --- Histograms ---

		aiCallDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_ai_call_duration_ms",
			Help:    "AI service call duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"capability", "stage"})

		vectorSearchDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_vector_search_duration_ms",
			Help:    "Vector search duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"resource_type", "stage"})

		skillAssemblyDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_skill_assembly_duration_ms",
			Help:    "Skill assembly duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"capability", "skill_count"})

		parallelExecutionDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_parallel_execution_ms",
			Help:    "Parallel execution duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"task", "status"})

		domainSkillSelectionDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_domain_skill_selection_ms",
			Help:    "Domain skill selection duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"skill_count", "method"})

		cmdbAssessmentDur = prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "iac_cmdb_assessment_ms",
			Help:    "CMDB assessment duration in milliseconds",
			Buckets: defaultBuckets,
		}, []string{"need_cmdb", "resource_type_count", "method"})

		// --- Counters ---

		aiCallTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iac_ai_call_total",
			Help: "Total number of AI service calls",
		}, []string{"capability", "status"})

		vectorSearchTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iac_vector_search_total",
			Help: "Total number of vector searches",
		}, []string{"resource_type", "status"})

		cmdbQueryTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "iac_cmdb_query_total",
			Help: "Total number of CMDB queries",
		}, []string{"resource_type", "status", "candidate_count"})

		// --- Gauges ---

		activeParallelTasks = prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "iac_active_parallel_tasks",
			Help: "Current number of active parallel tasks",
		}, []string{})

		// Register everything
		aiRegistry.MustRegister(
			aiCallDuration,
			vectorSearchDuration,
			skillAssemblyDuration,
			parallelExecutionDur,
			domainSkillSelectionDur,
			cmdbAssessmentDur,
			aiCallTotal,
			vectorSearchTotal,
			cmdbQueryTotal,
			activeParallelTasks,
		)
	})
}

// GetAIMetricsRegistry returns the private Prometheus registry used by all AI
// metrics. It is intended for Task 6 to merge into the combined /metrics
// endpoint via a prometheus.Gatherer.
func GetAIMetricsRegistry() *prometheus.Registry {
	initAIMetrics()
	return aiRegistry
}

// MetricsHandler returns an http.HandlerFunc that serves all AI metrics in
// Prometheus exposition format.
func MetricsHandler() http.HandlerFunc {
	initAIMetrics()
	h := promhttp.HandlerFor(aiRegistry, promhttp.HandlerOpts{})
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}

// ========== Convenience Functions (12 exported, signatures unchanged) ==========

// RecordAICallDuration records AI call duration in milliseconds.
func RecordAICallDuration(capability, stage string, durationMs float64) {
	initAIMetrics()
	aiCallDuration.WithLabelValues(capability, stage).Observe(durationMs)
}

// RecordVectorSearchDuration records vector search duration in milliseconds.
func RecordVectorSearchDuration(resourceType, stage string, durationMs float64) {
	initAIMetrics()
	vectorSearchDuration.WithLabelValues(resourceType, stage).Observe(durationMs)
}

// RecordSkillAssemblyDuration records skill assembly duration in milliseconds.
func RecordSkillAssemblyDuration(capability string, skillCount int, durationMs float64) {
	initAIMetrics()
	skillAssemblyDuration.WithLabelValues(capability, fmt.Sprintf("%d", skillCount)).Observe(durationMs)
}

// IncAICallCount increments the AI call counter.
func IncAICallCount(capability, status string) {
	initAIMetrics()
	aiCallTotal.WithLabelValues(capability, status).Inc()
}

// IncVectorSearchCount increments the vector search counter.
func IncVectorSearchCount(resourceType string, found bool) {
	status := "not_found"
	if found {
		status = "found"
	}
	initAIMetrics()
	vectorSearchTotal.WithLabelValues(resourceType, status).Inc()
}

// RecordParallelExecutionDuration records parallel execution duration in milliseconds.
func RecordParallelExecutionDuration(task, status string, durationMs float64) {
	initAIMetrics()
	parallelExecutionDur.WithLabelValues(task, status).Observe(durationMs)
}

// RecordDomainSkillSelection records domain skill selection duration in milliseconds.
func RecordDomainSkillSelection(skillCount int, method string, durationMs float64) {
	initAIMetrics()
	domainSkillSelectionDur.WithLabelValues(fmt.Sprintf("%d", skillCount), method).Observe(durationMs)
}

// RecordCMDBAssessment records CMDB assessment duration in milliseconds.
func RecordCMDBAssessment(needCMDB bool, resourceTypeCount int, method string, durationMs float64) {
	initAIMetrics()
	cmdbAssessmentDur.WithLabelValues(
		fmt.Sprintf("%t", needCMDB),
		fmt.Sprintf("%d", resourceTypeCount),
		method,
	).Observe(durationMs)
}

// IncCMDBQueryCount increments the CMDB query counter.
func IncCMDBQueryCount(resourceType string, found bool, candidateCount int) {
	status := "not_found"
	if found {
		if candidateCount > 1 {
			status = "multiple"
		} else {
			status = "found"
		}
	}
	initAIMetrics()
	cmdbQueryTotal.WithLabelValues(resourceType, status, fmt.Sprintf("%d", candidateCount)).Inc()
}

// SetActiveParallelTasks sets the current number of active parallel tasks.
func SetActiveParallelTasks(count int) {
	initAIMetrics()
	activeParallelTasks.WithLabelValues().Set(float64(count))
}

// Timer is a simple timer for conveniently recording durations.
type Timer struct {
	start time.Time
}

// NewTimer creates a new Timer that records the current time.
func NewTimer() *Timer {
	return &Timer{start: time.Now()}
}

// ElapsedMs returns the number of milliseconds elapsed since the Timer was created.
func (t *Timer) ElapsedMs() float64 {
	return float64(time.Since(t.start).Milliseconds())
}
