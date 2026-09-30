package services

import (
	"caiyun/internal/models"
	"testing"
	"time"
)

func TestPartialHistoricalSnapshotsDoNotBecomeMillionCloudGrowth(t *testing.T) {
	now := time.Date(2026, 9, 30, 17, 0, 0, 0, cstZone)
	stats := []*models.CloudStats{{Date: "2026-09-29", CloudCount: 302795, SampledAccounts: 20}}
	points := completeTrendDataAt(stats, 7, 1361698, 100, now)
	if points[5].HasData || points[6].Comparable || points[6].CloudDiff != 0 {
		t.Fatalf("partial cohort invented growth: %+v", points)
	}
	if points[0].HasData || points[0].CloudCount != 0 {
		t.Fatalf("missing history was backfilled: %+v", points[0])
	}
	if points[6].CloudCount != 1361698 || !points[6].HasData {
		t.Fatal("current balance should stay accurate")
	}
}

func TestCompleteComparableSnapshotsIncludeExchangeSpending(t *testing.T) {
	now := time.Date(2026, 9, 30, 17, 0, 0, 0, cstZone)
	points := completeTrendDataAt([]*models.CloudStats{{Date: "2026-09-29", CloudCount: 1000, SampledAccounts: 2}}, 2, 850, 2, now)
	if !points[1].Comparable || points[1].CloudDiff != -150 {
		t.Fatalf("net balance diff=%+v", points[1])
	}
}

func TestNewAccountsDoNotCountAsTaskEarnings(t *testing.T) {
	accounts := []*models.Account{{ID: 1, CloudCount: 1010}, {ID: 2, CloudCount: 1000000}}
	previous := []*models.CloudStats{{AccountID: 1, CloudCount: 1000}, {AccountID: 3, CloudCount: 400}}
	if got := comparableCloudBalanceDiff(accounts, previous); got != 10 {
		t.Fatalf("new/deleted account balances counted as earned clouds: %d", got)
	}
}
