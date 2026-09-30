package observability

import (
	"io"
	"net/http"
	"sync/atomic"

	"github.com/felixge/httpsnoop"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// NewHTTPHandler keeps tracing and the writer's optional streaming/hijacking
// interfaces. otelhttp v0.54 forwards a WriteHeader(200) on every Write/Flush,
// including after an explicit error status. Commit the final status once at
// the transport boundary while still permitting informational responses.
func NewHTTPHandler(handler http.Handler, name string) http.Handler {
	traced := otelhttp.NewHandler(handler, name)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var committed atomic.Bool
		writer := httpsnoop.Wrap(w, httpsnoop.Hooks{
			WriteHeader: func(next httpsnoop.WriteHeaderFunc) httpsnoop.WriteHeaderFunc {
				return func(code int) {
					if code >= 100 && code < 200 && code != http.StatusSwitchingProtocols {
						if !committed.Load() {
							next(code)
						}
						return
					}
					if committed.CompareAndSwap(false, true) {
						next(code)
					}
				}
			},
			Write: func(next httpsnoop.WriteFunc) httpsnoop.WriteFunc {
				return func(p []byte) (int, error) {
					committed.Store(true)
					return next(p)
				}
			},
			Flush: func(next httpsnoop.FlushFunc) httpsnoop.FlushFunc {
				return func() {
					committed.Store(true)
					next()
				}
			},
			ReadFrom: func(next httpsnoop.ReadFromFunc) httpsnoop.ReadFromFunc {
				return func(src io.Reader) (int64, error) {
					committed.Store(true)
					return next(src)
				}
			},
		})
		traced.ServeHTTP(writer, r)
	})
}
