package http

import (
	"context"
	"errors"
	"io"
	stdhttp "net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRequestTimeoutOverrideCoversBodyWithoutMutatingClient(t *testing.T) {
	server := httptest.NewServer(stdhttp.HandlerFunc(func(w stdhttp.ResponseWriter, r *stdhttp.Request) {
		w.WriteHeader(200)
		w.(stdhttp.Flusher).Flush()
		select {
		case <-r.Context().Done():
			return
		case <-time.After(80 * time.Millisecond):
		}
		_, _ = io.WriteString(w, "data: complete\n\n")
	}))
	defer server.Close()
	client := NewClient()
	client.client.Timeout = 25 * time.Millisecond
	resp, err := client.PostWithTimeout(server.URL, nil, map[string]int{}, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	body, err := client.ReadResponseBody(resp)
	if err != nil || body != "data: complete\n\n" {
		t.Fatalf("extended body read=%q err=%v", body, err)
	}
	if client.client.Timeout != 25*time.Millisecond {
		t.Fatal("request changed the shared client timeout")
	}
	resp, err = client.Post(server.URL, nil, map[string]int{})
	if err == nil {
		_, err = client.ReadResponseBody(resp)
	}
	if err == nil {
		t.Fatal("ordinary requests lost their original timeout")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.RequestWithContextOptions(ctx, "POST", server.URL, nil, nil, WithRequestTimeout(time.Second))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("request ignored caller cancellation: %v", err)
	}
}
