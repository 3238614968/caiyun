package cache

import (
	"context"
	"errors"

	"github.com/go-redis/redis/v8"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type redisSpanKey struct{}

// redisTraceHook instruments every go-redis command at the client boundary.
// It exports only command names and pipeline sizes; keys, payloads and secrets
// stay out of trace attributes.
type redisTraceHook struct{}

func (redisTraceHook) BeforeProcess(ctx context.Context, cmd redis.Cmder) (context.Context, error) {
	ctx, span := otel.Tracer("caiyun/redis").Start(ctx, "redis."+cmd.Name(), trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.String("db.operation.name", cmd.Name()))
	return context.WithValue(ctx, redisSpanKey{}, span), nil
}

func (redisTraceHook) AfterProcess(ctx context.Context, cmd redis.Cmder) error {
	finishRedisSpan(ctx, cmd.Err())
	return nil
}

func (redisTraceHook) BeforeProcessPipeline(ctx context.Context, cmds []redis.Cmder) (context.Context, error) {
	ctx, span := otel.Tracer("caiyun/redis").Start(ctx, "redis.pipeline", trace.WithSpanKind(trace.SpanKindClient))
	span.SetAttributes(attribute.Int("db.redis.pipeline_length", len(cmds)))
	return context.WithValue(ctx, redisSpanKey{}, span), nil
}

func (redisTraceHook) AfterProcessPipeline(ctx context.Context, cmds []redis.Cmder) error {
	var err error
	for _, cmd := range cmds {
		if commandErr := cmd.Err(); commandErr != nil && !errors.Is(commandErr, redis.Nil) {
			err = commandErr
			break
		}
	}
	finishRedisSpan(ctx, err)
	return nil
}

func finishRedisSpan(ctx context.Context, err error) {
	span, _ := ctx.Value(redisSpanKey{}).(trace.Span)
	if span == nil {
		return
	}
	if err != nil && !errors.Is(err, redis.Nil) {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
	span.End()
}
