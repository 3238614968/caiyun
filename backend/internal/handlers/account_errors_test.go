package handlers

import (
	"caiyun/internal/services"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAccountWriteErrorsAreActionableAndRetainInternalCause(t *testing.T) {
	for _, tc := range []struct {
		err    error
		status int
	}{
		{services.ErrAccountExists, http.StatusConflict},
		{services.ErrInvalidAuthorization, http.StatusBadRequest},
		{errors.New("database-private-detail"), http.StatusInternalServerError},
	} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		respondAccountWriteError(c, tc.err)
		if w.Code != tc.status || strings.Contains(w.Body.String(), "database-private-detail") {
			t.Fatalf("unsafe account response: %d %s", w.Code, w.Body)
		}
		if tc.status == 500 && len(c.Errors) != 1 {
			t.Fatal("creation failure discarded the cause needed for trace diagnostics")
		}
	}
}
