package metrics

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// TestHTTPMetrics_RequestsTotalRegistered verifies that after a request,
// the registry's Gather() output contains the iac_http_requests_total metric.
func TestHTTPMetrics_RequestsTotalRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()

	r := gin.New()
	r.Use(HTTPMetricsMiddleware(reg))
	r.GET("/ping", func(c *gin.Context) {
		c.String(http.StatusOK, "pong")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range families {
		if mf.GetName() == "iac_http_requests_total" {
			found = true
			break
		}
	}
	assert.True(t, found, "Gather() must contain iac_http_requests_total after a request")
}

// TestHTTPMetrics_RouteLabel verifies that the route label uses the
// route template (c.FullPath()) rather than the actual request path.
// A route registered as /api/v1/workspaces/:id should produce label
// "/api/v1/workspaces/:id" even when the request path is
// "/api/v1/workspaces/ws-123".
func TestHTTPMetrics_RouteLabel(t *testing.T) {
	reg := prometheus.NewRegistry()

	r := gin.New()
	r.Use(HTTPMetricsMiddleware(reg))
	r.GET("/api/v1/workspaces/:id", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/ws-123", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	families, err := reg.Gather()
	require.NoError(t, err)

	var routeLabel string
	for _, mf := range families {
		if mf.GetName() == "iac_http_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "route" {
						routeLabel = lp.GetValue()
					}
				}
			}
		}
	}

	assert.Equal(t, "/api/v1/workspaces/:id", routeLabel,
		"route label must be the template, not the actual path /api/v1/workspaces/ws-123")
}

// TestHTTPMetrics_NilRegistry verifies that passing a nil registry does
// not panic and the middleware still forwards the request.
func TestHTTPMetrics_NilRegistry(t *testing.T) {
	assert.NotPanics(t, func() {
		mw := HTTPMetricsMiddleware(nil)

		r := gin.New()
		r.Use(mw)
		r.GET("/safe", func(c *gin.Context) {
			c.String(http.StatusOK, "safe")
		})

		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/safe", nil)
		r.ServeHTTP(w, req)

		assert.Equal(t, http.StatusOK, w.Code)
	}, "HTTPMetricsMiddleware(nil) must not panic")
}

// TestHTTPMetrics_DurationRegistered verifies that the histogram metric
// iac_http_request_duration_seconds is also present after a request.
func TestHTTPMetrics_DurationRegistered(t *testing.T) {
	reg := prometheus.NewRegistry()

	r := gin.New()
	r.Use(HTTPMetricsMiddleware(reg))
	r.GET("/duration", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/duration", nil)
	r.ServeHTTP(w, req)

	families, err := reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range families {
		if mf.GetName() == "iac_http_request_duration_seconds" {
			found = true
			break
		}
	}
	assert.True(t, found, "Gather() must contain iac_http_request_duration_seconds after a request")
}

// TestHTTPMetrics_UnknownRoute verifies that when a request does not match
// any registered route, the route label falls back to "unknown".
func TestHTTPMetrics_UnknownRoute(t *testing.T) {
	reg := prometheus.NewRegistry()

	r := gin.New()
	r.Use(HTTPMetricsMiddleware(reg))
	// No routes registered — any request will result in an empty FullPath().

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/does-not-exist", nil)
	r.ServeHTTP(w, req)

	families, err := reg.Gather()
	require.NoError(t, err)

	var routeLabel string
	for _, mf := range families {
		if mf.GetName() == "iac_http_requests_total" {
			for _, m := range mf.GetMetric() {
				for _, lp := range m.GetLabel() {
					if lp.GetName() == "route" {
						routeLabel = lp.GetValue()
					}
				}
			}
		}
	}

	assert.Equal(t, "unknown", routeLabel,
		"route label must be 'unknown' when no route matches")
}
