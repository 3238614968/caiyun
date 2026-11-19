package dbmigrate

import (
	"regexp"
	"strings"
	"testing"
)

func TestMiguHistoricalRewardBackfillSelectsOnlyVerifiedSongReceipts(t *testing.T) {
	content, err := migrationFS.ReadFile("sql/025_migu_note_rewards.sql")
	if err != nil {
		t.Fatal(err)
	}
	patterns := regexp.MustCompile("REGEXP '([^']+)'").FindAllSubmatch(content, -1)
	if len(patterns) != 2 {
		t.Fatal("live and archived history must both have a guarded backfill")
	}
	for _, pattern := range patterns {
		matcher := regexp.MustCompile(string(pattern[1]))
		for _, tc := range []struct {
			message string
			want    bool
		}{
			{"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符40", true},
			{"咪咕听歌5.0分钟，已确认领取1/3档，本次AI音符4，其余档位待确认（未重复领取）", true},
			{"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符0", true},
			{"咪咕AI音符余额134，本次奖励提示25（非累计余额）", false},
			{"咪咕听歌已提交，本次AI音符40", false},
			{"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符-40", false},
			{"咪咕听歌30.0分钟，已确认领取7/7档，本次AI音符9999999999999999999999", false},
		} {
			if got := matcher.MatchString(tc.message); got != tc.want {
				t.Errorf("backfill eligibility for %q = %v, want %v", tc.message, got, tc.want)
			}
		}
	}
	statements := splitSQLStatements(string(content))
	if len(statements) != 7 || !strings.Contains(statements[1], "CREATE PROCEDURE") || !strings.Contains(statements[5], "UPDATE `task_logs`") || !strings.Contains(statements[6], "UPDATE `task_logs_archive`") {
		t.Fatalf("migration parser broke procedure/backfill boundaries: %v", statements)
	}
}
