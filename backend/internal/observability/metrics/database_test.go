package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// resetDBMetrics clears package-level DB metric variables so each test starts
// from a clean state.
func resetDBMetrics() {
	dbQueriesTotal = nil
	dbQueryDuration = nil
	dbConnectionsOpen = nil
	dbConnectionsMax = nil
	dbConnectionsWaiting = nil
}

// TestRegisterDBMetrics_ValidRegistry verifies that calling RegisterDBMetrics
// with a valid registry does not panic and the expected metrics are present.
func TestRegisterDBMetrics_ValidRegistry(t *testing.T) {
	resetDBMetrics()
	defer resetDBMetrics()

	reg := prometheus.NewRegistry()

	assert.NotPanics(t, func() {
		RegisterDBMetrics(reg)
	}, "RegisterDBMetrics must not panic with a valid registry")

	families, err := reg.Gather()
	require.NoError(t, err)

	// The counter and histogram won't appear in Gather() until they have been
	// observed at least once. Instead, verify the package-level variables are
	// non-nil after registration.
	assert.NotNil(t, dbQueriesTotal, "dbQueriesTotal must be set after RegisterDBMetrics")
	assert.NotNil(t, dbQueryDuration, "dbQueryDuration must be set after RegisterDBMetrics")
	assert.NotNil(t, dbConnectionsOpen, "dbConnectionsOpen must be set after RegisterDBMetrics")
	assert.NotNil(t, dbConnectionsMax, "dbConnectionsMax must be set after RegisterDBMetrics")
	assert.NotNil(t, dbConnectionsWaiting, "dbConnectionsWaiting must be set after RegisterDBMetrics")

	// Force a value so the metric appears in Gather output.
	dbQueriesTotal.WithLabelValues("query").Inc()

	families, err = reg.Gather()
	require.NoError(t, err)

	found := false
	for _, mf := range families {
		if mf.GetName() == "iac_db_queries_total" {
			found = true
			break
		}
	}
	assert.True(t, found, "Gather() must contain iac_db_queries_total after Inc()")
}

// TestRegisterDBMetrics_NilRegistry verifies that passing nil does not panic.
func TestRegisterDBMetrics_NilRegistry(t *testing.T) {
	resetDBMetrics()
	defer resetDBMetrics()

	assert.NotPanics(t, func() {
		RegisterDBMetrics(nil)
	}, "RegisterDBMetrics(nil) must not panic")
}

// TestRegisterGORMCallbacks_NilDB verifies that passing nil does not panic.
func TestRegisterGORMCallbacks_NilDB(t *testing.T) {
	assert.NotPanics(t, func() {
		RegisterGORMCallbacks(nil)
	}, "RegisterGORMCallbacks(nil) must not panic")
}

// TestRegisterGORMCallbacks_CallbacksRegistered verifies that after calling
// RegisterGORMCallbacks the expected Before/After callbacks exist on the
// GORM callback chain.
func TestRegisterGORMCallbacks_CallbacksRegistered(t *testing.T) {
	resetDBMetrics()
	defer resetDBMetrics()

	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err, "opening in-memory sqlite must succeed")

	assert.NotPanics(t, func() {
		RegisterGORMCallbacks(db)
	}, "RegisterGORMCallbacks must not panic")

	// Verify callbacks are registered by checking that Get returns a non-nil function.
	fn := db.Callback().Query().Get("obs:before_query")
	assert.NotNil(t, fn, "obs:before_query callback must be registered")

	fn = db.Callback().Query().Get("obs:after_query")
	assert.NotNil(t, fn, "obs:after_query callback must be registered")

	fn = db.Callback().Create().Get("obs:before_create")
	assert.NotNil(t, fn, "obs:before_create callback must be registered")

	fn = db.Callback().Create().Get("obs:after_create")
	assert.NotNil(t, fn, "obs:after_create callback must be registered")

	fn = db.Callback().Update().Get("obs:before_update")
	assert.NotNil(t, fn, "obs:before_update callback must be registered")

	fn = db.Callback().Update().Get("obs:after_update")
	assert.NotNil(t, fn, "obs:after_update callback must be registered")

	fn = db.Callback().Delete().Get("obs:before_delete")
	assert.NotNil(t, fn, "obs:before_delete callback must be registered")

	fn = db.Callback().Delete().Get("obs:after_delete")
	assert.NotNil(t, fn, "obs:after_delete callback must be registered")
}

// TestRecordDBMetric_NilTx verifies that recordDBMetric does not panic when
// passed a nil *gorm.DB.
func TestRecordDBMetric_NilTx(t *testing.T) {
	resetDBMetrics()
	defer resetDBMetrics()

	assert.NotPanics(t, func() {
		recordDBMetric(nil, "query")
	}, "recordDBMetric(nil, ...) must not panic")
}

// TestStartDBStatsCollector_NilDB verifies that passing nil does not panic.
func TestStartDBStatsCollector_NilDB(t *testing.T) {
	assert.NotPanics(t, func() {
		StartDBStatsCollector(nil, 15)
	}, "StartDBStatsCollector(nil, ...) must not panic")
}
