package ws

import (
	"net/http/httptest"
	"testing"
	"time"
)

func TestSSEHeartbeatIntervalDefaultsAndBounds(t *testing.T) {
	t.Setenv("SSE_HEARTBEAT_INTERVAL", "")
	if got := sseHeartbeatInterval(); got != 3*time.Second {
		t.Fatalf("default heartbeat = %s, want 3s", got)
	}
	t.Setenv("SSE_HEARTBEAT_INTERVAL", "100ms")
	if got := sseHeartbeatInterval(); got != time.Second {
		t.Fatalf("bounded heartbeat = %s, want 1s", got)
	}
}

func TestRequestedSSELastEventIDPrefersHeaderAndSupportsReconnectQuery(t *testing.T) {
	queryRequest := httptest.NewRequest("GET", "/events?last_event_id=42", nil)
	if got := requestedSSELastEventID(queryRequest); got != 42 {
		t.Fatalf("query last event ID = %d, want 42", got)
	}
	queryRequest.Header.Set("Last-Event-ID", "43")
	if got := requestedSSELastEventID(queryRequest); got != 43 {
		t.Fatalf("header last event ID = %d, want 43", got)
	}
}
