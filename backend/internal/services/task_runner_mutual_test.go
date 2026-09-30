package services

import (
	"testing"

	"caiyun/internal/core/tasks"
)

func TestSelectCyclicAssistPeer(t *testing.T) {
	peers := []tasks.MailPeer{{AccountID: 3}, {AccountID: 1}, {AccountID: 4}, {AccountID: 2}}
	for _, test := range []struct {
		target uint
		want   uint
	}{
		{target: 1, want: 2},
		{target: 2, want: 3},
		{target: 3, want: 4},
		{target: 4, want: 1},
	} {
		if got := selectCyclicAssistPeer(peers, test.target); got == nil || got.AccountID != test.want {
			t.Fatalf("target %d helper = %+v, want %d", test.target, got, test.want)
		}
	}
}
