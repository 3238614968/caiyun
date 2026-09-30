package api

import "testing"

func TestParseStudentPerksStatus(t *testing.T) {
	resp := &CaiyunResponse{
		Result: map[string]interface{}{
			"certStatus":      float64(1),
			"certStatusDesc":  "已认证",
			"canCert":         false,
			"canClaim":        true,
			"prizeStatus":     float64(0),
			"stockStatus":     float64(1),
			"stockStatusDesc": "库存充足",
		},
	}
	status, ok := ParseStudentPerksStatus(resp)
	if !ok {
		t.Fatal("ParseStudentPerksStatus() failed")
	}
	if status.CertStatus != 1 || status.CertStatusDesc != "已认证" || !status.CanClaim || status.CanCert {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.StockStatusDesc != "库存充足" {
		t.Fatalf("StockStatusDesc = %q", status.StockStatusDesc)
	}
	if !StudentPerksReadyToClaim(status) {
		t.Fatal("status 1 should be ready to claim")
	}
}

func TestStudentPerksReadyToClaim(t *testing.T) {
	cases := []struct {
		name   string
		status StudentPerksStatus
		want   bool
	}{
		{"未认证", StudentPerksStatus{CertStatus: 0, CanClaim: false}, false},
		{"已认证可领取", StudentPerksStatus{CertStatus: 1, CanClaim: true}, true},
		{"已认证但不可领取", StudentPerksStatus{CertStatus: 1, CanClaim: false}, false},
		{"已领取", StudentPerksStatus{CertStatus: 2, CanClaim: false}, false},
		{"canClaim 标记", StudentPerksStatus{CertStatus: 0, CanClaim: true}, true},
	}
	for _, tc := range cases {
		if got := StudentPerksReadyToClaim(tc.status); got != tc.want {
			t.Fatalf("%s: StudentPerksReadyToClaim = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestStudentPerksPrizeClaimRequiresCode(t *testing.T) {
	api := &CaiyunAPI{}
	if _, err := api.StudentPerksPrizeClaim("  "); err == nil {
		t.Fatal("StudentPerksPrizeClaim should reject empty code")
	}
}

func TestMd5HexLowerMatchesPageSubmission(t *testing.T) {
	// 页面提交的是短验码的 MD5 十六进制小写串。
	if got := md5HexLower("1234"); got != "81dc9bdb52d04dc20036dbd8313ed055" {
		t.Fatalf("md5HexLower(\"1234\") = %q", got)
	}
}
