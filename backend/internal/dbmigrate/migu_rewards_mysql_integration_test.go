package dbmigrate

import (
	"fmt"
	"os"
	"testing"
	"time"
)

func TestMiguNoteMigrationMySQLPreservesHistoryAndBackfillsReceiptsOnce(t *testing.T) {
	if os.Getenv("CAIYUN_MYSQL_INTEGRATION") != "1" {
		t.Skip("set CAIYUN_MYSQL_INTEGRATION=1 to run MySQL integration tests")
	}
	admin := openMySQLIntegrationDB(t, mysqlIntegrationDSN(t, ""))
	dbName := fmt.Sprintf("caiyun_migu_it_%d", time.Now().UnixNano())
	if err := admin.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci").Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = admin.Exec("DROP DATABASE `" + dbName + "`").Error
		if sqlDB, err := admin.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	db := openMySQLIntegrationDB(t, mysqlIntegrationDSN(t, dbName))
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	messages := []string{
		"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符40",
		"咪咕听歌5.0分钟，已确认领取1/3档，本次AI音符4，其余档位待确认（未重复领取）",
		"咪咕AI音符余额134，本次奖励提示25（非累计余额）",
		"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符0",
		"咪咕听歌已提交，本次AI音符40",
	}
	for _, table := range []string{"task_logs", "task_logs_archive"} {
		if err := db.Exec("CREATE TABLE `" + table + "` (id BIGINT PRIMARY KEY, task_type VARCHAR(50) NOT NULL, message TEXT, cloud_gained BIGINT NOT NULL DEFAULT 0)").Error; err != nil {
			t.Fatal(err)
		}
		for i, message := range messages {
			code := "migu_song"
			if i == 2 {
				code = "migu_assets"
			}
			if err := db.Exec("INSERT INTO `"+table+"` (id, task_type, message, cloud_gained) VALUES (?, ?, ?, ?)", i+1, code, message, 21).Error; err != nil {
				t.Fatal(err)
			}
		}
	}
	content, err := migrationFS.ReadFile("sql/025_migu_note_rewards.sql")
	if err != nil {
		t.Fatal(err)
	}
	for run := 0; run < 2; run++ {
		for _, stmt := range splitSQLStatements(string(content)) {
			if _, err := sqlDB.ExecContext(t.Context(), stmt); err != nil {
				t.Fatalf("migration execution %d: %v", run, err)
			}
		}
		for _, table := range []string{"task_logs", "task_logs_archive"} {
			var rows []struct {
				CloudGained, MiguNotesGained int
			}
			if err := db.Table(table).Select("cloud_gained, migu_notes_gained").Order("id").Scan(&rows).Error; err != nil {
				t.Fatal(err)
			}
			want := []int{40, 4, 0, 0, 0}
			if len(rows) != len(want) {
				t.Fatalf("%s history changed: %+v", table, rows)
			}
			for i, row := range rows {
				if row.CloudGained != 21 || row.MiguNotesGained != want[i] {
					t.Fatalf("run=%d table=%s row=%d: %+v, want 21 clouds and %d notes", run, table, i, row, want[i])
				}
			}
		}
	}
}
