//go:build cgo

package repository

import (
	"testing"
	"time"

	"caiyun/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestRevenueQueriesIgnoreHistoricalDailySnapshotsWithoutDeletingLogs(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.TaskLog{}); err != nil {
		t.Fatal(err)
	}
	start := todayStart()
	end := start.Add(24 * time.Hour)
	for _, item := range []struct {
		kind string
		gain int
	}{{"receive", 100}, {"signin", 10}, {"todaycloud", 110}, {"today_cloud", 110}} {
		row := models.TaskLog{UserID: 1, AccountID: 2, TaskType: item.kind, Status: "success", CloudGained: item.gain, CreatedAt: start.Add(time.Hour)}
		if err := db.Create(&row).Error; err != nil {
			t.Fatal(err)
		}
	}
	repo := NewTaskLogRepository(db)
	for _, query := range []func() (int, error){func() (int, error) { return repo.GetTotalCloudGainedByAccountID(2) }, func() (int, error) { return repo.GetTotalCloudGainedByUserID(1) }, func() (int, error) { return repo.GetTodayCloudGainedByUserID(1) }} {
		if gain, err := query(); err != nil || gain != 110 {
			t.Fatalf("gain=%d err=%v", gain, err)
		}
	}
	for _, gain := range []int{repo.GetCloudGainedByAccountAndRange(2, start, end), repo.GetCloudGainedByUserAndRange(1, start, end), repo.GetCloudGainedGlobal(start, end)} {
		if gain != 110 {
			t.Fatalf("daily snapshots inflated revenue: %d", gain)
		}
	}
	var count int64
	if err := db.Model(&models.TaskLog{}).Count(&count).Error; err != nil || count != 4 {
		t.Fatalf("history changed: count=%d err=%v", count, err)
	}
}
