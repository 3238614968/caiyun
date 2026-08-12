package services

import (
	"testing"
	"time"
)

func TestExchangeSchedulerStopCancelsAndWaitsForExecutionContext(t *testing.T) {
	scheduler := NewExchangeScheduler(nil, nil, nil, nil, nil, nil, nil)
	done := make(chan struct{})
	scheduler.executionWG.Add(1)
	go func() {
		defer scheduler.executionWG.Done()
		<-scheduler.executionContext().Done()
		close(done)
	}()

	stopped := make(chan struct{})
	go func() {
		scheduler.Stop()
		close(stopped)
	}()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("scheduler execution context was not canceled")
	}
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("scheduler Stop did not wait for active execution cleanup")
	}
}
