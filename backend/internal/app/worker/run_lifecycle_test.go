package worker

import (
	"io"
	"log"
	"strings"
	"testing"

	"caiyun/internal/scheduler"
)

func TestAddDailyTaskExecutionJobRejectsInvalidSchedule(t *testing.T) {
	jobScheduler := scheduler.NewScheduler(scheduler.Config{Logger: log.New(io.Discard, "", 0)})
	err := addDailyTaskExecutionJob(jobScheduler, "definitely-not-a-cron", func() error { return nil })
	if err == nil {
		t.Fatal("invalid TASK_SCHEDULE was accepted")
	}
	if !strings.Contains(err.Error(), "Worker 拒绝启动") {
		t.Fatalf("error = %q, want fail-fast startup context", err)
	}
}

func TestAddDailyTaskExecutionJobAcceptsValidSchedule(t *testing.T) {
	jobScheduler := scheduler.NewScheduler(scheduler.Config{Logger: log.New(io.Discard, "", 0)})
	if err := addDailyTaskExecutionJob(jobScheduler, "0 8 * * *", func() error { return nil }); err != nil {
		t.Fatalf("valid TASK_SCHEDULE rejected: %v", err)
	}
}
