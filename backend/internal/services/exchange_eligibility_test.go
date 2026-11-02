package services

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestExchangeEligibilityBlockReason(t *testing.T) {
	cases := []struct{ code, wantContains string }{
		{"", ""},
		{"0", ""},
		{"2302", "当月"},
		{"2303", "备份"},
		{"2305", "非目标用户"},
		{"9999", "未给出具体原因"},
	}
	for _, c := range cases {
		got := exchangeEligibilityBlockReason(&exchangeEligibility{code: c.code})
		if c.wantContains == "" {
			if got != "" {
				t.Fatalf("exchangeCode=%q 不应拦截，得到 %q", c.code, got)
			}
			continue
		}
		if !strings.Contains(got, c.wantContains) {
			t.Fatalf("exchangeCode=%q 的原因 %q 不含 %q", c.code, got, c.wantContains)
		}
	}
	if exchangeEligibilityBlockReason(nil) != "" {
		t.Fatal("nil 资格信息不应拦截兑换")
	}
}

func TestFetchExchangeEligibilityParsesCode(t *testing.T) {
	var gotPath, gotQuery string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		// exchangeCode 故意用 JSON 数字，覆盖 "数字/字符串都能解析"。
		_, _ = io.WriteString(w, `{"code":0,"msg":"ok","result":{"exchangeCode":2305,"memo":"<p>该奖品中国移动用户可兑换</p>"}}`)
	}))
	defer server.Close()

	session := newMockExchangeSession(t, server)
	got, err := fetchExchangeEligibilityContext(context.Background(), session,
		&exchangeAuthContext{jwtToken: "jwt-token"}, "251230051")
	if err != nil {
		t.Fatalf("fetchExchangeEligibilityContext() err = %v", err)
	}
	if gotPath != "/ycloud/signin/page/receivePrizeDetails" {
		t.Fatalf("path = %q", gotPath)
	}
	if !strings.Contains(gotQuery, "marketId=sign_in_3_ex") || !strings.Contains(gotQuery, "prizeId=251230051") {
		t.Fatalf("query = %q", gotQuery)
	}
	if got.code != "2305" {
		t.Fatalf("exchangeCode = %q, want 2305", got.code)
	}
	if reason := exchangeEligibilityBlockReason(got); !strings.Contains(reason, "非目标用户") {
		t.Fatalf("block reason = %q", reason)
	}
	if note := exchangeEligibilityNote(got); !strings.Contains(note, "2305") {
		t.Fatalf("note = %q", note)
	}
}
