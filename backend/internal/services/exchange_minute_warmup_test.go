package services

import (
	"bytes"
	"context"
	"log"
	"strings"
	"sync"
	"testing"
	"time"

	"caiyun/internal/constants"
	"caiyun/internal/models"
)

func TestEveryExchangeSlotStartsWarmupOneMinuteEarly(t *testing.T) {
	if constants.ExchangePreInitSeconds != 60 {
		t.Fatal("warmup is not one minute")
	}
	for _, slot := range []time.Time{
		time.Date(2026, 10, 1, 10, 0, 0, 0, cstZone),
		time.Date(2026, 10, 1, 16, 0, 0, 0, cstZone),
		time.Date(2026, 10, 1, 17, 51, 0, 0, cstZone),
		time.Date(2026, 10, 2, 0, 0, 0, 0, cstZone),
	} {
		prepare := slot.Add(-time.Minute)
		h, m, ok := scheduledPrepareSlot(prepare)
		if !ok || h != slot.Hour() || m != slot.Minute() {
			t.Fatalf("slot %s starts at wrong time: %02d:%02d", slot, h, m)
		}
		h, m, ok = scheduledPrepareSlot(prepare.Add(27 * time.Second))
		if !ok || h != slot.Hour() || m != slot.Minute() {
			t.Fatal("restart in warmup window skipped slot")
		}
	}
}

func TestExactAndFallbackOnlyDispatchEachTaskOncePerDatedSlot(t *testing.T) {
	s := &ExchangeScheduler{}
	slot := time.Date(2026, 10, 1, 10, 0, 0, 0, cstZone)
	tasks := []*models.ExchangeTask{{ID: 296}, {ID: 352}}
	var wg sync.WaitGroup
	var mu sync.Mutex
	dispatched := 0
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			batch := s.undispatchedSlotTasks(slot, tasks)
			mu.Lock()
			dispatched += len(batch)
			mu.Unlock()
		}()
	}
	wg.Wait()
	if dispatched != 2 {
		t.Fatalf("duplicate dispatches=%d", dispatched)
	}
	if extra := s.undispatchedSlotTasks(slot, []*models.ExchangeTask{{ID: 296}, {ID: 999}}); len(extra) != 1 || extra[0].ID != 999 {
		t.Fatal("late new task was suppressed")
	}
	if len(s.undispatchedSlotTasks(slot.Add(24*time.Hour), tasks)) != 2 {
		t.Fatal("next day tasks were suppressed")
	}
}

func TestIncrementalPreparationReusesWarmSessions(t *testing.T) {
	s := NewExchangeScheduler(nil, nil, nil, nil, nil, nil, nil)
	slot := time.Date(2026, 10, 1, 10, 0, 0, 0, cstZone)
	if !s.markPreparedOnce(slot.Add(-time.Minute), 10, 0) || s.markPreparedOnce(slot.Add(-time.Minute), 10, 0) {
		t.Fatal("parallel preparation was not prevented")
	}
	s.finishPreparingSlot(slot)
	if !s.markPreparedOnce(slot.Add(-30*time.Second), 10, 0) {
		t.Fatal("late-task preparation was not allowed")
	}
	first := &models.ExchangeTask{ID: 1, ExchangeAccount: models.ExchangeAccount{AccountID: 7}}
	second := &models.ExchangeTask{ID: 2, ExchangeAccount: models.ExchangeAccount{AccountID: 8}}
	s.storeWarmSession(1, 7, &exchangePreparedSession{})
	if got := s.tasksNeedingWarmup([]*models.ExchangeTask{first, second}); len(got) != 1 || got[0].ID != 2 {
		t.Fatal("ready tasks were repeatedly initialized")
	}
	s.storeWarmSession(1, 7, &exchangePreparedSession{sourceAuth: "old-auth"})
	first.ExchangeAccount.Auth = "updated-auth"
	if got := s.tasksNeedingWarmup([]*models.ExchangeTask{first}); len(got) != 1 {
		t.Fatal("credentials updated during warmup did not invalidate session")
	}
}

func TestCanceledExchangeBatchDoesNotReportFalseFailures(t *testing.T) {
	var output bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&output)
	t.Cleanup(func() { log.SetOutput(previous) })
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := NewExchangeScheduler(nil, nil, nil, nil, nil, nil, nil)
	s.executeProductGroup(ctx, "251230051", []*models.ExchangeTask{{ID: 296}, {ID: 352}}, make(chan struct{}, 2))
	if !strings.Contains(output.String(), "失败 0 个任务，跳过 2 个任务") {
		t.Fatalf("unexecuted tasks counted as failed: %s", output.String())
	}
}
