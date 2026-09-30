package api

import "testing"

func TestRedInviteRiskPassed(t *testing.T) {
	cases := []struct {
		name string
		resp *CaiyunResponse
		want bool
	}{
		{
			name: "放行",
			resp: &CaiyunResponse{Result: map[string]interface{}{
				"returnCode": "0", "success": true, "body": float64(0),
			}},
			want: true,
		},
		{
			name: "风控闸门 body=999",
			resp: &CaiyunResponse{Result: map[string]interface{}{
				"returnCode": "0", "returnMsg": "操作成功", "body": float64(999), "success": false,
			}},
			want: false,
		},
		{
			name: "success=false",
			resp: &CaiyunResponse{Result: map[string]interface{}{"success": false}},
			want: false,
		},
		{
			name: "returnCode 非 0",
			resp: &CaiyunResponse{Result: map[string]interface{}{"returnCode": "500"}},
			want: false,
		},
		{
			name: "空响应",
			resp: nil,
			want: false,
		},
	}
	for _, tc := range cases {
		if got := RedInviteRiskPassed(tc.resp); got != tc.want {
			t.Fatalf("%s: RedInviteRiskPassed = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestParseRedInviteMonthInfo(t *testing.T) {
	resp := &CaiyunResponse{Result: map[string]interface{}{
		"newUserCloudNum": float64(500),
		"oldUserCloudNum": float64(20),
		"newUserRedNum":   "0.99",
		"oldUserRedNum":   "0.2",
		"newUserNum":      float64(1),
		"oldUserNum":      float64(2),
	}}
	info, ok := ParseRedInviteMonthInfo(resp)
	if !ok {
		t.Fatal("ParseRedInviteMonthInfo() failed")
	}
	if info.NewUserCloudNum != 500 || info.OldUserCloudNum != 20 {
		t.Fatalf("unexpected cloud nums: %+v", info)
	}
	if info.NewUserRedNum != "0.99" || info.OldUserRedNum != "0.2" {
		t.Fatalf("unexpected red envelopes: %+v", info)
	}
	if info.NewUserNum != 1 || info.OldUserNum != 2 {
		t.Fatalf("unexpected invite counts: %+v", info)
	}
}

func TestMCloudDayHelpers(t *testing.T) {
	info, ok := ParseMCloudDayActivityInfo(&CaiyunResponse{Result: map[string]interface{}{
		"online":         false,
		"extGiftOnline":  true,
		"blindboxOnline": false,
	}})
	if !ok || info.Online || !info.ExtGiftOnline || info.BlindboxOnline {
		t.Fatalf("unexpected activity info: %+v ok=%v", info, ok)
	}

	if MCloudDayGiftHasStock(&CaiyunResponse{Result: []interface{}{
		map[string]interface{}{"name": "a", "hasStock": false},
		map[string]interface{}{"name": "b", "hasStock": false},
	}}) {
		t.Fatal("hasStock should be false when every entry is out of stock")
	}
	if !MCloudDayGiftHasStock(&CaiyunResponse{Result: map[string]interface{}{
		"list": []interface{}{map[string]interface{}{"hasStock": float64(1)}},
	}}) {
		t.Fatal("hasStock should be true when a nested list entry has stock")
	}

	message := MCloudDayMessage(info, false)
	if message == "" {
		t.Fatal("MCloudDayMessage returned empty string")
	}
}

func TestRafflecodeActiveSessions(t *testing.T) {
	if got := RafflecodeActiveSessions(&CaiyunResponse{Result: map[string]interface{}{
		"rafflecodeConfigList": []interface{}{},
	}}); got != 0 {
		t.Fatalf("active sessions = %d, want 0", got)
	}
	if got := RafflecodeActiveSessions(&CaiyunResponse{Result: map[string]interface{}{
		"rafflecodeConfigList": []interface{}{map[string]interface{}{"id": "1"}, map[string]interface{}{"id": "2"}},
	}}); got != 2 {
		t.Fatalf("active sessions = %d, want 2", got)
	}
}

func TestMeituBackupEligibleAndParsers(t *testing.T) {
	if MeituBackupEligible(0, 1) || MeituBackupEligible(1, 0) || MeituBackupEligible(0, 0) {
		t.Fatal("MeituBackupEligible should require both statuses to be 1")
	}
	if !MeituBackupEligible(1, 1) {
		t.Fatal("MeituBackupEligible(1,1) should be true")
	}

	status, awarded, ok := ParseMeituAuthStatus(&CaiyunResponse{Result: map[string]interface{}{
		"authStatus": float64(0), "isAward": float64(0),
	}})
	if !ok || status != 0 || awarded != 0 {
		t.Fatalf("ParseMeituAuthStatus = (%d,%d,%v)", status, awarded, ok)
	}

	backup, _, ok := ParseMeituBackupStatus(&CaiyunResponse{Result: map[string]interface{}{
		"backupStatus": float64(0), "isAward": float64(0),
	}})
	if !ok || backup != 0 {
		t.Fatalf("ParseMeituBackupStatus = (%d,%v)", backup, ok)
	}

	authTotal, backupTotal, ok := ParseMeituPrizeCount(&CaiyunResponse{Result: map[string]interface{}{
		"authTotalCount": float64(1), "backupTotalCount": float64(1),
	}})
	if !ok || authTotal != 1 || backupTotal != 1 {
		t.Fatalf("ParseMeituPrizeCount = (%d,%d,%v)", authTotal, backupTotal, ok)
	}
}

func TestEmailSmsSwitchEnabled(t *testing.T) {
	cases := []struct {
		result interface{}
		want   bool
		known  bool
	}{
		{nil, false, false},
		{true, true, true},
		{false, false, true},
		{"true", true, true},
		{"FALSE", false, true},
		{"maybe", false, false},
	}
	for _, tc := range cases {
		got, known := EmailSmsSwitchEnabled(&CaiyunResponse{Result: tc.result})
		if got != tc.want || known != tc.known {
			t.Fatalf("EmailSmsSwitchEnabled(%v) = (%v,%v), want (%v,%v)", tc.result, got, known, tc.want, tc.known)
		}
	}
}

func TestNormalizeDataURL(t *testing.T) {
	if _, err := normalizeDataURL(""); err == nil {
		t.Fatal("normalizeDataURL should reject empty payload")
	}
	if _, err := normalizeDataURL("not base64!!"); err == nil {
		t.Fatal("normalizeDataURL should reject invalid base64")
	}
	got, err := normalizeDataURL("aGVsbG8=")
	if err != nil {
		t.Fatalf("normalizeDataURL() error = %v", err)
	}
	if got != "data:image/jpeg;base64,aGVsbG8=" {
		t.Fatalf("normalizeDataURL() = %q", got)
	}
	// 已带前缀的输入应被规范化，而不是二次拼接。
	got, err = normalizeDataURL("data:image/png;base64,aGVsbG8=")
	if err != nil {
		t.Fatalf("normalizeDataURL(with prefix) error = %v", err)
	}
	if got != "data:image/jpeg;base64,aGVsbG8=" {
		t.Fatalf("normalizeDataURL(with prefix) = %q", got)
	}
}

func TestParseUpgradeGiftPrizePool(t *testing.T) {
	entries := ParseUpgradeGiftPrizePool(&CaiyunResponse{Result: map[string]interface{}{
		"prizeId":             "260512001",
		"prizeName":           "移动云盘100G云空间年卡",
		"count":               float64(545823),
		"dailyRemainderCount": float64(99951),
	}})
	if len(entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(entries))
	}
	if entries[0].PrizeName != "移动云盘100G云空间年卡" || entries[0].DailyRemain != 99951 {
		t.Fatalf("unexpected entry: %+v", entries[0])
	}
}

func TestAIStoreAccreditPath(t *testing.T) {
	path := AIStoreAccreditPath(&CaiyunResponse{Result: map[string]interface{}{
		"data": map[string]interface{}{
			"authStatus": float64(1),
			"path":       "root:/myfaverapp/DFbnwaEyAFgA",
		},
	}})
	if path != "root:/myfaverapp/DFbnwaEyAFgA" {
		t.Fatalf("AIStoreAccreditPath = %q", path)
	}
	if AIStoreAccreditPath(&CaiyunResponse{Result: map[string]interface{}{}}) != "" {
		t.Fatal("AIStoreAccreditPath should be empty without path")
	}
}
