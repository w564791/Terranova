package health

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func setupTestRouter(db interface{}) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// db parameter is unused on purpose — we always pass nil *gorm.DB to
	// RegisterRoutes in these tests so that we can exercise the error paths
	// without needing a real database.
	RegisterRoutes(r, nil)
	return r
}

// TestHealthLive verifies that GET /health/live always returns 200 with
// {"status":"healthy"}.
func TestHealthLive(t *testing.T) {
	r := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health/live", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "healthy" {
		t.Errorf("expected status \"healthy\", got %q", body["status"])
	}
}

// TestHealth verifies the backward-compatible GET /health endpoint returns
// exactly {"status":"ok"} (约束 6.2 regression test).
func TestHealth(t *testing.T) {
	r := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("expected status \"ok\", got %q", body["status"])
	}

	// Ensure the body is exactly {"status":"ok"} with no extra keys.
	if len(body) != 1 {
		t.Errorf("expected exactly 1 key in response body, got %d: %v", len(body), body)
	}
}

// TestHealthReady_NilDB verifies that GET /health/ready returns 503 when the
// database connection is nil.
func TestHealthReady_NilDB(t *testing.T) {
	r := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health/ready", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "unhealthy" {
		t.Errorf("expected status \"unhealthy\", got %q", body["status"])
	}
}

// TestHealthStartup_NilDB verifies that GET /health/startup returns 503 when
// the database connection is nil (even if startupReady is true).
func TestHealthStartup_NilDB(t *testing.T) {
	// Mark startup as ready so the startup flag check passes — the DB check
	// should still cause a 503.
	startupReady.Store(true)
	defer startupReady.Store(false)

	r := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health/startup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}
}

// TestHealthStartup_NotReady verifies that GET /health/startup returns 503
// when startupReady has not been set, regardless of DB state.
func TestHealthStartup_NotReady(t *testing.T) {
	startupReady.Store(false)

	r := setupTestRouter(nil)

	req := httptest.NewRequest(http.MethodGet, "/health/startup", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected status 503, got %d", w.Code)
	}

	var body map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if body["status"] != "unhealthy" {
		t.Errorf("expected status \"unhealthy\", got %q", body["status"])
	}
}
