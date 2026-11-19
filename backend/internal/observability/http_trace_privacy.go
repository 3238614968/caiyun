package observability

import (
	"slices"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/attribute"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/trace"
)

type httpPrivacyProcessor struct{ sdktrace.SpanProcessor }

func (processor httpPrivacyProcessor) OnEnd(span sdktrace.ReadOnlySpan) {
	if span.SpanKind() == trace.SpanKindClient && span.InstrumentationScope().Name == otelhttp.ScopeName {
		span = outboundHTTPSpan{span}
	}
	processor.SpanProcessor.OnEnd(span)
}

type outboundHTTPSpan struct{ sdktrace.ReadOnlySpan }

func safeHTTPAttributes(attributes []attribute.KeyValue) []attribute.KeyValue {
	return slices.DeleteFunc(slices.Clone(attributes), func(attr attribute.KeyValue) bool {
		switch attr.Key {
		case "http.url", "http.target", "exception.message", "exception.stacktrace":
			return true
		}
		return strings.HasPrefix(string(attr.Key), "url.")
	})
}

func (span outboundHTTPSpan) Attributes() []attribute.KeyValue {
	return safeHTTPAttributes(span.ReadOnlySpan.Attributes())
}

func (span outboundHTTPSpan) Events() []sdktrace.Event {
	events := slices.Clone(span.ReadOnlySpan.Events())
	for index := range events {
		events[index].Attributes = safeHTTPAttributes(events[index].Attributes)
	}
	return events
}

func (span outboundHTTPSpan) Status() sdktrace.Status {
	return sdktrace.Status{Code: span.ReadOnlySpan.Status().Code}
}
