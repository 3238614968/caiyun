package response

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newRecorderContext() (*httptest.ResponseRecorder, *gin.Context) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	return recorder, ctx
}

func decodeResponse(t *testing.T, recorder *httptest.ResponseRecorder) Response {
	t.Helper()
	var payload Response
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v; body=%s", err, recorder.Body.String())
	}
	return payload
}

func TestSuccessWrapsData(t *testing.T) {
	recorder, ctx := newRecorderContext()

	Success(ctx, gin.H{"value": 42})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	payload := decodeResponse(t, recorder)
	if payload.Code != 0 || payload.Message != "success" {
		t.Fatalf("payload code/message = %d/%q, want 0/success", payload.Code, payload.Message)
	}
	data, ok := payload.Data.(map[string]interface{})
	if !ok || data["value"].(float64) != 42 {
		t.Fatalf("payload data = %#v, want value=42", payload.Data)
	}
}

func TestMessageOmitsData(t *testing.T) {
	recorder, ctx := newRecorderContext()

	Message(ctx, "操作成功")

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	payload := decodeResponse(t, recorder)
	if payload.Code != 0 || payload.Message != "操作成功" {
		t.Fatalf("payload code/message = %d/%q, want 0/操作成功", payload.Code, payload.Message)
	}
	if payload.Data != nil {
		t.Fatalf("payload data = %#v, want nil", payload.Data)
	}
}

func TestBadRequestUsesHttpStatusAsCode(t *testing.T) {
	recorder, ctx := newRecorderContext()

	BadRequest(ctx, "参数错误")

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusBadRequest)
	}
	payload := decodeResponse(t, recorder)
	if payload.Code != http.StatusBadRequest || payload.Message != "参数错误" {
		t.Fatalf("payload code/message = %d/%q, want 400/参数错误", payload.Code, payload.Message)
	}
}
