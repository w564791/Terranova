package metrics

import (
	"database/sql"
	"log"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/gorm"
)

// Package-level metric variables. These are set by RegisterDBMetrics and
// referenced by GORM callbacks and the DB stats collector. When nil (i.e.
// before RegisterDBMetrics is called), callers simply skip recording.
var (
	dbQueriesTotal        *prometheus.CounterVec
	dbQueryDuration       *prometheus.HistogramVec
	dbConnectionsOpen     prometheus.Gauge
	dbConnectionsMax      prometheus.Gauge
	dbConnectionsWaiting  prometheus.Gauge
)

// RegisterDBMetrics registers database-related Prometheus metrics on the
// provided registry. If reg is nil the call is a no-op.
func RegisterDBMetrics(reg *prometheus.Registry) {
	if reg == nil {
		return
	}

	dbQueriesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "iac_db_queries_total",
			Help: "Total number of database queries executed.",
		},
		[]string{"operation"},
	)

	dbQueryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "iac_db_query_duration_seconds",
			Help:    "Duration of database queries in seconds.",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	dbConnectionsOpen = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iac_db_connections_open",
		Help: "Number of open database connections.",
	})

	dbConnectionsMax = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iac_db_connections_max",
		Help: "Maximum number of open database connections.",
	})

	dbConnectionsWaiting = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "iac_db_connections_waiting",
		Help: "Number of database connections waited for.",
	})

	reg.MustRegister(
		dbQueriesTotal,
		dbQueryDuration,
		dbConnectionsOpen,
		dbConnectionsMax,
		dbConnectionsWaiting,
	)
}

// RegisterGORMCallbacks registers observability Before/After callbacks for
// Create, Query, Update, and Delete operations on the provided *gorm.DB.
// Callbacks are read-only (they never execute SQL) and each is wrapped in
// defer/recover to satisfy constraint 6.4.
// If db is nil the call is a no-op.
func RegisterGORMCallbacks(db *gorm.DB) {
	if db == nil {
		return
	}

	// --- Create ---
	db.Callback().Create().Before("gorm:create").Register("obs:before_create", func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in obs:before_create: %v", r)
			}
		}()
		tx.InstanceSet("obs:start_time", time.Now())
	})
	db.Callback().Create().After("gorm:create").Register("obs:after_create", makeAfterCallback("create"))

	// --- Query ---
	db.Callback().Query().Before("gorm:query").Register("obs:before_query", func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in obs:before_query: %v", r)
			}
		}()
		tx.InstanceSet("obs:start_time", time.Now())
	})
	db.Callback().Query().After("gorm:query").Register("obs:after_query", makeAfterCallback("query"))

	// --- Update ---
	db.Callback().Update().Before("gorm:update").Register("obs:before_update", func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in obs:before_update: %v", r)
			}
		}()
		tx.InstanceSet("obs:start_time", time.Now())
	})
	db.Callback().Update().After("gorm:update").Register("obs:after_update", makeAfterCallback("update"))

	// --- Delete ---
	db.Callback().Delete().Before("gorm:delete").Register("obs:before_delete", func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in obs:before_delete: %v", r)
			}
		}()
		tx.InstanceSet("obs:start_time", time.Now())
	})
	db.Callback().Delete().After("gorm:delete").Register("obs:after_delete", makeAfterCallback("delete"))
}

// makeAfterCallback returns a GORM callback function that records query
// duration and counter metrics for the given operation. The callback is
// wrapped in defer/recover (constraint 6.4) and silently skips recording
// when the metric variables are nil (metrics not yet registered).
func makeAfterCallback(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[metrics] recovered from panic in obs:after_%s: %v", operation, r)
			}
		}()

		recordDBMetric(tx, operation)
	}
}

// recordDBMetric extracts the start time from the GORM instance context and
// records the query count and duration metrics. It is safe to call with a nil
// tx or when metrics have not been registered yet.
func recordDBMetric(tx *gorm.DB, operation string) {
	if tx == nil {
		return
	}

	v, ok := tx.InstanceGet("obs:start_time")
	if !ok {
		return
	}

	startTime, ok := v.(time.Time)
	if !ok {
		return
	}

	duration := time.Since(startTime).Seconds()

	if dbQueriesTotal != nil {
		dbQueriesTotal.WithLabelValues(operation).Inc()
	}
	if dbQueryDuration != nil {
		dbQueryDuration.WithLabelValues(operation).Observe(duration)
	}
}

// StartDBStatsCollector launches a background goroutine that periodically
// reads sql.DBStats and updates the connection gauge metrics. If sqlDB is nil
// the call is a no-op.
func StartDBStatsCollector(sqlDB *sql.DB, interval time.Duration) {
	if sqlDB == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			func() {
				defer func() {
					if r := recover(); r != nil {
						log.Printf("[metrics] recovered from panic in DBStatsCollector: %v", r)
					}
				}()

				stats := sqlDB.Stats()

				if dbConnectionsOpen != nil {
					dbConnectionsOpen.Set(float64(stats.OpenConnections))
				}
				if dbConnectionsMax != nil {
					dbConnectionsMax.Set(float64(stats.MaxOpenConnections))
				}
				if dbConnectionsWaiting != nil {
					dbConnectionsWaiting.Set(float64(stats.WaitCount))
				}
			}()
		}
	}()
}
