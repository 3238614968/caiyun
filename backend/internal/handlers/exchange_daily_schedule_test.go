package handlers

import "testing"

func TestExchangeRequestIgnoresObsoleteOptionsButValidatesTimes(t *testing.T) {
	times, cron, policy, holidays, workdays, err := normalizeExchangeScheduleExtras([]string{"10:00", "16:00", "10:00:00"}, "invalid cron", "invalid policy", []string{"invalid date"}, 42)
	if err != nil || times != "10:00:00,16:00:00" || cron != "" || policy != "all" || holidays != "" || workdays != "" {
		t.Fatalf("legacy options not ignored: %q %q %q %q %q %v", times, cron, policy, holidays, workdays, err)
	}
	invalidDay := 99
	cycle, weekday, day, err := normalizeRestockConfig("invalid cycle", &invalidDay, &invalidDay)
	if err != nil || cycle != "daily" || weekday != nil || day != nil {
		t.Fatal("recurrence options still restrict daily exchange")
	}
	if _, _, _, _, _, err := normalizeExchangeScheduleExtras([]string{"25:00"}, "", "", nil, nil); err == nil {
		t.Fatal("invalid exchange time accepted")
	}
}
