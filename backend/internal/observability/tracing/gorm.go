package tracing

import (
	"log"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

// dbTracer is the tracer used by the GORM tracing callbacks.
// Under the noop TracerProvider this returns a noop tracer with zero overhead.
var dbTracer = otel.Tracer("iac-backend/db")

// RegisterGORMTracing registers Before/After callbacks for Create, Query,
// Update, and Delete operations on the provided *gorm.DB. The callbacks
// create OpenTelemetry spans that capture database operation details.
//
// Callbacks are read-only (they never execute SQL) and each is wrapped in
// defer/recover to satisfy constraint 6.4. Callback names use the "trace:"
// prefix to avoid collision with the existing "obs:" prefix metrics callbacks.
//
// If db is nil the call is a no-op.
func RegisterGORMTracing(db *gorm.DB) {
	if db == nil {
		return
	}

	// --- Create ---
	db.Callback().Create().Before("gorm:create").Register("trace:before_create", makeBeforeCallback("create"))
	db.Callback().Create().After("gorm:create").Register("trace:after_create", makeAfterTracingCallback("create"))

	// --- Query ---
	db.Callback().Query().Before("gorm:query").Register("trace:before_query", makeBeforeCallback("query"))
	db.Callback().Query().After("gorm:query").Register("trace:after_query", makeAfterTracingCallback("query"))

	// --- Update ---
	db.Callback().Update().Before("gorm:update").Register("trace:before_update", makeBeforeCallback("update"))
	db.Callback().Update().After("gorm:update").Register("trace:after_update", makeAfterTracingCallback("update"))

	// --- Delete ---
	db.Callback().Delete().Before("gorm:delete").Register("trace:before_delete", makeBeforeCallback("delete"))
	db.Callback().Delete().After("gorm:delete").Register("trace:after_delete", makeAfterTracingCallback("delete"))
}

// makeBeforeCallback returns a GORM callback function that extracts the parent
// span from tx.Statement.Context, creates a child span named "db.{operation}",
// and writes the new context back to tx.Statement.Context.
func makeBeforeCallback(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[tracing] recovered from panic in trace:before_%s: %v", operation, r)
			}
		}()

		if tx == nil || tx.Statement == nil {
			return
		}

		ctx := tx.Statement.Context
		ctx, _ = dbTracer.Start(ctx, "db."+operation, trace.WithSpanKind(trace.SpanKindClient))
		tx.Statement.Context = ctx
	}
}

// makeAfterTracingCallback returns a GORM callback function that extracts the
// span from tx.Statement.Context, sets standard DB attributes, records any
// error, and ends the span.
func makeAfterTracingCallback(operation string) func(*gorm.DB) {
	return func(tx *gorm.DB) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[tracing] recovered from panic in trace:after_%s: %v", operation, r)
			}
		}()

		if tx == nil || tx.Statement == nil {
			return
		}

		span := trace.SpanFromContext(tx.Statement.Context)
		if !span.IsRecording() {
			return
		}

		span.SetAttributes(
			semconv.DBSystemPostgreSQL,
			semconv.DBOperationName(operation),
		)

		if tx.Error != nil {
			span.SetStatus(codes.Error, tx.Error.Error())
			span.RecordError(tx.Error)
		}

		span.End()
	}
}
