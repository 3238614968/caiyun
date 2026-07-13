package scheduler

import (
	"io"
	"log"
	"testing"
	"time"
)

func TestStopWaitsForRunningCronJob(t *testing.T) {
	s := NewScheduler(Config{Logger: log.New(io.Discard, "", 0)})
	started := make(chan struct{})
	release := make(chan struct{})
	if _, err := s.cron.AddFunc("@every 1s", func() {
		close(started)
		<-release
	}); err != nil {
		t.Fatalf("add cron job: %v", err)
	}
	s.Start()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("cron job did not start")
	}

	stopped := make(chan struct{})
	go func() {
		s.Stop()
		close(stopped)
	}()

	select {
	case <-stopped:
		t.Fatal("Stop returned before the running cron job completed")
	case <-time.After(50 * time.Millisecond):
	}

	close(release)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not return after the running cron job completed")
	}
}
