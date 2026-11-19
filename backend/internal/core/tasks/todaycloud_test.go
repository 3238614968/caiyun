package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestTodayCloudCountsBeijingDayAndPagesWithoutDoubleCountingIDs(t *testing.T) {
	requests := 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		requests++
		if req.URL.Query().Get("type") != "1" {
			t.Fatal(req.URL)
		}
		page, _ := strconv.Atoi(req.URL.Query().Get("pageNumber"))
		rows := []map[string]interface{}{}
		if page == 1 {
			rows = []map[string]interface{}{
				{"id": "a", "inserttime": "2026-11-10T16:30:00Z", "num": "5"},
				{"id": "b", "updatetime": "2026-11-11 13:00:00", "num": 7},
				{"id": "old", "inserttime": "2026-11-10T15:59:59Z", "num": 100},
				{"id": "spent", "inserttime": "2026-11-11 12:00:00", "num": -3},
			}
		} else if page == 2 {
			rows = []map[string]interface{}{
				{"id": "b", "updatetime": "2026-11-11 13:00:00", "num": 7},
				{"id": "c", "inserttime": "2026-11-11", "num": "3"},
			}
		} else {
			t.Fatal("unbounded pagination")
		}
		data, _ := json.Marshal(map[string]interface{}{"code": "0", "result": map[string]interface{}{"records": rows, "pages": 2}})
		return string(data)
	})
	task := NewTodayCloudTask(job.client, job.logger)
	task.now = func() time.Time { return time.Date(2026, 11, 11, 2, 0, 0, 0, time.UTC) }
	for i := 0; i < 2; i++ {
		if err := task.Run(); err != nil {
			t.Fatal(err)
		}
		if task.Pending() || task.TotalCloud() != 15 || task.TodayCount() != 3 {
			t.Fatalf("pending=%v count=%d total=%d", task.Pending(), task.TodayCount(), task.TotalCloud())
		}
	}
	if requests != 4 {
		t.Fatal(requests)
	}
}

func TestTodayCloudRejectsBusinessErrorsAndMarksUnknownRowsPending(t *testing.T) {
	for _, tc := range []struct {
		body             string
		failure, pending bool
	}{
		{`{"code":"90001","msg":"未登录"}`, true, false},
		{`{"result":{"records":[]}}`, true, false},
		{`{"code":0,"result":{}}`, true, false},
		{`{"code":0,"result":{"records":[{"num":5}]}}`, false, true},
		{`{"code":0,"result":{"records":[]}}`, false, false},
	} {
		t.Run(tc.body, func(t *testing.T) {
			job := taskStepTestJob(t, func(*http.Request) string { return tc.body })
			task := NewTodayCloudTask(job.client, job.logger)
			err := task.Run()
			if (err != nil) != tc.failure || task.Pending() != tc.pending {
				t.Fatalf("pending=%v err=%v", task.Pending(), err)
			}
		})
	}
}

func TestTodayCloudStopsOnRepeatedPageRatherThanInflatingSum(t *testing.T) {
	rows := make([]map[string]interface{}, 300)
	for i := range rows {
		rows[i] = map[string]interface{}{"id": i, "inserttime": "2026-11-11", "num": 1}
	}
	data, _ := json.Marshal(map[string]interface{}{"code": 0, "result": map[string]interface{}{"records": rows}})
	requests := 0
	job := taskStepTestJob(t, func(*http.Request) string { requests++; return string(data) })
	task := NewTodayCloudTask(job.client, job.logger)
	task.now = func() time.Time { return time.Date(2026, 11, 11, 0, 0, 0, 0, time.UTC) }
	if err := task.Run(); err != nil || !task.Pending() || requests != 2 || task.TotalCloud() != 300 {
		t.Fatalf("requests=%d total=%d pending=%v err=%v", requests, task.TotalCloud(), task.Pending(), err)
	}
}

func TestWeChatNonzeroStringCodesNeverBecomeSuccessOrUnboundDiagnosis(t *testing.T) {
	for _, tc := range []struct {
		code, message            string
		failed, skipped, pending bool
	}{
		{"90001", "未登录", true, false, false},
		{"500", "系统繁忙", true, false, false},
		{"602", "活动已结束", false, true, false},
		{"401", "账号未绑定公众号", false, false, true},
	} {
		t.Run(tc.message, func(t *testing.T) {
			job := taskStepTestJob(t, func(*http.Request) string { return fmt.Sprintf(`{"code":%q,"msg":%q}`, tc.code, tc.message) })
			task := NewWeChatTask(job.client, job.logger)
			for _, run := range []func() error{task.RunSignIn, func() error { return task.RunDraw(1) }} {
				err := run()
				if (err != nil) != tc.failed || task.Skipped() != tc.skipped || task.Pending() != tc.pending {
					t.Fatalf("pending=%v skipped=%v err=%v", task.Pending(), task.Skipped(), err)
				}
				if tc.failed && strings.Contains(task.Message(), "绑定") {
					t.Fatal("unrelated failure misdiagnosed as unbound account")
				}
			}
		})
	}
}

func TestWeChatDrawIsFreeBoundedAndRequiresCountChange(t *testing.T) {
	for _, confirmed := range []bool{true, false} {
		t.Run(fmt.Sprint(confirmed), func(t *testing.T) {
			draws := 0
			job := taskStepTestJob(t, func(req *http.Request) string {
				switch req.URL.Path {
				case "/market/playoffic/drawInfo":
					n := 50
					if draws > 0 && confirmed {
						n = 49
					}
					return fmt.Sprintf(`{"code":0,"result":{"surplusNumber":%d}}`, n)
				case "/market/playoffic/draw":
					draws++
					return `{"code":0,"result":{"prizeName":"流量券"}}`
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return ""
			})
			task := NewWeChatTask(job.client, job.logger)
			if err := task.RunDraw(50); err != nil {
				t.Fatal(err)
			}
			if draws != 1 || task.Pending() == confirmed || strings.Contains(task.Message(), "确认1次") != confirmed {
				t.Fatalf("draws=%d message=%s", draws, task.Message())
			}
			if confirmed {
				if err := task.RunDraw(50); err != nil || draws != 1 {
					t.Fatal("a second, paid draw was attempted")
				}
			}
		})
	}
}
