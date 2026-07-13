package http

import (
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func newRetryTestClient() *Client {
	client := NewClient()
	client.retryDelay = 0
	return client
}

func closeTestResponse(t *testing.T, response *stdhttp.Response) {
	t.Helper()
	if response == nil || response.Body == nil {
		return
	}
	_, _ = io.Copy(io.Discard, response.Body)
	if err := response.Body.Close(); err != nil {
		t.Fatalf("close response body: %v", err)
	}
}

func TestSafeMethodsRetryByDefault(t *testing.T) {
	for _, method := range []string{stdhttp.MethodGet, stdhttp.MethodHead, stdhttp.MethodOptions} {
		t.Run(method, func(t *testing.T) {
			var attempts int32
			server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
				current := atomic.AddInt32(&attempts, 1)
				if current < 3 {
					writer.WriteHeader(stdhttp.StatusBadGateway)
					return
				}
				writer.WriteHeader(stdhttp.StatusOK)
			}))
			defer server.Close()

			response, err := newRetryTestClient().Request(method, server.URL, nil, nil)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer closeTestResponse(t, response)

			if response.StatusCode != stdhttp.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, stdhttp.StatusOK)
			}
			if got := atomic.LoadInt32(&attempts); got != 3 {
				t.Fatalf("attempts = %d, want 3", got)
			}
		})
	}
}

func TestUnsafeMethodsDoNotRetryByDefault(t *testing.T) {
	for _, method := range []string{stdhttp.MethodPost, stdhttp.MethodPatch, stdhttp.MethodPut, stdhttp.MethodDelete} {
		t.Run(method, func(t *testing.T) {
			var attempts int32
			server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
				if atomic.AddInt32(&attempts, 1) == 1 {
					writer.WriteHeader(stdhttp.StatusServiceUnavailable)
					return
				}
				writer.WriteHeader(stdhttp.StatusOK)
			}))
			defer server.Close()

			response, err := newRetryTestClient().Request(method, server.URL, nil, map[string]string{"value": "once"})
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer closeTestResponse(t, response)

			if response.StatusCode != stdhttp.StatusServiceUnavailable {
				t.Fatalf("status = %d, want %d", response.StatusCode, stdhttp.StatusServiceUnavailable)
			}
			if got := atomic.LoadInt32(&attempts); got != 1 {
				t.Fatalf("attempts = %d, want 1", got)
			}
		})
	}
}

func TestUnsafeMethodRetriesWithIdempotencyKeyAndReplaysBody(t *testing.T) {
	const (
		idempotencyKey = "operation-123"
		requestBody    = `{"value":"repeatable"}`
	)

	var attempts int32
	var mu sync.Mutex
	var keys []string
	var bodies []string
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}
		mu.Lock()
		keys = append(keys, request.Header.Get("Idempotency-Key"))
		bodies = append(bodies, string(body))
		mu.Unlock()

		if atomic.AddInt32(&attempts, 1) < 3 {
			writer.WriteHeader(stdhttp.StatusBadGateway)
			return
		}
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	defer server.Close()

	response, err := newRetryTestClient().RequestWithOptions(
		stdhttp.MethodPost,
		server.URL,
		nil,
		requestBody,
		WithIdempotencyKey(idempotencyKey),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer closeTestResponse(t, response)

	if response.StatusCode != stdhttp.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, stdhttp.StatusOK)
	}
	if got := atomic.LoadInt32(&attempts); got != 3 {
		t.Fatalf("attempts = %d, want 3", got)
	}

	mu.Lock()
	defer mu.Unlock()
	for index := range keys {
		if keys[index] != idempotencyKey {
			t.Errorf("attempt %d idempotency key = %q, want %q", index+1, keys[index], idempotencyKey)
		}
		if bodies[index] != requestBody {
			t.Errorf("attempt %d body = %q, want %q", index+1, bodies[index], requestBody)
		}
	}
}

func TestEmptyIdempotencyKeyDoesNotEnableUnsafeRetry(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		atomic.AddInt32(&attempts, 1)
		writer.WriteHeader(stdhttp.StatusBadGateway)
	}))
	defer server.Close()

	response, err := newRetryTestClient().RequestWithOptions(
		stdhttp.MethodPost,
		server.URL,
		nil,
		nil,
		WithIdempotencyKey("   "),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer closeTestResponse(t, response)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestClientErrorsDoNotRetryOnFourHundreds(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		atomic.AddInt32(&attempts, 1)
		writer.WriteHeader(stdhttp.StatusTooManyRequests)
	}))
	defer server.Close()

	response, err := newRetryTestClient().Get(server.URL, nil)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer closeTestResponse(t, response)

	if got := atomic.LoadInt32(&attempts); got != 1 {
		t.Fatalf("attempts = %d, want 1", got)
	}
}

func TestRetryBackoffIsExponentialAndBounded(t *testing.T) {
	const base = 100 * time.Millisecond
	for attempt := 1; attempt <= 8; attempt++ {
		shift := attempt - 1
		if shift > 6 {
			shift = 6
		}
		minimum := base * time.Duration(1<<shift)
		maximum := minimum + minimum/4
		got := retryBackoff(base, attempt)
		if got < minimum || got > maximum {
			t.Fatalf("retryBackoff(%v, %d) = %v, want [%v, %v]", base, attempt, got, minimum, maximum)
		}
	}
}
