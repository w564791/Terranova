package services

import (
	"os"
	"testing"

	"iac-platform/internal/observability/metrics"

	"github.com/prometheus/client_golang/prometheus"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func TestAcquireTask_WithGORMCallbacks(t *testing.T) {
	// Try to connect to local PostgreSQL.
	// Use DATABASE_URL env var, or fall back to a default local connection.
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "host=localhost user=postgres password=postgres dbname=iac_platform port=5432 sslmode=disable"
	}

	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skip("PostgreSQL not available, skipping SKIP LOCKED regression test:", err)
	}

	// Verify the connection is actually reachable.
	sqlDB, err := db.DB()
	if err != nil {
		t.Skip("Failed to get sql.DB:", err)
	}
	if err := sqlDB.Ping(); err != nil {
		t.Skip("PostgreSQL not reachable:", err)
	}
	defer sqlDB.Close()

	// Register observability GORM callbacks (same as what database.go does).
	reg := prometheus.NewRegistry()
	metrics.RegisterDBMetrics(reg)
	metrics.RegisterGORMCallbacks(db)

	// Create the service and attempt to acquire a task.
	// The key assertion is that no panic occurs when GORM callbacks
	// are registered alongside a SKIP LOCKED query.
	svc := NewTaskLockService(db)
	_, err = svc.AcquireTask("test-agent", 60)

	// We accept any of these outcomes:
	//   - "no available tasks" (normal when table is empty or no pending tasks)
	//   - A DB error if the table does not exist
	// What we do NOT accept: a panic from the GORM callbacks.
	if err != nil {
		t.Logf("AcquireTask returned error (expected): %v", err)
	} else {
		t.Log("AcquireTask succeeded (task was available)")
	}
}
