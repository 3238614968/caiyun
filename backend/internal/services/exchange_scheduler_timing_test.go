package services

import (
	"testing"
	"time"

	"caiyun/internal/constants"
)

func TestMarkPreparedOnceDedupsPerSlot(t *testing.T) {
	s := &ExchangeScheduler{
		preparedSlots:  make(map[string]bool),
		scheduledFires: make(map[string]*time.Timer),
	}
	now := time.Date(2026, 3, 17, 11, 59, 30, 0, time.Local) // 12:00 槽位的准备时刻

	if !s.markPreparedOnce(now, 12, 0) {
		t.Fatal("markPreparedOnce() first call = false, want true")
	}
	if s.markPreparedOnce(now, 12, 0) {
		t.Fatal("markPreparedOnce() duplicate call = true, want false")
	}
	if !s.markPreparedOnce(now, 12, 30) {
		t.Fatal("markPreparedOnce() different slot = false, want true")
	}
}

func TestSchedulePreciseFireRegistersTimerBeforeSlot(t *testing.T) {
	s := &ExchangeScheduler{
		preparedSlots:  make(map[string]bool),
		scheduledFires: make(map[string]*time.Timer),
		stopChan:       make(chan struct{}),
	}
	defer close(s.stopChan)

	now := time.Now()
	// 选下一个整分边界作为抢兑槽位，并保证至少 15 秒余量，避免精确触发器
	// 在断言执行期间回调（回调会访问未初始化的 repo）。
	slotStart := now.Truncate(time.Minute).Add(time.Minute)
	if time.Until(slotStart) < 15*time.Second {
		slotStart = slotStart.Add(time.Minute)
	}
	slotHour, slotMinute := slotStart.Hour(), slotStart.Minute()
	prepareTime := slotStart.Add(-time.Duration(constants.ExchangePreInitSeconds) * time.Second)

	s.schedulePreciseFire(prepareTime, slotHour, slotMinute)

	s.queueMutex.RLock()
	timerCount := len(s.scheduledFires)
	s.queueMutex.RUnlock()
	if timerCount != 1 {
		t.Fatalf("scheduledFires len = %d, want 1", timerCount)
	}

	// 重复登记同一槽位不应新增定时器。
	s.schedulePreciseFire(prepareTime, slotHour, slotMinute)
	s.queueMutex.RLock()
	again := len(s.scheduledFires)
	s.queueMutex.RUnlock()
	if again != 1 {
		t.Fatalf("scheduledFires len after duplicate = %d, want 1", again)
	}
}

func TestExchangeRequestControllerAllowsFirstWaveBurst(t *testing.T) {
	t.Setenv("EXCHANGE_REQUEST_PACING_ENABLED", "true")
	t.Setenv("EXCHANGE_REQUESTS_PER_SECOND", "30")
	t.Setenv("EXCHANGE_REQUEST_BURST", "30")

	controller := newExchangeRequestController()
	if controller == nil || !controller.enabled {
		t.Fatal("expected enabled controller")
	}
	defer controller.Stop()

	start := time.Now()
	for i := 0; i < 30; i++ {
		if err := controller.WaitContext(nil); err != nil {
			t.Fatalf("WaitContext() error = %v", err)
		}
	}
	// 一整波 30 个并发请求应在远小于 1 秒内放行。
	if elapsed := time.Since(start); elapsed > 1500*time.Millisecond {
		t.Fatalf("first wave of 30 took %s, want < 1.5s", elapsed)
	}
}

func TestExchangeFireOffsetClamps(t *testing.T) {
	if got := exchangeFireOffset(); got != 0 {
		t.Fatalf("default exchangeFireOffset() = %s, want 0", got)
	}

	t.Setenv("EXCHANGE_FIRE_OFFSET", "1200ms")
	if got := exchangeFireOffset(); got != 1200*time.Millisecond {
		t.Fatalf("exchangeFireOffset() = %s, want 1200ms", got)
	}

	t.Setenv("EXCHANGE_FIRE_OFFSET", "9999s")
	if got := exchangeFireOffset(); got != 5*time.Second {
		t.Fatalf("exchangeFireOffset() over-cap = %s, want clamp to 5s", got)
	}

	t.Setenv("EXCHANGE_FIRE_OFFSET", "-9999s")
	if got := exchangeFireOffset(); got != -5*time.Second {
		t.Fatalf("exchangeFireOffset() under-cap = %s, want clamp to -5s", got)
	}
}
