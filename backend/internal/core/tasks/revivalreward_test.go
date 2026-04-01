package tasks

import "testing"

func TestIsRevivalRewardAlreadyClaimed(t *testing.T) {
	tests := []struct {
		name    string
		message string
		want    bool
	}{
		{name: "claimed this month", message: "本月已领取复活卡奖励", want: true},
		{name: "claimed generic", message: "您已领取复活卡，请下月再试", want: true},
		{name: "unrelated", message: "活动异常，请稍后重试", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isRevivalRewardAlreadyClaimed(tt.message); got != tt.want {
				t.Fatalf("isRevivalRewardAlreadyClaimed(%q) = %v, want %v", tt.message, got, tt.want)
			}
		})
	}
}

func TestFirstNonEmpty(t *testing.T) {
	if got := firstNonEmpty("", " ", "ok", "later"); got != "ok" {
		t.Fatalf("firstNonEmpty returned %q, want ok", got)
	}
}
