package http

import (
	"errors"
	stdhttp "net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestCircuitBreakerRejectsRequestsAfterFailureThreshold(t *testing.T) {
	var calls int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		atomic.AddInt32(&calls, 1)
		writer.WriteHeader(stdhttp.StatusBadGateway)
	}))
	defer server.Close()

	client := NewClient(WithCircuitBreakerConfig(CircuitBreakerConfig{
		FailureThreshold: 1,
		OpenTimeout:      time.Hour,
		HalfOpenRequests: 1,
	}))
	client.maxRetries = 0

	response, err := client.Get(server.URL, nil)
	if err != nil {
		t.Fatalf("first request error = %v", err)
	}
	closeTestResponse(t, response)

	response, err = client.Get(server.URL, nil)
	if response != nil {
		closeTestResponse(t, response)
		t.Fatal("open circuit returned a response")
	}
	if !errors.Is(err, ErrUpstreamCircuitOpen) {
		t.Fatalf("second request error = %v, want ErrUpstreamCircuitOpen", err)
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("upstream calls = %d, want 1", got)
	}
}

func TestCircuitBreakerResetsAfterSuccessfulLogicalRequest(t *testing.T) {
	var calls int32
	server := httptest.NewServer(stdhttp.HandlerFunc(func(writer stdhttp.ResponseWriter, request *stdhttp.Request) {
		current := atomic.AddInt32(&calls, 1)
		if current == 1 {
			writer.WriteHeader(stdhttp.StatusBadGateway)
			return
		}
		writer.WriteHeader(stdhttp.StatusOK)
	}))
	defer server.Close()

	client := NewClient(WithCircuitBreakerConfig(CircuitBreakerConfig{FailureThreshold: 2, OpenTimeout: time.Hour, HalfOpenRequests: 1}))
	client.maxRetries = 0
	response, err := client.Get(server.URL, nil)
	if err != nil {
		t.Fatalf("first request error = %v", err)
	}
	closeTestResponse(t, response)

	response, err = client.Get(server.URL, nil)
	if err != nil {
		t.Fatalf("second request error = %v", err)
	}
	if response.StatusCode != stdhttp.StatusOK {
		t.Fatalf("second status = %d, want 200", response.StatusCode)
	}
	closeTestResponse(t, response)

	response, err = client.Get(server.URL, nil)
	if err != nil {
		t.Fatalf("third request error = %v", err)
	}
	closeTestResponse(t, response)
	if got := atomic.LoadInt32(&calls); got != 3 {
		t.Fatalf("upstream calls = %d, want 3", got)
	}
}
