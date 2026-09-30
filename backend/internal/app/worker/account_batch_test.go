package worker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestAccountBatchRunsIndependentAccountsConcurrentlyOnceAndKeepsFailures(t *testing.T) {
	started := make(chan uint, 4)
	release := make(chan struct{})
	done := make(chan error, 1)
	var active, peak, calls atomic.Int32
	wantErr := errors.New("account failed")
	go func() {
		done <- runAccountBatch(context.Background(), []uint{1, 2, 1, 3}, 2, func(id uint) error {
			calls.Add(1)
			n := active.Add(1)
			for old := peak.Load(); n > old && !peak.CompareAndSwap(old, n); old = peak.Load() {
			}
			started <- id
			<-release
			active.Add(-1)
			if id == 2 {
				return wantErr
			}
			return nil
		})
	}()
	for i := 0; i < 2; i++ {
		select {
		case <-started:
		case <-time.After(time.Second):
			close(release)
			t.Fatal("accounts still serialized")
		}
	}
	close(release)
	if err := <-done; !errors.Is(err, wantErr) || calls.Load() != 3 || peak.Load() != 2 {
		t.Fatalf("calls=%d peak=%d err=%v", calls.Load(), peak.Load(), err)
	}
}
