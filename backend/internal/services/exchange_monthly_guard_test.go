package services

import (
	"testing"
	"time"

	"caiyun/internal/models"
)

func TestExchangeMonthlySeriesUsesProductCategory(t *testing.T) {
	series := exchangeMonthlySeriesForProduct(&models.Product{
		Category:   "音乐类会员",
		PrizedName: "QQ音乐绿钻会员月卡",
	}, "QQ音乐绿钻会员月卡")

	if series.Key != "category:音乐类会员" {
		t.Fatalf("series.Key = %q, want category:音乐类会员", series.Key)
	}
	if series.Label != "音乐类会员" {
		t.Fatalf("series.Label = %q, want 音乐类会员", series.Label)
	}
}

func TestExchangeMonthlySeriesInfersMusicFromPrizeName(t *testing.T) {
	series := exchangeMonthlySeriesForProduct(nil, "QQ音乐绿钻会员月卡")
	if series.Key != "category:音乐类会员" {
		t.Fatalf("series.Key = %q, want category:音乐类会员", series.Key)
	}
}

func TestExchangeMonthlySeriesLockMessages(t *testing.T) {
	lockMessages := []string{
		"本月已兑换同类商品",
		"每月只能兑换一次",
		"重复兑奖",
	}
	for _, message := range lockMessages {
		if !isExchangeMonthlySeriesLockMessage(message) {
			t.Fatalf("message %q should lock current month series", message)
		}
	}

	if isExchangeMonthlySeriesLockMessage("奖品单日已耗尽") {
		t.Fatal("daily sold-out message should not lock the monthly series")
	}
	if isExchangeMonthlySeriesLockMessage("滑块验证参数不能为空") {
		t.Fatal("captcha parameter error should not lock the monthly series")
	}
}

func TestExchangeRecordLocksMonthlySeriesOnSuccess(t *testing.T) {
	record := &models.ExchangeRecord{
		Status:  string(models.ExchangeRecordSuccess),
		Message: "兑换成功",
	}
	if !exchangeRecordLocksMonthlySeries(record) {
		t.Fatal("success exchange record should lock current month series")
	}
}

func TestExchangeMonthlyWindowStartsAtMonthBoundary(t *testing.T) {
	now := time.Date(2026, 6, 11, 16, 0, 1, 0, time.Local)
	start, end := exchangeMonthlyWindow(now)
	if start.Day() != 1 || start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("unexpected month start: %s", start)
	}
	if end.Format("2006-01-02") != "2026-07-01" {
		t.Fatalf("unexpected next month start: %s", end)
	}
}
