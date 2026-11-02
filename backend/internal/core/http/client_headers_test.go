package http

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Use a real HTTP transport: an in-process RoundTripper that reads only the
// first matching map key hides duplicated headers sent to the upstream gateway.
func TestRequestMergesHeaderAliasesOnTheWire(t *testing.T) {
	for _, http2 := range []bool{false, true} {
		name := "http1"
		if http2 {
			name = "http2"
		}
		t.Run(name, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if http2 && r.ProtoMajor != 2 {
					t.Errorf("expected HTTP/2, got %s", r.Proto)
				}
				for field, want := range map[string]string{
					"jwttoken": "fixture-jwt", "x-deviceinfo": "request-device",
					"user-agent": "request-agent", "content-type": "application/json",
					"x-requested-with": "com.chinamobile.mcloud",
				} {
					if got := r.Header.Values(field); len(got) != 1 || got[0] != want {
						t.Errorf("%s=%q, want exactly [%q]", field, got, want)
					}
				}
				_, _ = io.WriteString(w, `{"code":0}`)
			}))
			server.EnableHTTP2 = http2
			server.StartTLS()
			defer server.Close()
			client := NewClient(WithTransport(server.Client().Transport))
			client.SetJWTToken("fixture-jwt")
			response, err := client.Post(server.URL, map[string]string{
				"jwtToken": "fixture-jwt", "jwttoken": "fixture-jwt",
				"x-deviceinfo": "request-device", "user-agent": "request-agent",
				"content-type": "application/json", "X-Requested-With": "com.chinamobile.mcloud",
			}, map[string]string{"operation": "fixture"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := client.ReadResponseBody(response); err != nil {
				t.Fatal(err)
			}
		})
	}
}
