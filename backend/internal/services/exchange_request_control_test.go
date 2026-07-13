package services

import (
	"strings"
	"testing"
	"time"

	"caiyun/internal/models"
)

func TestAcquireExchangeSubmitProtectionLocalFallback(t *testing.T) {
	t.Setenv("EXCHANGE_SUBMIT_LOCK_TTL", "50ms")
	resetExchangeRequestControlsForTests()

	account := &models.ExchangeAccount{ID: 1, AccountID: 1001}
	if locked, reason := acquireExchangeSubmitProtection(nil, account, "prize-1"); !locked || reason != "" {
		t.Fatalf("expected first acquire to pass, got locked=%t reason=%q", locked, reason)
	}

	if locked, reason := acquireExchangeSubmitProtection(nil, account, "prize-1"); locked || !strings.Contains(reason, "幂等保护") {
		t.Fatalf("expected duplicate protection, got locked=%t reason=%q", locked, reason)
	}

	time.Sleep(70 * time.Millisecond)
	if locked, reason := acquireExchangeSubmitProtection(nil, account, "prize-1"); !locked || reason != "" {
		t.Fatalf("expected lock to expire, got locked=%t reason=%q", locked, reason)
	}
}

func TestLocalExchangeSubmitLocksJanitorCleansExpiredEntries(t *testing.T) {
	t.Setenv("EXCHANGE_SUBMIT_LOCK_CLEANUP_INTERVAL", "10ms")
	locks := newLocalExchangeSubmitLocks()
	defer locks.Stop()

	if !locks.Acquire("account:prize", 15*time.Millisecond) {
		t.Fatal("expected first acquire to succeed")
	}
	time.Sleep(60 * time.Millisecond)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		locks.mu.Lock()
		size := len(locks.expires)
		locks.mu.Unlock()
		if size == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected janitor to clean expired local submit locks")
}

func TestExchangeRequestControllerStopIsIdempotent(t *testing.T) {
	controller := newExchangeRequestController()
	controller.Stop()
	controller.Stop()
}

func TestExchangeFailureClassificationAndRetryability(t *testing.T) {
	cases := []struct {
		message   string
		label     string
		retryable bool
	}{
		{message: "奖品单日已耗尽", label: "sold_out", retryable: false},
		{message: "云朵不足", label: "insufficient_cloud", retryable: false},
		{message: "Token 无效", label: "auth", retryable: false},
		{message: "幂等保护：5 秒内已有相同账号的同商品请求正在处理，请稍后重试", label: "duplicate_window", retryable: true},
		{message: "请求失败：dial tcp timeout", label: "timeout", retryable: true},
		{message: "请求返回异常 | http_status=429", label: "rate_limited", retryable: true},
		{message: "商品已下架或不存在，请更新商品列表后重新创建抢兑任务", label: "product_unavailable", retryable: false},
	}

	for _, tc := range cases {
		if got := exchangeFailureReasonLabel(tc.message); got != tc.label {
			t.Fatalf("label mismatch for %q: got %q want %q", tc.message, got, tc.label)
		}
		if got := isExchangeRetryableFailure(tc.message); got != tc.retryable {
			t.Fatalf("retryability mismatch for %q: got %t want %t", tc.message, got, tc.retryable)
		}
	}
}
