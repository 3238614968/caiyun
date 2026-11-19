package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestDailyTaskSummaryRejectsUnauthenticatedAndInvalidDateReads(t *testing.T) {
	for _, scenario := range []struct {
		query  string
		user   bool
		status int
	}{
		{"", false, http.StatusUnauthorized}, {"?date=2026-11-32", true, http.StatusBadRequest},
	} {
		writer := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(writer)
		ctx.Request = httptest.NewRequest(http.MethodGet, "/api/v1/tasks/daily-summary"+scenario.query, nil)
		if scenario.user {
			ctx.Set("user_id", uint(7))
		}
		(&TaskHandler{}).GetTaskDailySummary(ctx)
		if writer.Code != scenario.status {
			t.Fatalf("status=%d want=%d body=%s", writer.Code, scenario.status, writer.Body.String())
		}
	}
}
