package handlers

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"caiyun/internal/security"
	"github.com/gin-gonic/gin"
)

func TestExchangeCredentialReadErrorIsActionableAndDoesNotLeakDetails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	respondExchangeServiceError(c, fmt.Errorf("%w: internal-private-detail", security.ErrCredentialUnreadable))
	if w.Code != http.StatusConflict || !strings.Contains(w.Body.String(), "数据加密密钥") {
		t.Fatalf("credential failure status=%d body=%s", w.Code, w.Body.String())
	}
	if strings.Contains(w.Body.String(), "internal-private-detail") {
		t.Fatal("credential failure leaked internal details")
	}
}
