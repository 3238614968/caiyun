package tasks

import (
	"strings"
	"testing"

	"caiyun/internal/core/api"
)

func TestDescribeStudentPerksStatus(t *testing.T) {
	message := describeStudentPerksStatus(api.StudentPerksStatus{
		CertStatus:      0,
		CertStatusDesc:  "未认证",
		CanCert:         true,
		CanClaim:        false,
		StockStatusDesc: "库存充足",
	})
	if message == "" {
		t.Fatal("describeStudentPerksStatus returned empty message")
	}
	for _, want := range []string{"未认证", "可认证=true", "可领奖=false", "库存充足"} {
		if !strings.Contains(message, want) {
			t.Fatalf("message %q missing %q", message, want)
		}
	}

	fallback := describeStudentPerksStatus(api.StudentPerksStatus{CertStatus: 2})
	if !strings.Contains(fallback, "certStatus=2") {
		t.Fatalf("fallback message = %q, want certStatus=2", fallback)
	}
}

func TestResultItems(t *testing.T) {
	if items := resultItems(&api.CaiyunResponse{Result: []interface{}{1, 2, 3}}); len(items) != 3 {
		t.Fatalf("array result items = %d, want 3", len(items))
	}
	nested := &api.CaiyunResponse{Result: map[string]interface{}{
		"records": []interface{}{map[string]interface{}{"id": "1"}},
	}}
	if items := resultItems(nested); len(items) != 1 {
		t.Fatalf("nested records items = %d, want 1", len(items))
	}
	if items := resultItems(&api.CaiyunResponse{Result: map[string]interface{}{"other": 1}}); len(items) != 0 {
		t.Fatalf("unexpected items = %d, want 0", len(items))
	}
	if items := resultItems(nil); items != nil {
		t.Fatalf("nil response items = %v, want nil", items)
	}
}
