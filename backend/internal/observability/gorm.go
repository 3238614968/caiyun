package observability

import (
	"context"
	"errors"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

const gormSpanKey = "caiyun:otel:span"

// InstallGORMTracing adds callback spans at the GORM infrastructure boundary.
// SQL text, bound values and model payloads stay out of attributes so database
// credentials and user data are not exported with traces.
func InstallGORMTracing(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("GORM database is nil")
	}
	if err := db.Callback().Create().Before("gorm:create").Register("caiyun:otel:before_create", startGORMSpan("create")); err != nil {
		return fmt.Errorf("register GORM create tracing start: %w", err)
	}
	if err := db.Callback().Create().After("gorm:create").Register("caiyun:otel:after_create", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM create tracing finish: %w", err)
	}
	if err := db.Callback().Query().Before("gorm:query").Register("caiyun:otel:before_query", startGORMSpan("query")); err != nil {
		return fmt.Errorf("register GORM query tracing start: %w", err)
	}
	if err := db.Callback().Query().After("gorm:query").Register("caiyun:otel:after_query", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM query tracing finish: %w", err)
	}
	if err := db.Callback().Update().Before("gorm:update").Register("caiyun:otel:before_update", startGORMSpan("update")); err != nil {
		return fmt.Errorf("register GORM update tracing start: %w", err)
	}
	if err := db.Callback().Update().After("gorm:update").Register("caiyun:otel:after_update", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM update tracing finish: %w", err)
	}
	if err := db.Callback().Delete().Before("gorm:delete").Register("caiyun:otel:before_delete", startGORMSpan("delete")); err != nil {
		return fmt.Errorf("register GORM delete tracing start: %w", err)
	}
	if err := db.Callback().Delete().After("gorm:delete").Register("caiyun:otel:after_delete", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM delete tracing finish: %w", err)
	}
	if err := db.Callback().Row().Before("gorm:row").Register("caiyun:otel:before_row", startGORMSpan("row")); err != nil {
		return fmt.Errorf("register GORM row tracing start: %w", err)
	}
	if err := db.Callback().Row().After("gorm:row").Register("caiyun:otel:after_row", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM row tracing finish: %w", err)
	}
	if err := db.Callback().Raw().Before("gorm:raw").Register("caiyun:otel:before_raw", startGORMSpan("raw")); err != nil {
		return fmt.Errorf("register GORM raw tracing start: %w", err)
	}
	if err := db.Callback().Raw().After("gorm:raw").Register("caiyun:otel:after_raw", finishGORMSpan); err != nil {
		return fmt.Errorf("register GORM raw tracing finish: %w", err)
	}
	return nil
}

func startGORMSpan(operation string) func(*gorm.DB) {
	return func(db *gorm.DB) {
		if db == nil || db.Statement == nil {
			return
		}
		ctx := db.Statement.Context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, span := otel.Tracer("caiyun/gorm").Start(ctx, "gorm."+operation, trace.WithSpanKind(trace.SpanKindClient))
		db.Statement.Context = ctx
		if db.Statement.Table != "" {
			span.SetAttributes(attribute.String("db.collection.name", db.Statement.Table))
		}
		db.InstanceSet(gormSpanKey, span)
	}
}

func finishGORMSpan(db *gorm.DB) {
	if db == nil {
		return
	}
	value, ok := db.InstanceGet(gormSpanKey)
	if !ok {
		return
	}
	span, ok := value.(trace.Span)
	if !ok || span == nil {
		return
	}
	if db.Error != nil && !errors.Is(db.Error, gorm.ErrRecordNotFound) {
		span.RecordError(db.Error)
		span.SetStatus(codes.Error, db.Error.Error())
	}
	span.End()
}
