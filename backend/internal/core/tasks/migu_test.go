package tasks

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
)

type miguTaskTransport func(*http.Request) (*http.Response, error)

func (f miguTaskTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func miguTaskReply(req *http.Request, raw string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(raw)), Request: req}
}

type miguHarness struct {
	client  *corehttp.Client
	calls   map[string]int
	handler func(*http.Request) (*http.Response, error)
	balance func(*http.Request) (*http.Response, error)
}

func newMiguHarness(t *testing.T) *miguHarness {
	t.Helper()
	h := &miguHarness{calls: map[string]int{}}
	h.client = corehttp.NewClient(corehttp.WithTransport(miguTaskTransport(func(req *http.Request) (*http.Response, error) {
		h.calls[req.URL.Path]++
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			return miguTaskReply(req, `{"success":true,"code":"0000","data":{"token":"fixture-ticket"}}`), nil
		case "/user/h5/token-validate/v3.0":
			return miguTaskReply(req, `{"code":"000000","data":{"userId":"music-user","msisdn":"138****0001","pacmtoken":"fixture-pacm"}}`), nil
		case "/act-task/join-activity", "/user/h5/user-info/v1.0":
			return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
		case "/act-jbzx/ai-bean/query-available":
			if h.balance != nil {
				return h.balance(req)
			}
			return miguTaskReply(req, `{"code":"000000","data":{"availableNum":0}}`), nil
		default:
			if h.handler != nil {
				return h.handler(req)
			}
			t.Fatalf("unexpected fixture request %s", req.URL.Path)
			return nil, fmt.Errorf("unexpected fixture request")
		}
	})))
	return h
}

func (h *miguHarness) task(store Storage) *MiguTask {
	return NewMiguTask(h.client).SetAccountContext("13800000001", "Basic fixture-cloud").SetStorage(store)
}

func TestMiguSignChecksStateBeforeAndAfterWrite(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		writes              int
		pending, fail       bool
	}{
		{"already signed", `{"isSign":true}`, `{"isSign":true}`, 0, false, false},
		{"confirmed", `{"isSign":false}`, `{"isSign":true}`, 1, false, false},
		{"unconfirmed ack", `{"isSign":false}`, `{"isSign":false}`, 1, true, false},
		{"missing state", `{}`, `{}`, 0, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMiguHarness(t)
			h.handler = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/act-jbzx/new-user-receive/status" {
					return miguTaskReply(req, `{"code":"000000","data":{"signTask":true}}`), nil
				}
				data := tc.before
				if h.calls["/act-jbzx/sign"] > 0 {
					data = tc.after
				}
				if req.URL.Path == "/act-jbzx/sign" {
					if req.Method != "POST" {
						t.Fatal("sign must be POST")
					}
					data = `{}`
				} else if req.URL.Path != "/act-jbzx/sign/status" {
					t.Fatal(req.URL.Path)
				}
				return miguTaskReply(req, `{"code":"000000","data":`+data+`}`), nil
			}
			task := h.task(nil)
			err := task.Run("migu_signin")
			if (err != nil) != tc.fail || task.Pending() != tc.pending || h.calls["/act-jbzx/sign"] != tc.writes {
				t.Fatalf("err=%v pending=%v calls=%v message=%s", err, task.Pending(), h.calls, task.Message())
			}
		})
	}
}

func TestMiguNewUserEligibilityAndDailyReceiveStatus(t *testing.T) {
	for _, where := range []string{"status", "claim", "already received", "ordinary sign route"} {
		t.Run(where, func(t *testing.T) {
			h := newMiguHarness(t)
			h.handler = func(req *http.Request) (*http.Response, error) {
				if strings.HasSuffix(req.URL.Path, "/status") {
					if where == "ordinary sign route" {
						return miguTaskReply(req, `{"code":"000000","data":{"signTask":true}}`), nil
					}
					if where == "status" {
						return miguTaskReply(req, `{"code":"200013","info":"非新人"}`), nil
					}
					if where == "already received" {
						return miguTaskReply(req, `{"code":"000000","data":{"currentDay":2,"days":[{"day":1,"receiveStatus":1},{"day":2,"receiveStatus":1}]}}`), nil
					}
					return miguTaskReply(req, `{"code":"000000","data":{"isSign":false}}`), nil
				}
				return miguTaskReply(req, `{"code":"200013","info":"非新人"}`), nil
			}
			task := h.task(nil)
			if err := task.Run("migu_new_user"); err != nil || task.Pending() || (where != "already received" && !task.Skipped()) {
				t.Fatalf("err=%v pending=%v skipped=%v", err, task.Pending(), task.Skipped())
			}
			want := 0
			if where == "claim" {
				want = 1
			}
			if h.calls["/act-jbzx/new-user-receive"] != want {
				t.Fatal("ineligible/already-received gift was submitted")
			}
		})
	}
}

func TestMiguAssetNoticeIsNotReportedAsCumulativeBalance(t *testing.T) {
	h := newMiguHarness(t)
	h.balance = func(req *http.Request) (*http.Response, error) {
		return miguTaskReply(req, `{"code":"000000","data":{"availableNum":58}}`), nil
	}
	h.handler = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/act-jbzx/music-bean-accumulate" {
			notice := 13
			if h.calls[req.URL.Path] > 1 {
				notice = 0
			}
			return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"num":%d}}`, notice)), nil
		}
		return miguTaskReply(req, `{"code":"000000","data":{"availableNum":58}}`), nil
	}
	task := h.task(nil)
	for i := 0; i < 2; i++ {
		if err := task.Run("migu_assets"); err != nil || task.NotesGained() != 0 || !strings.Contains(task.Message(), "AI音符余额58") || strings.Contains(task.Message(), "金币累计") {
			t.Fatalf("err=%v message=%s", err, task.Message())
		}
	}
}

func TestMiguReportUsesCurrentTaskAndConfirmsCounters(t *testing.T) {
	for _, tc := range []struct {
		name, before, after string
		writes              int
		pending, fail       bool
	}{
		{"completed", `[{"id":"qsq","taskName":"社区","completeTime":1}]`, `[]`, 0, false, false},
		{"confirmed", `[{"id":"qsq","taskName":"社区","completeTime":0}]`, `[{"id":"qsq","completeTime":1}]`, 1, false, false},
		{"needs three", `[{"id":"qsq","completeTime":2,"needCompleteTime":3}]`, `[{"id":"qsq","completeTime":3,"needCompleteTime":3}]`, 1, false, false},
		{"missing task", `[{"id":"ggrw","completeTime":0}]`, `[]`, 0, true, false},
		{"missing counter", `[{"id":"qsq"}]`, `[]`, 0, false, true},
		{"ack only", `[{"id":"qsq","completeTime":0}]`, `[{"id":"qsq","completeTime":0}]`, 1, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newMiguHarness(t)
			h.handler = func(req *http.Request) (*http.Response, error) {
				data := tc.before
				if h.calls["/act-task/task-report"] > 0 {
					data = tc.after
				}
				if req.URL.Path == "/act-task/task-report" {
					var payload map[string]string
					_ = json.NewDecoder(req.Body).Decode(&payload)
					if payload["taskId"] != "qsq" {
						t.Fatal("unknown real-app task was reported")
					}
					data = `{}`
				}
				return miguTaskReply(req, `{"code":"000000","data":`+data+`}`), nil
			}
			task := h.task(nil)
			err := task.Run("migu_community")
			if (err != nil) != tc.fail || task.Pending() != tc.pending || h.calls["/act-task/task-report"] != tc.writes {
				t.Fatalf("err=%v pending=%v calls=%v", err, task.Pending(), h.calls)
			}
		})
	}
}

func TestMiguCheckinWindowsUseBeijingTime(t *testing.T) {
	for _, code := range []string{"migu_noon", "migu_night"} {
		for _, hour := range []int{11, 12, 13, 21, 22, 23} {
			t.Run(fmt.Sprintf("%s/%d", code, hour), func(t *testing.T) {
				h := newMiguHarness(t)
				id, allowed := "noon-check", hour == 12
				if code == "migu_night" {
					id, allowed = "night-check", hour == 22
				}
				h.handler = func(req *http.Request) (*http.Response, error) {
					if req.URL.Path == "/act-task/query-tasks" {
						complete := h.calls["/act-task/task-report"]
						return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":[{"id":%q,"completeTime":%d}]}`, id, complete)), nil
					}
					return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
				}
				task := h.task(nil)
				// Give UTC to the task; host timezone must not affect the window.
				task.now = func() time.Time { return time.Date(2026, 11, 15, hour-8, 30, 0, 0, time.UTC) }
				if err := task.Run(code); err != nil || task.Pending() == allowed || (h.calls["/act-task/task-report"] == 1) != allowed {
					t.Fatalf("err=%v pending=%v calls=%v", err, task.Pending(), h.calls)
				}
			})
		}
	}
}

func TestMiguLoginGiftUsesCompletionInsteadOfPresentationReceipt(t *testing.T) {
	for _, received := range []int{0, 1} {
		h := newMiguHarness(t)
		h.handler = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/act-task/query-tasks" {
				return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":[{"id":"server-uuid","taskName":"登录即送","completeTime":%d,"rceiveTime":%d,"extData":{"taskType":"xrrw"}}]}`, h.calls["/act-task/login-access"], received)), nil
			}
			return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
		}
		task := h.task(nil)
		if err := task.Run("migu_login"); err != nil || task.Pending() || task.NotesGained() != 0 || h.calls["/act-task/login-access"] != 1 {
			t.Fatalf("err=%v pending=%v calls=%v", err, task.Pending(), h.calls)
		}
	}
}

func decodeMiguBody(t *testing.T, value string) []byte {
	t.Helper()
	raw, err := hex.DecodeString(value)
	if err != nil {
		t.Fatal(err)
	}
	key := "ccTWaprX2aWmTIgA"
	for i := range raw {
		raw[i] -= key[i%len(key)]
	}
	return raw
}

func TestMiguSongTopupClaimsAllEligibleTiersAndIsRepeatable(t *testing.T) {
	h := newMiguHarness(t)
	songTime := int64(60000)
	states := []int{2, 0, 0, 0, 0, 0, 0}
	var reportCE string
	h.handler = func(req *http.Request) (*http.Response, error) {
		if reportCE != "" && req.Header.Get("Ce") != reportCE {
			t.Fatal("song accounting identity changed")
		}
		switch req.URL.Path {
		case "/act-jbzx/song-task/v2.0":
			var tiers []map[string]interface{}
			for i, state := range states {
				tiers = append(tiers, map[string]interface{}{"id": fmt.Sprintf("SONG_%d", i+1), "status": state})
			}
			data, _ := json.Marshal(map[string]interface{}{"songTime": songTime, "tasks": tiers})
			return miguTaskReply(req, `{"code":"000000","data":`+string(data)+`}`), nil
		case "/act-message/report/song-duration/v1":
			body, _ := io.ReadAll(req.Body)
			var report struct {
				Duration      int64
				TransactionID string
			}
			if json.Unmarshal(decodeMiguBody(t, string(body)), &report) != nil || report.Duration != 1740000 || len(report.TransactionID) != 36 {
				t.Fatalf("invalid topup %+v", report)
			}
			reportCE = req.Header.Get("Ce")
			songTime += report.Duration
			for i := range states {
				if states[i] == 0 {
					states[i] = 1
				}
			}
		case "/act-jbzx/receive-task-rewards/v2.0":
			var payload struct{ TaskID string }
			_ = json.NewDecoder(req.Body).Decode(&payload)
			var tier int
			_, _ = fmt.Sscanf(payload.TaskID, "SONG_%d", &tier)
			if tier < 1 || tier > 7 || states[tier-1] != 1 {
				t.Fatalf("not claimable/already claimed: %s", payload.TaskID)
			}
			states[tier-1] = 2
			return miguTaskReply(req, `{"code":"000000","data":{"aiBeanNum":2}}`), nil
		default:
			t.Fatal(req.URL.Path)
		}
		return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
	}
	task := h.task(nil)
	for i := 0; i < 2; i++ {
		if err := task.Run("migu_song"); err != nil || task.Pending() || !strings.Contains(task.Message(), "7/7档") {
			t.Fatalf("err=%v pending=%v message=%s", err, task.Pending(), task.Message())
		}
		wantNotes := 12
		if i > 0 {
			wantNotes = 0
		}
		if task.NotesGained() != wantNotes {
			t.Fatalf("run %d booked %d notes, want %d", i, task.NotesGained(), wantNotes)
		}
	}
	if h.calls["/act-message/report/song-duration/v1"] != 1 || h.calls["/act-jbzx/receive-task-rewards/v2.0"] != 6 {
		t.Fatalf("replayed report/reward: %v", h.calls)
	}
}

func TestMiguSongDoesNotClaimWithoutConfirmedProgress(t *testing.T) {
	for _, data := range []string{
		`{"songTime":0,"tasks":[{"id":"SONG_1","status":0}]}`,
		`{"tasks":[{"id":"SONG_1","status":0}]}`,
		`{"songTime":0,"tasks":[{"id":"SONG_1"}]}`,
	} {
		h := newMiguHarness(t)
		h.handler = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/act-jbzx/song-task/v2.0" {
				return miguTaskReply(req, `{"code":"000000","data":`+data+`}`), nil
			}
			return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
		}
		task := h.task(nil)
		err := task.Run("migu_song")
		if h.calls["/act-jbzx/receive-task-rewards/v2.0"] != 0 || (err == nil && !task.Pending()) || h.calls["/act-message/report/song-duration/v1"] > 1 {
			t.Fatalf("err=%v pending=%v calls=%v", err, task.Pending(), h.calls)
		}
	}
}

func TestMiguClaimOnlyModeAndUnconfirmedRewards(t *testing.T) {
	t.Setenv("CAIYUN_MIGU_SONG_MINUTES", "0")
	h := newMiguHarness(t)
	h.handler = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path == "/act-jbzx/song-task/v2.0" {
			return miguTaskReply(req, `{"code":"000000","data":{"songTime":30000,"tasks":[{"id":"SONG_1","status":1},{"id":"SONG_2","status":0}]}}`), nil
		}
		return miguTaskReply(req, `{"code":"200002","info":"已领取"}`), nil
	}
	task := h.task(nil)
	if err := task.Run("migu_song"); err != nil || !task.Pending() || task.NotesGained() != 0 || h.calls["/act-message/report/song-duration/v1"] != 0 || h.calls["/act-jbzx/receive-task-rewards/v2.0"] != 1 {
		t.Fatalf("err=%v pending=%v calls=%v", err, task.Pending(), h.calls)
	}
}

func TestMiguSongCustomTargetDoesNotWaitForLaterTiers(t *testing.T) {
	t.Setenv("CAIYUN_MIGU_SONG_MINUTES", "5")
	h := newMiguHarness(t)
	h.handler = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/act-jbzx/song-task/v2.0" {
			t.Fatalf("completed target caused a mutation: %s", req.URL.Path)
		}
		// Optional presentation fields can be decimal minutes. They must not
		// interfere with the integer accounting/status fields used by the task.
		return miguTaskReply(req, `{"code":"000000","data":{"songTime":300000,"tasks":[{"id":"SONG_1","status":2,"needTime":0.5},{"id":"SONG_2","status":2},{"id":"SONG_3","status":2},{"id":"SONG_4","status":0}]}}`), nil
	}
	task := h.task(nil)
	if err := task.Run("migu_song"); err != nil || task.Pending() {
		t.Fatalf("err=%v pending=%v message=%s", err, task.Pending(), task.Message())
	}
}

func TestMiguSongDoesNotTopupWhenAllRewardsAlreadyClaimed(t *testing.T) {
	h := newMiguHarness(t)
	h.handler = func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/act-jbzx/song-task/v2.0" {
			t.Fatalf("claimed rewards caused a mutation: %s", req.URL.Path)
		}
		return miguTaskReply(req, `{"code":"000000","data":{"songTime":0,"tasks":[{"id":"SONG_1","status":2}]}}`), nil
	}
	task := h.task(nil)
	if err := task.Run("migu_song"); err != nil || task.Pending() {
		t.Fatalf("err=%v pending=%v", err, task.Pending())
	}
}

func TestMiguSessionCacheBindsAccountAuthAndStableDevice(t *testing.T) {
	h := newMiguHarness(t)
	store := NewMemoryStore()
	var devices []string
	h.handler = func(req *http.Request) (*http.Response, error) {
		devices = append(devices, req.Header.Get("Deviceid"))
		numField := "num"
		if strings.Contains(req.URL.Path, "ai-bean") {
			numField = "availableNum"
		}
		return miguTaskReply(req, `{"code":"000000","data":{"`+numField+`":10}}`), nil
	}
	h.balance = h.handler
	first := h.task(store)
	if err := first.Run("migu_assets"); err != nil {
		t.Fatal(err)
	}
	if err := h.task(store).Run("migu_assets"); err != nil || h.calls["/user/h5/token-validate/v3.0"] != 1 || h.calls["/user/h5/user-info/v1.0"] != 1 {
		t.Fatalf("cached session not reused: err=%v calls=%v", err, h.calls)
	}
	changed := h.task(store).SetAccountContext("13800000001", "Basic changed-cloud")
	if err := changed.Run("migu_assets"); err != nil || h.calls["/user/h5/token-validate/v3.0"] != 2 {
		t.Fatalf("changed auth reused stale session: %v", err)
	}
	other := h.task(store).SetAccountContext("13800000002", "Basic other-cloud")
	if err := other.Run("migu_assets"); err != nil || h.calls["/user/h5/token-validate/v3.0"] != 3 {
		t.Fatalf("other account reused session: %v", err)
	}
	if devices[0] != devices[2] || devices[0] != devices[4] || devices[0] == devices[6] {
		t.Fatal("device must be stable per account and isolated across accounts")
	}
	var cached api.MiguSession
	raw, _ := store.Get(changed.sessionKey())
	_ = json.Unmarshal([]byte(raw), &cached)
	cached.CreatedAt = time.Now().Add(-601 * time.Second).Unix()
	encoded, _ := json.Marshal(cached)
	_ = store.Set(changed.sessionKey(), string(encoded))
	if err := h.task(store).SetAccountContext("13800000001", "Basic changed-cloud").Run("migu_assets"); err != nil || h.calls["/user/h5/token-validate/v3.0"] != 4 {
		t.Fatalf("expired session was reused: %v", err)
	}
}
