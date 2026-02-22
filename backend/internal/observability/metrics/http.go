package metrics

import (
	"log"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
)

// HTTPMetricsMiddleware returns a Gin middleware that records HTTP request
// metrics on the provided Prometheus registry:
//
//   - iac_http_requests_total       (CounterVec)   — method, route, status
//   - iac_http_request_duration_seconds (HistogramVec) — method, route, status
//   - iac_http_requests_in_flight   (Gauge)        — no labels
//
// The middleware is safe to use even if reg is nil (it becomes a no-op).
// The entire handler body is wrapped in defer/recover per constraint 6.1.
// The route label uses c.FullPath() (route template) to prevent cardinality
// explosion (constraint 6.6).
func HTTPMetricsMiddleware(reg *prometheus.Registry) gin.HandlerFunc {
	// If the registry is nil, return a no-op middleware to avoid panics.
	if reg == nil {
		return func(c *gin.Context) {
			c.Next()
		}
	}

	requestsTotal := prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_http_requests_total",
			Help: "Total number of HTTP requests processed.",
		},
		[]string{"method", "route", "status"},
	)

	requestDuration := prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iac_http_request_duration_seconds",
			Help:    "HTTP request duration in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"method", "route", "status"},
	)

	requestsInFlight := prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "iac_http_requests_in_flight",
			Help: "Number of HTTP requests currently being processed.",
		},
	)

	reg.MustRegister(requestsTotal, requestDuration, requestsInFlight)

	return func(c *gin.Context) {
		// Constraint 6.1: panic recovery — never let metric recording crash the server.
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in HTTPMetricsMiddleware: %v", r)
			}
		}()

		requestsInFlight.Inc()
		defer requestsInFlight.Dec()
		start := time.Now()

		c.Next()

		elapsed := time.Since(start).Seconds()

		route := c.FullPath()
		if route == "" {
			route = "unknown"
		}
		method := c.Request.Method
		status := strconv.Itoa(c.Writer.Status())

		requestsTotal.WithLabelValues(method, route, status).Inc()
		requestDuration.WithLabelValues(method, route, status).Observe(elapsed)
	}
}
