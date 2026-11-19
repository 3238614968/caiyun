package worker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"testing"
	"time"

	"caiyun/internal/models"
	"caiyun/internal/scheduler"
	"caiyun/internal/services"

	"github.com/robfig/cron/v3"
)

type miguJobFixture struct {
	configs      []*models.TaskConfig
	configErr    error
	accountCount int
	accountReads int
	requests     []services.SubmitOperationRequest
	rows         map[string]bool
	dispatchErr  error
}

func (f *miguJobFixture) List() ([]*models.TaskConfig, error) { return f.configs, f.configErr }

func (f *miguJobFixture) FindActiveAccountsAfterID(lastID uint, limit int) ([]*models.Account, error) {
	f.accountReads++
	var batch []*models.Account
	for i := int(lastID) + 1; i <= f.accountCount && len(batch) < limit; i++ {
		batch = append(batch, &models.Account{ID: uint(i), UserID: 10, IsActive: true})
	}
	return batch, nil
}

func (f *miguJobFixture) Submit(_ context.Context, request services.SubmitOperationRequest) (*models.Operation, bool, error, error) {
	f.requests = append(f.requests, request)
	if f.rows == nil {
		f.rows = map[string]bool{}
	}
	created := !f.rows[request.IdempotencyKey]
	f.rows[request.IdempotencyKey] = true
	return &models.Operation{ID: request.IdempotencyKey}, created, f.dispatchErr, nil
}

func TestMiguWindowDispatchRespectsTaskSwitches(t *testing.T) {
	for _, tc := range []struct {
		enabled, batch, expected bool
	}{{true, true, true}, {true, false, false}, {false, true, false}} {
		f := &miguJobFixture{configs: []*models.TaskConfig{{TaskType: "migu_noon", IsEnabled: tc.enabled, RunInBatch: tc.batch}}, accountCount: 1}
		at := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC) // 12:00 Beijing.
		if err := submitMiguWindowTask(context.Background(), "migu_noon", at, f, f, f); err != nil {
			t.Fatal(err)
		}
		if (len(f.requests) == 1) != tc.expected || (f.accountReads > 0) != tc.expected {
			t.Fatalf("switches ignored: %+v requests=%v", tc, f.requests)
		}
	}
}

func TestMiguWindowDispatchPaginatesAndDeduplicatesByBeijingDate(t *testing.T) {
	f := &miguJobFixture{accountCount: 201}
	at := time.Date(2026, 11, 15, 14, 0, 0, 0, time.UTC) // 22:00 Beijing, host may be UTC.
	for _, tick := range []time.Time{at, at.Add(20 * time.Minute)} {
		if err := submitMiguWindowTask(context.Background(), "migu_night", tick, f, f, f); err != nil {
			t.Fatal(err)
		}
	}
	if len(f.rows) != 201 || f.accountReads != 4 || len(f.requests) != 402 {
		t.Fatalf("pagination/dedupe incorrect: rows=%d reads=%d requests=%d", len(f.rows), f.accountReads, len(f.requests))
	}
	for _, request := range f.requests {
		payload, ok := request.Payload.(services.AccountTaskOperationPayload)
		if !ok || payload.TaskType != "migu_night" || payload.AccountID != request.AccountID || request.UserID != 10 || request.OperationType != models.OperationTypeAccountTask || request.IdempotencyKey != fmt.Sprintf("cron:migu_night:2026-11-15:%d", request.AccountID) {
			t.Fatalf("wrong operation contract: %+v", request)
		}
	}
}

func TestMiguWindowDispatchSkipsOtherHoursAndPropagatesConfigFailure(t *testing.T) {
	f := &miguJobFixture{accountCount: 1, configErr: errors.New("fixture db unavailable")}
	at := time.Date(2026, 11, 15, 5, 0, 0, 0, time.UTC) // 13:00, window closed.
	if err := submitMiguWindowTask(context.Background(), "migu_noon", at, f, f, f); err != nil || f.accountReads != 0 {
		t.Fatal("window closure must skip dispatch")
	}
	if err := submitMiguWindowTask(context.Background(), "migu_noon", at.Add(-time.Hour), f, f, f); err == nil || len(f.requests) != 0 {
		t.Fatal("config failure must not enable the task")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := submitMiguWindowTask(ctx, "migu_noon", at.Add(-time.Hour), f, f, f); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestMiguWindowDispatchKeepsDurableOutboxOnRedisFailure(t *testing.T) {
	f := &miguJobFixture{accountCount: 2, dispatchErr: errors.New("fixture redis unavailable")}
	at := time.Date(2026, 11, 15, 4, 0, 0, 0, time.UTC)
	if err := submitMiguWindowTask(context.Background(), "migu_noon", at, f, f, f); err != nil || len(f.rows) != 2 {
		t.Fatalf("durable queued commands lost: rows=%d err=%v", len(f.rows), err)
	}
}

func TestMiguWindowJobsRegisterWithoutChangingDailySchedule(t *testing.T) {
	s := scheduler.NewScheduler(scheduler.Config{Logger: log.New(io.Discard, "", 0)})
	defer s.Stop()
	if err := addDailyTaskExecutionJob(s, "0 8 * * *", func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	f := &miguJobFixture{}
	registerMiguWindowJobs(context.Background(), s, f, f, f, nil)
	if len(s.GetJobs()) != 3 {
		t.Fatal("daily job replaced or either Migu job missing")
	}
	for _, name := range []string{"daily_task_execution", "migu_noon_checkin", "migu_night_checkin"} {
		if _, err := s.GetJobStatus(name); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.GetJobStatus("migu_window_checkin"); err == nil {
		t.Fatal("shared polling job must not remain alongside the independent jobs")
	}
}

func TestMiguWindowSchedulesStayInsideBeijingWindows(t *testing.T) {
	jobs := miguWindowJobDefinitions()
	if len(jobs) != 2 || jobs[0].taskType != "migu_noon" || jobs[1].taskType != "migu_night" {
		t.Fatalf("unexpected job definitions: %+v", jobs)
	}
	for _, job := range jobs {
		t.Run(job.name, func(t *testing.T) {
			schedule, err := cron.ParseStandard(job.schedule)
			if err != nil {
				t.Fatal(err)
			}
			hourUTC := 4 // 12:00 Beijing.
			if job.taskType == "migu_night" {
				hourUTC = 14
			}
			start := time.Date(2026, 11, 15, hourUTC, 0, 0, 0, time.UTC)
			for _, zone := range []*time.Location{time.UTC, time.FixedZone("other host", -5*3600)} {
				if next := schedule.Next(start.Add(-time.Second).In(zone)); !next.Equal(start) {
					t.Fatalf("window start=%s want=%s", next, start)
				}
				if next := schedule.Next(start.Add(2 * time.Minute).In(zone)); !next.Equal(start.Add(20 * time.Minute)) {
					t.Fatalf("missed/restarted window next=%s", next)
				}
				if next := schedule.Next(start.Add(20 * time.Minute).In(zone)); !next.Equal(start.Add(40 * time.Minute)) {
					t.Fatalf("last check-in must be at minute 40: %s", next)
				}
				if next := schedule.Next(start.Add(40 * time.Minute).In(zone)); !next.Equal(start.AddDate(0, 0, 1)) {
					t.Fatalf("schedule escaped its check-in window: %s", next)
				}
			}
		})
	}
}

func TestMiguWindowJobsNeverDispatchTheOtherCheckin(t *testing.T) {
	for _, job := range miguWindowJobDefinitions() {
		f := &miguJobFixture{accountCount: 1}
		wrongHourUTC := 14
		if job.taskType == "migu_night" {
			wrongHourUTC = 4
		}
		at := time.Date(2026, 11, 15, wrongHourUTC, 0, 0, 0, time.UTC)
		if err := submitMiguWindowTask(context.Background(), job.taskType, at, f, f, f); err != nil || len(f.requests) != 0 || f.accountReads != 0 {
			t.Fatalf("%s dispatched in the other task's window: %v", job.name, err)
		}
	}
}
