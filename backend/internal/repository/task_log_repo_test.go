package repository

import "testing"

func TestNormalizeTaskLogPageUsesSafeBounds(t *testing.T) {
	if offset, limit := normalizeTaskLogPage(-10, 0); offset != 0 || limit != defaultTaskLogPageLimit {
		t.Fatalf("normalizeTaskLogPage(-10,0) = (%d,%d), want (0,%d)", offset, limit, defaultTaskLogPageLimit)
	}
	if offset, limit := normalizeTaskLogPage(5, maxTaskLogPageLimit+1); offset != 5 || limit != defaultTaskLogPageLimit {
		t.Fatalf("normalizeTaskLogPage(5,max+1) = (%d,%d), want (5,%d)", offset, limit, defaultTaskLogPageLimit)
	}
	if offset, limit := normalizeTaskLogPage(3, 99); offset != 3 || limit != 99 {
		t.Fatalf("normalizeTaskLogPage(3,99) = (%d,%d), want (3,99)", offset, limit)
	}
}
