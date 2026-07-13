package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"caiyun/internal/services"
	appErrors "caiyun/pkg/errors"
	apiresponse "caiyun/pkg/response"

	"github.com/gin-gonic/gin"
)

func TestRespondTaskHandlerErrorMapsStableBusinessErrors(t *testing.T) {
	gin.SetMode(gin.TestMode)
	tests := []struct {
		name         string
		err          error
		status       int
		businessCode appErrors.BusinessCode
	}{
		{name: "account not found", err: services.ErrAccountNotFound, status: http.StatusNotFound, businessCode: appErrors.BusinessCodeAccountNotFound},
		{name: "deadline", err: context.DeadlineExceeded, status: http.StatusGatewayTimeout, businessCode: appErrors.BusinessCodeTaskTimeout},
		{name: "canceled", err: context.Canceled, status: http.StatusRequestTimeout, businessCode: appErrors.BusinessCodeTaskTimeout},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(recorder)
			respondTaskHandlerError(c, tt.err)
			if recorder.Code != tt.status {
				t.Fatalf("status = %d, want %d", recorder.Code, tt.status)
			}
			var body apiresponse.Response
			if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
				t.Fatalf("decode response: %v", err)
			}
			if body.BusinessCode != string(tt.businessCode) {
				t.Fatalf("business_code = %q, want %q", body.BusinessCode, tt.businessCode)
			}
		})
	}
}

func TestRespondTaskHandlerErrorDoesNotLeakUnknownError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	const secret = "sql connection private detail"
	respondTaskHandlerError(c, errors.New(secret))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusInternalServerError)
	}
	if strings.Contains(recorder.Body.String(), secret) {
		t.Fatalf("response leaked internal error: %s", recorder.Body.String())
	}
	if len(c.Errors) != 1 {
		t.Fatalf("gin errors = %d, want 1", len(c.Errors))
	}
}
