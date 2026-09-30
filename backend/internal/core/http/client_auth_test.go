package http

import "testing"

func TestSetAuthAcceptsCaseInsensitiveBasicScheme(t *testing.T) {
	for _, value := range []string{"Basic abc", "basic abc", "BASIC abc", "abc"} {
		client := NewClient()
		client.SetAuth(value)
		if got := client.buildHeaders("https://mail.10086.cn", nil)["Authorization"]; got != "Basic abc" {
			t.Fatalf("SetAuth(%q) produced %q", value, got)
		}
	}
}
