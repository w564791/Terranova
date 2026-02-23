package tracing

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"iac-platform/internal/observability/metrics"
)

// TestRegisterGORMTracing_NilDB verifies that passing nil does not panic.
func TestRegisterGORMTracing_NilDB(t *testing.T) {
	assert.NotPanics(t, func() {
		RegisterGORMTracing(nil)
	}, "RegisterGORMTracing(nil) must not panic")
}

// TestRegisterGORMTracing_ValidDB verifies that calling RegisterGORMTracing
// with a valid in-memory database does not panic and the expected callbacks
// are registered.
func TestRegisterGORMTracing_ValidDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "opening in-memory sqlite must succeed")

	assert.NotPanics(t, func() {
		RegisterGORMTracing(db)
	}, "RegisterGORMTracing must not panic with a valid db")

	// Verify all trace callbacks are registered.
	for _, tc := range []struct {
		op   string
		kind string
	}{
		{"create", "before"},
		{"create", "after"},
		{"query", "before"},
		{"query", "after"},
		{"update", "before"},
		{"update", "after"},
		{"delete", "before"},
		{"delete", "after"},
	} {
		name := "trace:" + tc.kind + "_" + tc.op
		var fn interface{}
		switch tc.op {
		case "create":
			fn = db.Callback().Create().Get(name)
		case "query":
			fn = db.Callback().Query().Get(name)
		case "update":
			fn = db.Callback().Update().Get(name)
		case "delete":
			fn = db.Callback().Delete().Get(name)
		}
		assert.NotNilf(t, fn, "callback %s must be registered", name)
	}
}

// TestRegisterGORMTracing_NoConflictWithMetrics verifies that both metrics
// (obs:*) and tracing (trace:*) callbacks can coexist on the same *gorm.DB
// without name conflicts.
func TestRegisterGORMTracing_NoConflictWithMetrics(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "opening in-memory sqlite must succeed")

	// Register metrics callbacks first (obs:* prefix).
	assert.NotPanics(t, func() {
		metrics.RegisterGORMCallbacks(db)
	}, "RegisterGORMCallbacks must not panic")

	// Register tracing callbacks second (trace:* prefix).
	assert.NotPanics(t, func() {
		RegisterGORMTracing(db)
	}, "RegisterGORMTracing must not panic after RegisterGORMCallbacks")

	// Verify both sets of callbacks are present.
	assert.NotNil(t, db.Callback().Query().Get("obs:before_query"),
		"obs:before_query must still be registered")
	assert.NotNil(t, db.Callback().Query().Get("trace:before_query"),
		"trace:before_query must be registered")
	assert.NotNil(t, db.Callback().Query().Get("obs:after_query"),
		"obs:after_query must still be registered")
	assert.NotNil(t, db.Callback().Query().Get("trace:after_query"),
		"trace:after_query must be registered")
}

// TestRegisterGORMTracing_NoopProvider verifies that when the noop tracer
// provider is active (no OTEL_EXPORTER_OTLP_ENDPOINT set), the callbacks
// register and execute without errors. Under noop, tracer.Start() returns a
// noop span with zero overhead.
func TestRegisterGORMTracing_NoopProvider(t *testing.T) {
	// Ensure noop provider by unsetting the endpoint.
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "")

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "opening in-memory sqlite must succeed")

	assert.NotPanics(t, func() {
		RegisterGORMTracing(db)
	}, "RegisterGORMTracing must not panic under noop provider")

	// Execute a query to trigger the callbacks — they should be silent no-ops.
	type Dummy struct {
		ID   uint
		Name string
	}
	_ = db.AutoMigrate(&Dummy{})

	assert.NotPanics(t, func() {
		db.Create(&Dummy{Name: "test"})
	}, "Create with tracing callbacks must not panic under noop provider")

	assert.NotPanics(t, func() {
		var d Dummy
		db.First(&d)
	}, "Query with tracing callbacks must not panic under noop provider")

	assert.NotPanics(t, func() {
		db.Model(&Dummy{}).Where("id = ?", 1).Update("name", "updated")
	}, "Update with tracing callbacks must not panic under noop provider")

	assert.NotPanics(t, func() {
		db.Where("id = ?", 1).Delete(&Dummy{})
	}, "Delete with tracing callbacks must not panic under noop provider")
}
