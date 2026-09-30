package observability

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestHTTPTracingPreservesStatusWithoutDuplicateHeaders(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, status := range []int{200, 401, 404, 409, 500} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			router := gin.New()
			router.GET("/test", func(c *gin.Context) {
				c.JSON(status, gin.H{"code": status})
			})
			var logs bytes.Buffer
			server := httptest.NewUnstartedServer(NewHTTPHandler(router, "test"))
			server.Config.ErrorLog = log.New(&logs, "", 0)
			server.Start()
			defer server.Close()
			resp, err := server.Client().Get(server.URL + "/test")
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			body, err := io.ReadAll(resp.Body)
			if err != nil || resp.StatusCode != status || !strings.Contains(string(body), "code") {
				t.Fatalf("status=%d body=%s err=%v", resp.StatusCode, body, err)
			}
			if logs.Len() != 0 {
				t.Fatalf("unexpected transport warning: %s", logs.String())
			}
		})
	}
}

func TestHTTPTracingPreservesStreamingAndWriterCapabilities(t *testing.T) {
	var logs bytes.Buffer
	server := httptest.NewUnstartedServer(NewHTTPHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, ok := w.(http.Hijacker); !ok {
			t.Error("tracing wrapper removed websocket hijacking")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, chunk := range []string{"data: first\n\n", "data: second\n\n"} {
			_, _ = io.WriteString(w, chunk)
			if err := http.NewResponseController(w).Flush(); err != nil {
				t.Errorf("stream flush: %v", err)
			}
		}
	}), "stream"))
	server.Config.ErrorLog = log.New(&logs, "", 0)
	server.Start()
	defer server.Close()
	resp, err := server.Client().Get(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil || string(body) != "data: first\n\ndata: second\n\n" || resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d body=%q err=%v", resp.StatusCode, body, err)
	}
	if logs.Len() != 0 {
		t.Fatalf("unexpected transport warning: %s", logs.String())
	}
}
