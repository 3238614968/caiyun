package observability

import "testing"

func TestLoadTraceConfigEnforcesProductionTransportPolicy(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "collector.telemetry.svc:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")

	if _, err := LoadTraceConfig("caiyun-api", "test"); err == nil {
		t.Fatal("LoadTraceConfig accepted clear-text OTLP in production")
	}
}

func TestLoadTraceConfigAllowsDevelopmentCollectorAndReadsSampling(t *testing.T) {
	t.Setenv("APP_ENV", "development")
	t.Setenv("OTEL_ENABLED", "true")
	t.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "http://collector.local:4317")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", "")
	t.Setenv("OTEL_EXPORTER_OTLP_INSECURE", "true")
	t.Setenv("OTEL_SAMPLE_RATIO", "0.25")
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", "")

	config, err := LoadTraceConfig("caiyun-api", "test")
	if err != nil {
		t.Fatalf("LoadTraceConfig returned error: %v", err)
	}
	if !config.Enabled || !config.Insecure || config.SampleRatio != 0.25 {
		t.Fatalf("unexpected trace config: %#v", config)
	}
}

func TestTraceTLSCertificatePathPrefersTracesSpecificSetting(t *testing.T) {
	t.Setenv("OTEL_EXPORTER_OTLP_CERTIFICATE", "/global/ca.pem")
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "/traces/ca.pem")
	if actual := traceTLSCertificatePath(); actual != "/traces/ca.pem" {
		t.Fatalf("traceTLSCertificatePath() = %q", actual)
	}
	t.Setenv("OTEL_EXPORTER_OTLP_TRACES_CERTIFICATE", "")
	if actual := traceTLSCertificatePath(); actual != "/global/ca.pem" {
		t.Fatalf("traceTLSCertificatePath fallback = %q", actual)
	}
}

func TestTraceConfigValidateRejectsMismatchedURLSecurity(t *testing.T) {
	tests := []TraceConfig{
		{Enabled: true, Endpoint: "http://collector.local:4317", SampleRatio: 1, ServiceName: "caiyun-api", Environment: "development"},
		{Enabled: true, Endpoint: "https://collector.local:4317", Insecure: true, SampleRatio: 1, ServiceName: "caiyun-api", Environment: "development"},
		{Enabled: true, Endpoint: "collector.local:4317", SampleRatio: 2, ServiceName: "caiyun-api", Environment: "development"},
		{Enabled: true, Endpoint: "collector.local:4317", SampleRatio: 1, Environment: "development"},
		{Enabled: true, Endpoint: "collector.local:4317", Insecure: true, TLSCertificatePath: "/ca.pem", SampleRatio: 1, ServiceName: "caiyun-api", Environment: "development"},
	}
	for _, config := range tests {
		if err := config.Validate(); err == nil {
			t.Fatalf("Validate accepted invalid trace config: %#v", config)
		}
	}
}
