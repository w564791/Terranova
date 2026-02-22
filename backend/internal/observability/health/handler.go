package health

import (
	"net/http"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// startupReady is an atomic flag that is set to true once all services have
// been initialised. It is consumed by the /health/startup endpoint.
var startupReady atomic.Bool

// MarkStartupReady should be called from main.go after all services have been
// initialised. It flips the startupReady flag so that /health/startup begins
// returning 200.
func MarkStartupReady() {
	startupReady.Store(true)
}

// dbCheckTimeout is the default timeout used for database connectivity checks.
const dbCheckTimeout = 2 * time.Second

// RegisterRoutes registers the three-tier health-check endpoints on the
// provided gin.Engine:
//
//	GET /health        – backward-compatible, always returns {"status":"ok"}
//	GET /health/live   – liveness probe, always 200
//	GET /health/ready  – readiness probe, checks DB connectivity
//	GET /health/startup – startup probe, checks DB + startupReady flag
func RegisterRoutes(r *gin.Engine, db *gorm.DB) {
	// 约束 6.2: /health 返回 {"status":"ok"} 不变
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Liveness – if the process can serve HTTP, it is alive.
	r.GET("/health/live", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	})

	// Readiness – the service is ready to accept traffic when the DB is reachable.
	r.GET("/health/ready", func(c *gin.Context) {
		if err := CheckDatabase(db, dbCheckTimeout); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"checks": gin.H{"database": err.Error()},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"checks": gin.H{"database": "ok"},
		})
	})

	// Startup – the service has completed its initialisation sequence.
	r.GET("/health/startup", func(c *gin.Context) {
		if !startupReady.Load() {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"checks": gin.H{"startup": "not ready"},
			})
			return
		}
		if err := CheckDatabase(db, dbCheckTimeout); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"checks": gin.H{"database": err.Error()},
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"status": "healthy",
			"checks": gin.H{
				"startup":  "ready",
				"database": "ok",
			},
		})
	})
}
