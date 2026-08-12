package observability

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"caiyun/internal/envutil"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"google.golang.org/grpc/credentials"
)

// TraceConfig configures optional OTLP/gRPC trace export. Collection is opt-in:
// a process emits spans only when OTEL_EXPORTER_OTLP_ENDPOINT or the traces
// endpoint is configured. This preserves local development and test behavior.
type TraceConfig struct {
	Enabled            bool
	Endpoint           string
	Insecure           bool
	TLSCertificatePath string
	SampleRatio        float64
	ServiceName        string
	ServiceVersion     string
	Environment        string
}

// LoadTraceConfig reads standard OTLP endpoint names and Caiyun's explicit
// sampling toggle. Secrets are never copied into resource attributes or spans.
func LoadTraceConfig(serviceName, serviceVersion string) (TraceConfig, error) {
	endpoint := strings.TrimSpace(envutil.String("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", ""))
	if endpoint == "" {
		endpoint = strings.TrimSpace(envutil.String("OTEL_EXPORTER_OTLP_ENDPOINT", ""))
	}
	enabled := envutil.Bool("OTEL_ENABLED", endpoint != "")
	environment := strings.TrimSpace(envutil.String("APP_ENV", "development"))
	ratio, err := traceSampleRatio(envutil.String("OTEL_SAMPLE_RATIO", ""), environment)
	if err != nil {
		return TraceConfig{}, err
	}
	config := TraceConfig{
		Enabled:            enabled,
		Endpoint:           endpoint,
		Insecure:           envutil.Bool("OTEL_EXPORTER_OTLP_INSECURE", false),
		TLSCertificatePath: traceTLSCertificatePath(),
		SampleRatio:        ratio,
		ServiceName:        strings.TrimSpace(serviceName),
		ServiceVersion:     serviceVersion,
		Environment:        environment,
	}
	if err := config.Validate(); err != nil {
		return TraceConfig{}, err
	}
	return config, nil
}

// Validate keeps transport policy at the configuration boundary. In
// particular, production traces must not silently fall back to clear-text
// OTLP/gRPC when a Collector endpoint is supplied.
func (config TraceConfig) Validate() error {
	if strings.TrimSpace(config.ServiceName) == "" {
		return fmt.Errorf("OTel service name 不能为空")
	}
	if !config.Enabled {
		return nil
	}
	endpoint := strings.TrimSpace(config.Endpoint)
	if endpoint == "" {
		return fmt.Errorf("OTEL_ENABLED=true 时必须设置 OTEL_EXPORTER_OTLP_ENDPOINT 或 OTEL_EXPORTER_OTLP_TRACES_ENDPOINT")
	}
	if config.SampleRatio < 0 || config.SampleRatio > 1 {
		return fmt.Errorf("OTEL_SAMPLE_RATIO 必须在 0 到 1 之间")
	}
	if strings.EqualFold(strings.TrimSpace(config.Environment), "production") && config.Insecure {
		return fmt.Errorf("生产环境禁止 OTEL_EXPORTER_OTLP_INSECURE=true")
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "http://") && !config.Insecure {
		return fmt.Errorf("http OTLP endpoint 必须设置 OTEL_EXPORTER_OTLP_INSECURE=true")
	}
	if strings.HasPrefix(strings.ToLower(endpoint), "https://") && config.Insecure {
		return fmt.Errorf("https OTLP endpoint 禁止设置 OTEL_EXPORTER_OTLP_INSECURE=true")
	}
	if config.Insecure && strings.TrimSpace(config.TLSCertificatePath) != "" {
		return fmt.Errorf("OTEL_EXPORTER_OTLP_INSECURE=true 时禁止设置 OTLP TLS CA 证书")
	}
	return nil
}

func traceTLSCertificatePath() string {
	path := strings.TrimSpace(envutil.String("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", ""))
	if path == "" {
		path = strings.TrimSpace(envutil.String("OTEL_EXPORTER_OTLP_CERTIFICATE", ""))
	}
	return path
}

func traceSampleRatio(raw, environment string) (float64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		if strings.EqualFold(environment, "production") {
			return 0.10, nil
		}
		return 1, nil
	}
	ratio, err := strconv.ParseFloat(raw, 64)
	if err != nil || ratio < 0 || ratio > 1 {
		return 0, fmt.Errorf("OTEL_SAMPLE_RATIO 必须在 0 到 1 之间")
	}
	return ratio, nil
}

// StartTracing installs a provider, propagation, batching and resource
// attributes for one process. The returned shutdown function flushes pending
// spans under the caller-provided bounded context.
func StartTracing(ctx context.Context, config TraceConfig) (func(context.Context) error, error) {
	if err := config.Validate(); err != nil {
		return nil, err
	}
	if !config.Enabled {
		return func(context.Context) error { return nil }, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	options := make([]otlptracegrpc.Option, 0, 2)
	if strings.Contains(config.Endpoint, "://") {
		options = append(options, otlptracegrpc.WithEndpointURL(config.Endpoint))
	} else {
		options = append(options, otlptracegrpc.WithEndpoint(config.Endpoint))
	}
	if config.Insecure {
		options = append(options, otlptracegrpc.WithInsecure())
	}
	if config.TLSCertificatePath != "" {
		transportCredentials, err := credentials.NewClientTLSFromFile(config.TLSCertificatePath, "")
		if err != nil {
			return nil, fmt.Errorf("加载 OTLP TLS CA 证书: %w", err)
		}
		options = append(options, otlptracegrpc.WithTLSCredentials(transportCredentials))
	}
	exporter, err := otlptracegrpc.New(ctx, options...)
	if err != nil {
		return nil, fmt.Errorf("初始化 OTLP trace exporter: %w", err)
	}
	res := resource.NewWithAttributes(
		"",
		attribute.String("service.name", config.ServiceName),
		attribute.String("service.version", config.ServiceVersion),
		attribute.String("deployment.environment.name", config.Environment),
	)
	provider := sdktrace.NewTracerProvider(
		sdktrace.WithSampler(sdktrace.ParentBased(sdktrace.TraceIDRatioBased(config.SampleRatio))),
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(provider)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
	return provider.Shutdown, nil
}
