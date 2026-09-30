package api

import "testing"

func TestPrizeCenterUnclaimedFiltersAndSorts(t *testing.T) {
	entries := []PrizeCenterEntry{
		{OID: "3", Flag: PrizeFlagUnclaimed, ExpireTime: "2026-12-01T00:00:00"},
		{OID: "1", Flag: PrizeFlagUnclaimed, ExpireTime: "2026-10-01T00:00:00"},
		{OID: "2", Flag: PrizeFlagClaimed, ExpireTime: "2026-09-01T00:00:00"},
		{OID: "4", Flag: PrizeFlagExpired, ExpireTime: "2026-08-01T00:00:00"},
		{OID: "5", Flag: PrizeFlagUnclaimed, ExpireTime: ""},
	}
	got := PrizeCenterUnclaimed(entries)
	if len(got) != 3 {
		t.Fatalf("unclaimed count = %d, want 3", len(got))
	}
	wantOrder := []string{"1", "3", "5"}
	for i, want := range wantOrder {
		if got[i].OID != want {
			t.Fatalf("unclaimed[%d] = %q, want %q (order %v)", i, got[i].OID, want, orderOf(got))
		}
	}
}

func orderOf(entries []PrizeCenterEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, entry.OID)
	}
	return out
}

func TestParsePrizeCenterPage(t *testing.T) {
	resp := &CaiyunResponse{
		Result: map[string]interface{}{
			"totalPages": float64(3),
			"result": []interface{}{
				map[string]interface{}{
					"oId":        "1273414545",
					"prizeName":  "移动云盘10G个人云空间年卡",
					"prizeId":    "251230002",
					"marketid":   "National_NewLoginGif",
					"marketname": "National_NewLoginGif",
					"expireTime": "2026-12-31T23:59:59",
					"flag":       float64(1),
				},
			},
		},
	}
	page := parsePrizeCenterPage(resp)
	if page.TotalPages != 3 {
		t.Fatalf("TotalPages = %d, want 3", page.TotalPages)
	}
	if len(page.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(page.Entries))
	}
	entry := page.Entries[0]
	if entry.OID != "1273414545" || entry.PrizeID != "251230002" || entry.Flag != PrizeFlagUnclaimed {
		t.Fatalf("unexpected entry: %+v", entry)
	}
	if entry.PrizeName != "移动云盘10G个人云空间年卡" {
		t.Fatalf("PrizeName = %q", entry.PrizeName)
	}
}

func TestParsePrizeCenterPageUsesRecordsFallback(t *testing.T) {
	resp := &CaiyunResponse{
		Result: map[string]interface{}{
			"records": []interface{}{map[string]interface{}{"oId": "9", "flag": float64(2)}},
		},
	}
	page := parsePrizeCenterPage(resp)
	if len(page.Entries) != 1 || page.Entries[0].OID != "9" {
		t.Fatalf("records fallback failed: %+v", page)
	}
	// totalPages 缺省时回落到 1，避免调用方无限翻页。
	if page.TotalPages != 1 {
		t.Fatalf("TotalPages = %d, want 1", page.TotalPages)
	}
}

func TestPrizeCenterAcceptRequiresSMS(t *testing.T) {
	api := &CaiyunAPI{}
	if _, err := api.PrizeCenterAccept("123", ""); err == nil {
		t.Fatal("PrizeCenterAccept should reject empty sms code")
	}
	if _, err := api.PrizeCenterSendSMS("123", 0); err == nil {
		t.Fatal("PrizeCenterSendSMS should reject zero puzzle offset")
	}
}
