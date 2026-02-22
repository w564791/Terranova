package services

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestAIMetricsSignatures verifies that all 12 exported convenience functions
// plus Timer compile and can be called with the documented parameter types.
// If this test compiles, the 111 call-sites in the codebase remain compatible.
func TestAIMetricsSignatures(t *testing.T) {
	// Histograms
	RecordAICallDuration("nlq", "total", 123.4)
	RecordVectorSearchDuration("host", "embed", 45.6)
	RecordSkillAssemblyDuration("nlq", 3, 78.9)
	RecordParallelExecutionDuration("vector_search", "success", 200.0)
	RecordDomainSkillSelection(2, "auto", 15.0)
	RecordCMDBAssessment(true, 3, "rule", 50.0)

	// Counters
	IncAICallCount("nlq", "success")
	IncVectorSearchCount("host", true)
	IncCMDBQueryCount("host", true, 1)

	// Gauge
	SetActiveParallelTasks(5)

	// Timer
	timer := NewTimer()
	elapsed := timer.ElapsedMs()
	if elapsed < 0 {
		t.Error("ElapsedMs returned negative value")
	}

	// MetricsHandler returns http.HandlerFunc
	var _ http.HandlerFunc = MetricsHandler()
}

// TestAIMetricsOutput verifies that the /metrics endpoint produces output
// containing the expected metric names.
func TestAIMetricsOutput(t *testing.T) {
	// Record at least one observation per metric so they appear in output
	RecordAICallDuration("test_cap", "total", 42.0)
	IncAICallCount("test_cap", "success")

	handler := MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	// Must contain these metric names
	for _, want := range []string{
		"iac_ai_call_duration_ms",
		"iac_ai_call_total",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing %q", want)
		}
	}
}

// TestAIMetricsBuckets verifies that histogram bucket boundaries include the
// expected le values from defaultBuckets.
func TestAIMetricsBuckets(t *testing.T) {
	RecordAICallDuration("bucket_test", "total", 5.0)

	handler := MetricsHandler()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	body := rec.Body.String()

	for _, want := range []string{
		`le="10"`,
		`le="250"`,
		`le="60000"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics output missing bucket boundary %s\nBody:\n%s", want, body)
		}
	}
}

// TestAIMetricsRegistry verifies that GetAIMetricsRegistry returns a non-nil
// registry that can be used as a Gatherer.
func TestAIMetricsRegistry(t *testing.T) {
	reg := GetAIMetricsRegistry()
	if reg == nil {
		t.Fatal("GetAIMetricsRegistry() returned nil")
	}

	// Gather should succeed even if no observations yet for some metrics
	mfs, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error: %v", err)
	}
	if len(mfs) == 0 {
		t.Error("Gather() returned no metric families")
	}
}
