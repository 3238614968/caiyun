package ws

import (
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
