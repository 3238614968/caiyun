//go:build cgo

package repository

import (
	"context"
	"testing"
	"time"

	"caiyun/internal/models"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestDailySummaryDoesNotMultiplyRetriesOrCumulativeSnapshots(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.Account{}, &models.TaskLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&models.Account{ID: 2, UserID: 7, Phone: "13900000001"}).Error; err != nil {
		t.Fatal(err)
	}
	start := todayStart()
	logs := []models.TaskLog{
		{UserID: 7, AccountID: 2, TaskType: "signin", Status: "success", CloudGained: 5},
		{UserID: 7, AccountID: 2, TaskType: "tasklist", Status: "pending", CloudGained: 11},
		{UserID: 7, AccountID: 2, TaskType: "signin", Status: "failed"},
		{UserID: 7, AccountID: 2, TaskType: "signin", Status: "success"},
		{UserID: 7, AccountID: 2, TaskType: "tasklist", Status: "pending"},
		{UserID: 7, AccountID: 2, TaskType: "todaycloud", Status: "success", CloudGained: 500},
		{UserID: 7, AccountID: 2, TaskType: "exchange", Status: "failed"},
		{UserID: 8, AccountID: 3, TaskType: "signin", Status: "success", CloudGained: 999},
		{UserID: 7, AccountID: 2, TaskType: "migu_song", Status: "success", MiguNotesGained: 40},
		{UserID: 7, AccountID: 2, TaskType: "migu_signin", Status: "failed", MiguNotesGained: 5},
		{UserID: 7, AccountID: 2, TaskType: "migu_song", Status: "success"},
		{UserID: 7, AccountID: 2, TaskType: "migu_assets", Status: "success", Message: "咪咕AI音符余额134，本次奖励提示25（非累计余额）"},
		{UserID: 8, AccountID: 3, TaskType: "migu_song", Status: "success", MiguNotesGained: 999},
	}
	for i := range logs {
		logs[i].CreatedAt = start.Add(time.Duration(i+1) * time.Minute)
		if err := db.Create(&logs[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	outside := models.TaskLog{UserID: 7, AccountID: 2, TaskType: "signin", Status: "success", CloudGained: 999, MiguNotesGained: 999, CreatedAt: start.AddDate(0, 0, 1)}
	if err := db.Create(&outside).Error; err != nil {
		t.Fatal(err)
	}
	rows, total, err := NewTaskLogRepository(db).DailySummaries(context.Background(), 7, start, start.AddDate(0, 0, 1), 0, 20)
	if err != nil || total != 1 || len(rows) != 1 {
		t.Fatalf("summary: %+v total=%d err=%v", rows, total, err)
	}
	if row := rows[0]; row.CloudGained != 16 || row.MiguNotesGained != 45 || row.SuccessCount != 4 || row.FailedCount != 1 || row.PendingCount != 1 {
		t.Fatalf("double-counted daily outcomes: %+v", row)
	}
	accountID := uint(2)
	repo := NewTaskLogRepository(db)
	if failed, count, err := repo.FindByFilterInRange(context.Background(), 7, &accountID, "signin", "failed", false, start, start.AddDate(0, 0, 1), true, 0, 20); err != nil || len(failed) != 0 || count != 0 {
		t.Fatalf("resolved retries reappeared as current failures: rows=%v count=%d err=%v", failed, count, err)
	}
	if rewards, count, err := repo.FindByFilterInRange(context.Background(), 7, &accountID, "", "", true, start, start.AddDate(0, 0, 1), false, 0, 20); err != nil || len(rewards) != 4 || count != 4 {
		t.Fatalf("reward filter omitted notes or included balance notifications: rows=%v count=%d err=%v", rewards, count, err)
	}
	var count int64
	if err := db.Model(&models.TaskLog{}).Count(&count).Error; err != nil || count != 14 {
		t.Fatalf("raw history changed: count=%d err=%v", count, err)
	}
}

func TestTaskLogArchivePreservesSeparateNoteRewards(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&models.TaskLog{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec("CREATE TABLE task_logs_archive AS SELECT * FROM task_logs WHERE 1 = 0").Error; err != nil {
		t.Fatal(err)
	}
	entry := &models.TaskLog{UserID: 7, AccountID: 2, TaskType: "migu_song", Status: "success", MiguNotesGained: 40, CreatedAt: todayStart().Add(-time.Hour)}
	if err := db.Create(entry).Error; err != nil {
		t.Fatal(err)
	}
	moved, err := NewHistoryArchiveRepository(db).ArchiveTaskLogsBefore(context.Background(), todayStart(), 100)
	if err != nil || moved != 1 {
		t.Fatalf("archive: moved=%d err=%v", moved, err)
	}
	var archived models.TaskLog
	if err := db.Table("task_logs_archive").Select("migu_notes_gained, cloud_gained").Take(&archived).Error; err != nil || archived.MiguNotesGained != 40 || archived.CloudGained != 0 {
		t.Fatalf("archive mixed currencies or lost notes: %+v err=%v", archived, err)
	}
}
