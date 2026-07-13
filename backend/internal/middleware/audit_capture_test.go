package middleware

import (
	"bytes"
	"net/http/httptest"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

func TestLimitedAuditCaptureBoundsAndMarksTruncation(t *testing.T) {
	if auditCaptureSize != 64<<10 {
		t.Fatalf("auditCaptureSize = %d, want 64 KiB", auditCaptureSize)
	}

	capture := newLimitedAuditCapture(auditCaptureSize)
	payload := bytes.Repeat([]byte("a"), auditCaptureSize+1024)

	written, err := capture.Write(payload)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != len(payload) {
		t.Fatalf("Write() = %d, want full input length %d", written, len(payload))
	}
	if got := capture.body.Len(); got != auditCaptureSize {
		t.Fatalf("captured bytes = %d, want %d", got, auditCaptureSize)
	}
	if !bytes.Equal(capture.body.Bytes(), payload[:auditCaptureSize]) {
		t.Fatal("capture does not contain the expected payload prefix")
	}
	if !capture.truncated {
		t.Fatal("capture.truncated = false, want true")
	}
	if formatted := formatAuditCapture(capture); !strings.HasSuffix(formatted, " [capture_truncated]") {
		t.Fatalf("formatAuditCapture() = %q, want truncation marker", formatted)
	}
}

func TestBodyLogWriterForwardsFullBodyAndLimitsCapture(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	capture := newLimitedAuditCapture(auditCaptureSize)
	writer := &bodyLogWriter{
		ResponseWriter: context.Writer,
		capture:        capture,
		captureBody:    true,
	}
	payload := bytes.Repeat([]byte("response-body-"), auditCaptureSize/len("response-body-")+1024)

	written, err := writer.Write(payload)
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if written != len(payload) {
		t.Fatalf("Write() = %d, want full response length %d", written, len(payload))
	}
	if !bytes.Equal(recorder.Body.Bytes(), payload) {
		t.Fatalf("forwarded response length = %d, want %d", recorder.Body.Len(), len(payload))
	}
	if got := capture.body.Len(); got != auditCaptureSize {
		t.Fatalf("captured response bytes = %d, want %d", got, auditCaptureSize)
	}
	if !bytes.Equal(capture.body.Bytes(), payload[:auditCaptureSize]) {
		t.Fatal("response capture does not contain the expected payload prefix")
	}
	if !capture.truncated {
		t.Fatal("response capture was not marked as truncated")
	}
}

func TestBodyLogWriterWriteStringIsCapturedAndForwarded(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	capture := newLimitedAuditCapture(auditCaptureSize)
	writer := &bodyLogWriter{
		ResponseWriter: context.Writer,
		capture:        capture,
		captureBody:    true,
	}

	const payload = "string-response"
	written, err := writer.WriteString(payload)
	if err != nil {
		t.Fatalf("WriteString() error = %v", err)
	}
	if written != len(payload) || recorder.Body.String() != payload {
		t.Fatalf("WriteString() wrote %d and forwarded %q", written, recorder.Body.String())
	}
	if capture.body.String() != payload {
		t.Fatalf("captured body = %q", capture.body.String())
	}
}

func TestShouldCaptureAuditBodySkipsBinaryAndMultipartRequests(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, contentType := range []string{"application/octet-stream", "multipart/form-data; boundary=test"} {
		recorder := httptest.NewRecorder()
		context, _ := gin.CreateTestContext(recorder)
		context.Request = httptest.NewRequest("POST", "/api/v1/accounts", strings.NewReader("secret"))
		context.Request.Header.Set("Content-Type", contentType)
		if shouldCaptureAuditBody(context) {
			t.Fatalf("shouldCaptureAuditBody(%q) = true", contentType)
		}
	}
}

func TestTruncateStringPreservesValidUTF8(t *testing.T) {
	input := strings.Repeat("中🙂", 16)
	got := truncateString(input, 17)
	if !utf8.ValidString(got) {
		t.Fatalf("truncateString returned invalid UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") || len(strings.TrimSuffix(got, "...")) > 17 {
		t.Fatalf("truncateString result = %q", got)
	}
}
