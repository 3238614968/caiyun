package sms

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSolveSlideAcceptsReferenceSolverXCoordinate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body map[string]string
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body["puzzle"] != "piece" || body["picture"] != "background" {
			t.Errorf("unexpected solver payload: %v %v", body, err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"message":"success","data":{"x":31,"confidence":0.9}}`))
	}))
	defer server.Close()
	t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
	t.Setenv("CAIYUN_SMS_API_TOKEN", "fixture-token")
	result, err := SolveSlideContext(context.Background(), "piece", "background")
	if err != nil || result == nil || result.Offset != 31 {
		t.Fatalf("reference coordinate=%+v err=%v", result, err)
	}
}
