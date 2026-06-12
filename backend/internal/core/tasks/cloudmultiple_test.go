package tasks

import "testing"

func TestIsCloudMultipleAlreadyClaimed(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "claimed", message: "已领取", want: true},
		{name: "claimed with spaces", message: " 云朵 翻倍 已领取 ", want: true},
		{name: "not claimed", message: "访问资源不存在", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCloudMultipleAlreadyClaimed(tt.message); got != tt.want {
				t.Fatalf("isCloudMultipleAlreadyClaimed(%q) = %t, want %t", tt.message, got, tt.want)
			}
		})
	}
}
