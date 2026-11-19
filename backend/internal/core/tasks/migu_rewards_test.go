package tasks

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestMiguMutationsRecordNewNoteBalanceDeltaOnce(t *testing.T) {
	for _, code := range []string{"migu_login", "migu_signin", "migu_new_user", "migu_community", "migu_mobile", "migu_noon", "migu_night"} {
		t.Run(code, func(t *testing.T) {
			h := newMiguHarness(t)
			mutations := 0
			h.balance = func(req *http.Request) (*http.Response, error) {
				return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"availableNum":%d}}`, 100+25*mutations)), nil
			}
			h.handler = func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/act-task/query-tasks":
					var items []string
					for _, id := range []string{"xrrw", "qsq", "zyd-sign", "noon-check", "night-check"} {
						items = append(items, fmt.Sprintf(`{"id":%q,"taskName":%q,"completeTime":%d,"rceiveTime":%d}`, id, id, mutations, mutations))
					}
					return miguTaskReply(req, `{"code":"000000","data":[`+strings.Join(items, ",")+`]}`), nil
				case "/act-jbzx/sign/status", "/act-jbzx/new-user-receive/status":
					if code == "migu_signin" && req.URL.Path == "/act-jbzx/new-user-receive/status" {
						return miguTaskReply(req, `{"code":"000000","data":{"signTask":true}}`), nil
					}
					return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"isSign":%t}}`, mutations > 0)), nil
				default:
					mutations++
					return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
				}
			}
			task := h.task(nil)
			hour := 12
			if code == "migu_night" {
				hour = 22
			}
			task.now = func() time.Time { return time.Date(2026, 11, 16, hour-8, 30, 0, 0, time.UTC) }
			if err := task.Run(code); err != nil || task.Pending() || task.NotesGained() != 25 || !strings.Contains(task.Message(), "本次AI音符25") {
				t.Fatalf("err=%v pending=%v notes=%d message=%s", err, task.Pending(), task.NotesGained(), task.Message())
			}
			if err := task.Run(code); err != nil || task.NotesGained() != 0 || mutations != 1 || h.calls["/act-jbzx/ai-bean/query-available"] != 2 || h.calls["/act-jbzx/music-bean-accumulate"] != 0 {
				t.Fatalf("repeat counted old earnings or consumed notices: err=%v notes=%d mutations=%d calls=%v", err, task.NotesGained(), mutations, h.calls)
			}
		})
	}
}

func TestMiguBalanceSnapshotsNeverCountAnExistingOrUnknownBalance(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		want                int
	}{
		{"new notes", `{"availableNum":94}`, `{"availableNum":134}`, 40},
		{"unchanged", `{"availableNum":134}`, `{"availableNum":134}`, 0},
		{"spent notes", `{"availableNum":134}`, `{"availableNum":100}`, 0},
		{"missing before", `{}`, `{"availableNum":134}`, 0},
		{"missing after", `{"availableNum":94}`, `{}`, 0},
		{"invalid negative balance", `{"availableNum":-1}`, `{"availableNum":134}`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMiguHarness(t)
			h.balance = func(req *http.Request) (*http.Response, error) {
				data := tc.before
				if h.calls[req.URL.Path] > 1 {
					data = tc.after
				}
				return miguTaskReply(req, `{"code":"000000","data":`+data+`}`), nil
			}
			h.handler = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/act-jbzx/new-user-receive/status" {
					return miguTaskReply(req, `{"code":"000000","data":{"signTask":true}}`), nil
				}
				return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"isSign":%t}}`, h.calls["/act-jbzx/sign"] > 0)), nil
			}
			task := h.task(nil)
			if err := task.Run("migu_signin"); err != nil || task.Pending() || task.NotesGained() != tc.want {
				t.Fatalf("wallet snapshots changed the completed task or invented earnings: err=%v pending=%v notes=%d", err, task.Pending(), task.NotesGained())
			}
		})
	}
}

func TestMiguSongCountsOnlyVerifiedPositiveClaimReceipts(t *testing.T) {
	t.Setenv("CAIYUN_MIGU_SONG_MINUTES", "0")
	h := newMiguHarness(t)
	h.handler = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/act-jbzx/song-task/v2.0" {
			states := `[ {"id":"SONG_1","status":1},{"id":"SONG_2","status":1},{"id":"SONG_3","status":1} ]`
			if h.calls[req.URL.Path] > 1 {
				states = `[ {"id":"SONG_1","status":2},{"id":"SONG_2","status":1},{"id":"SONG_3","status":2} ]`
			}
			return miguTaskReply(req, `{"code":"000000","data":{"songTime":300000,"tasks":`+states+`}}`), nil
		}
		amount := []int{4, 8, -2}[h.calls["/act-jbzx/receive-task-rewards/v2.0"]-1]
		return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"aiBeanNum":%d}}`, amount)), nil
	}
	task := h.task(nil)
	if err := task.Run("migu_song"); err != nil || !task.Pending() || task.NotesGained() != 4 || !strings.Contains(task.Message(), "本次AI音符4") {
		t.Fatalf("unconfirmed or negative claim was counted: err=%v pending=%v notes=%d message=%s", err, task.Pending(), task.NotesGained(), task.Message())
	}
}
