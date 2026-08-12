package worker

import (
	"io"
	"log"
	"testing"

	"caiyun/internal/scheduler"
)

func TestRegisterWorkerJobCriticality(t *testing.T) {
	scheduler := scheduler.NewScheduler(scheduler.Config{Logger: log.New(io.Discard, "", 0)})
	defer scheduler.Stop()

	registered, err := registerWorkerJob(scheduler, "critical", "bad cron", JobCritical, "critical", func() error { return nil })
	if err == nil || registered {
		t.Fatalf("critical invalid schedule = registered:%t err:%v", registered, err)
	}

	registered, err = registerWorkerJob(scheduler, "optional", "bad cron", JobNonCritical, "optional", func() error { return nil })
	if err != nil || registered {
		t.Fatalf("non-critical invalid schedule = registered:%t err:%v", registered, err)
	}
}
