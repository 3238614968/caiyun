package api

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestShutdownAPIServerStopsRealtimeBeforeHTTP(t *testing.T) {
	order := make([]string, 0, 2)
	err := shutdownAPIServer(
		func(context.Context) error { order = append(order, "realtime"); return nil },
		func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("HTTP shutdown context has no deadline")
			}
			order = append(order, "http")
			return nil
		},
		time.Second,
	)
	if err != nil {
		t.Fatalf("shutdownAPIServer() error = %v", err)
	}
	if got, want := len(order), 2; got != want || order[0] != "realtime" || order[1] != "http" {
		t.Fatalf("shutdown order = %v, want [realtime http]", order)
	}
}

func TestShutdownAPIServerUsesOneBudgetAndClosesHTTPAfterRealtimeError(t *testing.T) {
	want := errors.New("realtime drain timed out")
	httpCalled := false
	err := shutdownAPIServer(
		func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				t.Fatal("realtime shutdown context has no deadline")
			}
			return want
		},
		func(context.Context) error { httpCalled = true; return nil },
		time.Second,
	)
	if !errors.Is(err, want) {
		t.Fatalf("shutdownAPIServer() error = %v, want %v", err, want)
	}
	if !httpCalled {
		t.Fatal("HTTP shutdown did not run after realtime shutdown failed")
	}
}

func TestShutdownAPIServerReservesHTTPBudgetAfterRealtimeTimeout(t *testing.T) {
	var httpDeadline time.Time
	err := shutdownAPIServer(
		func(ctx context.Context) error {
			<-ctx.Done()
			return ctx.Err()
		},
		func(ctx context.Context) error {
			var ok bool
			httpDeadline, ok = ctx.Deadline()
			if !ok || time.Until(httpDeadline) <= 0 {
				t.Fatal("HTTP shutdown received no remaining process budget")
			}
			return nil
		},
		90*time.Millisecond,
	)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("shutdownAPIServer() error = %v, want realtime deadline", err)
	}
	if httpDeadline.IsZero() {
		t.Fatal("HTTP shutdown was not called")
	}
}

func TestShutdownAPIServerAllowsNilHooksAndNormalizesBudget(t *testing.T) {
	called := false
	err := shutdownAPIServer(nil, func(ctx context.Context) error {
		called = true
		deadline, ok := ctx.Deadline()
		if !ok || time.Until(deadline) <= 0 {
			t.Fatal("HTTP shutdown did not receive normalized deadline")
		}
		return nil
	}, 0)
	if err != nil || !called {
		t.Fatalf("shutdownAPIServer() = (%v, %v), want (nil, true)", err, called)
	}
	if err := shutdownAPIServer(nil, nil, time.Second); err != nil {
		t.Fatalf("nil shutdown hook returned %v", err)
	}
}
