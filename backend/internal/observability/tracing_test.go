package observability

import "testing"

func TestTraceSampleRatio(t *testing.T) {
	production, err := traceSampleRatio("", "production")
	if err != nil || production != 0.10 {
		t.Fatalf("production ratio = %v, %v", production, err)
	}
	development, err := traceSampleRatio("", "development")
	if err != nil || development != 1 {
		t.Fatalf("development ratio = %v, %v", development, err)
	}
	if _, err := traceSampleRatio("1.1", "production"); err == nil {
		t.Fatal("traceSampleRatio accepted out-of-range sample ratio")
	}
}
