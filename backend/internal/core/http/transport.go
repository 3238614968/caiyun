package http

import (
	"context"
	"errors"
	"net"
	"net/http"
	"time"
)

var taskTransport = newTaskTransport()

func newTaskTransport() *http.Transport {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	dialer := &net.Dialer{Timeout: 30 * time.Second, KeepAlive: 30 * time.Second}
	transport.DialContext = ipv4FirstDialer(dialer.DialContext)
	return transport
}

// Choose a reachable address before sending any HTTP bytes. This works for
// POSTs without replaying mutations when a server has broken IPv6 routing.
// IPv6-only hosts and explicitly requested IPv6 addresses still work.
func ipv4FirstDialer(dial func(context.Context, string, string) (net.Conn, error)) func(context.Context, string, string) (net.Conn, error) {
	return func(ctx context.Context, network, address string) (net.Conn, error) {
		host, _, err := net.SplitHostPort(address)
		ip := net.ParseIP(host)
		if network != "tcp" || err != nil || ip != nil && ip.To4() == nil {
			return dial(ctx, network, address)
		}
		conn, firstErr := dial(ctx, "tcp4", address)
		if firstErr == nil {
			return conn, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		conn, err = dial(ctx, network, address)
		if err == nil {
			return conn, nil
		}
		return nil, errors.Join(firstErr, err)
	}
}
