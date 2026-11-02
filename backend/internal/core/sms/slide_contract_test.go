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
	if err != nil || result == nil || result.Offset != 31 || !result.HasConfidence || result.Confidence != 0.9 {
		t.Fatalf("reference coordinate=%+v err=%v", result, err)
	}
}

func TestSolveSlideValidatesCoordinateAndConfidence(t *testing.T) {
	for _, tc := range []struct {
		name, data string
		wantError  bool
	}{
		{"legacy-without-confidence", `{"offset":31}`, false},
		{"missing-coordinate", `{"confidence":0.9}`, true},
		{"null-coordinate", `{"offset":null,"confidence":0.9}`, true},
		{"zero-confidence", `{"offset":31,"confidence":0}`, false},
		{"fractional-offset", `{"offset":31.5}`, true},
		{"negative-offset", `{"offset":-1}`, true},
		{"overflow-offset", `{"offset":2147483648}`, true},
		{"invalid-confidence", `{"offset":31,"confidence":"NaN"}`, true},
		{"out-of-range-confidence", `{"offset":31,"confidence":1.1}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"code":0,"data":` + tc.data + `}`))
			}))
			defer server.Close()
			t.Setenv("CAIYUN_SMS_API_BASE_URL", server.URL)
			result, err := SolveSlideContext(context.Background(), "piece", "background")
			if (err != nil) != tc.wantError {
				t.Fatalf("result=%+v err=%v wantError=%v", result, err, tc.wantError)
			}
			if !tc.wantError && result.HasConfidence != (tc.name == "zero-confidence") {
				t.Fatalf("unexpected confidence presence: %+v", result)
			}
		})
	}
}
