package observability

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"testing/iotest"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
)

type traceRoundTripper func(*http.Request) (*http.Response, error)

func (transport traceRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return transport(req)
}

func TestOutboundHTTPTraceDoesNotExportPrivateURLs(t *testing.T) {
	for _, convention := range []string{"default", "http/dup"} {
		t.Run(convention, func(t *testing.T) {
			t.Setenv("OTEL_SEMCONV_STABILITY_OPT_IN", convention)
			exporter := tracetest.NewInMemoryExporter()
			provider := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(httpPrivacyProcessor{sdktrace.NewSimpleSpanProcessor(exporter)}))
			defer provider.Shutdown(t.Context())
			for index, scenario := range []struct{ method, path, failure string }{
				{"GET", "/mail?sid=private-mail-session", ""},
				{"PUT", "/private-object?signature=private-signature", "transport"},
				{"GET", "/mail?sid=private-mail-session", "read"},
			} {
				address := "https://upstream.invalid" + scenario.path
				originalErr := errors.New("request failed at " + address)
				transport := otelhttp.NewTransport(traceRoundTripper(func(req *http.Request) (*http.Response, error) {
					if req.URL.String() != address {
						t.Fatal("privacy filter changed the upstream URL")
					}
					var body io.Reader = strings.NewReader("ok")
					switch scenario.failure {
					case "transport":
						return nil, originalErr
					case "read":
						body = iotest.ErrReader(originalErr)
					}
					return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(body)}, nil
				}), otelhttp.WithTracerProvider(provider))
				req, err := http.NewRequestWithContext(t.Context(), scenario.method, address, nil)
				if err != nil {
					t.Fatal(err)
				}
				resp, err := transport.RoundTrip(req)
				if err == nil {
					_, err = io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
				}
				if (scenario.failure == "" && err != nil) || (scenario.failure != "" && !errors.Is(err, originalErr)) {
					t.Fatalf("unexpected caller error: %v", err)
				}
				spans := exporter.GetSpans()
				if len(spans) != index+1 {
					t.Fatal("outbound trace was lost")
				}
				span := spans[index]
				exported := fmt.Sprint(span.Attributes, span.Events, span.Status, span.Name)
				if strings.Contains(exported, "private-") || strings.Contains(exported, "http.url") || strings.Contains(exported, "url.full") || !strings.Contains(exported, "upstream.invalid") || span.Name != "HTTP "+scenario.method {
					t.Fatalf("unexpected exported metadata: %s", exported)
				}
				if (span.Status.Code == codes.Error) != (scenario.failure != "") {
					t.Fatalf("incorrect exported status: %v", span.Status)
				}
			}
		})
	}
}
