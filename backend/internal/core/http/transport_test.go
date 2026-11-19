package http

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
)

func TestTaskDialerUsesIPv4AndFallsBackForIPv6OnlyHosts(t *testing.T) {
	for _, fixture := range []struct {
		name, network, address, want string
		failIPv4                     bool
	}{
		{"dual stack", "tcp", "service.example:443", "tcp4", false},
		{"IPv6-only DNS", "tcp", "service.example:443", "tcp4,tcp", true},
		{"explicit IPv6", "tcp", "[::1]:443", "tcp", false},
		{"explicit tcp6", "tcp6", "service.example:443", "tcp6", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			var calls []string
			left, right := net.Pipe()
			defer left.Close()
			defer right.Close()
			dial := ipv4FirstDialer(func(ctx context.Context, network, address string) (net.Conn, error) {
				calls = append(calls, network)
				if fixture.failIPv4 && network == "tcp4" {
					return nil, errors.New("no IPv4 address")
				}
				return left, nil
			})
			conn, err := dial(context.Background(), fixture.network, fixture.address)
			if err != nil || conn != left || strings.Join(calls, ",") != fixture.want {
				t.Fatalf("calls=%v conn=%v err=%v", calls, conn, err)
			}
		})
	}
}

func TestTaskDialerHonorsCancellationAndPreservesBothFailures(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	calls := 0
	dial := ipv4FirstDialer(func(context.Context, string, string) (net.Conn, error) {
		calls++
		cancel()
		return nil, errors.New("unreachable")
	})
	if _, err := dial(ctx, "tcp", "service.example:443"); !errors.Is(err, context.Canceled) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
	v4, v6 := errors.New("IPv4 unavailable"), errors.New("IPv6 unreachable")
	dial = ipv4FirstDialer(func(_ context.Context, network, _ string) (net.Conn, error) {
		if network == "tcp4" {
			return nil, v4
		}
		return nil, v6
	})
	if _, err := dial(context.Background(), "tcp", "service.example:443"); !errors.Is(err, v4) || !errors.Is(err, v6) {
		t.Fatalf("lost network failure: %v", err)
	}
}
