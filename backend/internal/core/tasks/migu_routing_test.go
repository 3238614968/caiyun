package tasks

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestMiguDailySignUsesNewcomerRouteAndDoesNotClaimTwice(t *testing.T) {
	for _, already := range []bool{false, true} {
		t.Run(fmt.Sprint(already), func(t *testing.T) {
			h := newMiguHarness(t)
			signed := already
			h.balance = func(req *http.Request) (*http.Response, error) {
				notes := 100
				if signed && !already {
					notes += 5
				}
				return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"availableNum":%d}}`, notes)), nil
			}
			h.handler = func(req *http.Request) (*http.Response, error) {
				switch req.URL.Path {
				case "/act-jbzx/new-user-receive/status":
					return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":{"signTask":false,"isSign":%t,"currentDay":2}}`, signed)), nil
				case "/act-jbzx/new-user-receive":
					signed = true
					return miguTaskReply(req, `{"code":"000000","data":{"scoreNum":5}}`), nil
				default:
					t.Fatalf("newcomer account must never query or submit the ordinary sign route: %s", req.URL.Path)
					return nil, nil
				}
			}
			task := h.task(nil)
			wantNotes, wantWrites := 5, 1
			if already {
				wantNotes, wantWrites = 0, 0
			}
			if err := task.Run("migu_signin"); err != nil || task.Pending() || task.NotesGained() != wantNotes || !strings.Contains(task.Message(), "新人礼通道") {
				t.Fatalf("daily sign: err=%v pending=%v notes=%d message=%s", err, task.Pending(), task.NotesGained(), task.Message())
			}
			if err := task.Run("migu_new_user"); err != nil || task.Pending() || task.NotesGained() != 0 || h.calls["/act-jbzx/new-user-receive"] != wantWrites {
				t.Fatalf("new-user task replayed today's sign: err=%v notes=%d calls=%v", err, task.NotesGained(), h.calls)
			}
		})
	}
}

func TestMiguDailySignDoesNotBypassEligibilityOrHideARealRejection(t *testing.T) {
	for _, routeCode := range []string{"000000", "200013", "199999"} {
		t.Run(routeCode, func(t *testing.T) {
			h := newMiguHarness(t)
			h.handler = func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/act-jbzx/new-user-receive/status" {
					return miguTaskReply(req, fmt.Sprintf(`{"code":%q,"data":{"signTask":true}}`, routeCode)), nil
				}
				if req.URL.Path == "/act-jbzx/sign/status" {
					return miguTaskReply(req, `{"code":"000000","data":{"isSign":false}}`), nil
				}
				if req.URL.Path != "/act-jbzx/sign" {
					t.Fatalf("eligibility failure tried another mutation: %s", req.URL.Path)
				}
				return miguTaskReply(req, `{"code":"200013","info":"暂无法参与"}`), nil
			}
			task := h.task(nil)
			err := task.Run("migu_signin")
			wantWrites := 1
			if routeCode == "199999" {
				wantWrites = 0
			}
			if err == nil || task.NotesGained() != 0 || h.calls["/act-jbzx/sign"] != wantWrites {
				t.Fatalf("rejection became success: err=%v notes=%d calls=%v", err, task.NotesGained(), h.calls)
			}
		})
	}
}

func TestMiguLoginGiftCompletedWithoutReceiptIsNotReplayed(t *testing.T) {
	for _, complete := range []int{0, 1} {
		h := newMiguHarness(t)
		h.handler = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/act-task/query-tasks" {
				return miguTaskReply(req, fmt.Sprintf(`{"code":"000000","data":[{"id":"gift","taskName":"登录即送","completeTime":%d,"needCompleteTime":1}]}`, complete)), nil
			}
			if req.URL.Path != "/act-task/login-access" {
				t.Fatalf("behavior gift tried a separate reward claim: %s", req.URL.Path)
			}
			return miguTaskReply(req, `{"code":"000000","data":{}}`), nil
		}
		task := h.task(nil)
		if err := task.Run("migu_login"); err != nil || task.Pending() != (complete == 0) || task.NotesGained() != 0 || h.calls["/act-task/login-access"] != 1-complete {
			t.Fatalf("err=%v pending=%v notes=%d calls=%v", err, task.Pending(), task.NotesGained(), h.calls)
		}
	}
}

func TestMiguVisibleActivityOutsideServerWindowRemainsPending(t *testing.T) {
	for _, info := range []string{"不在任务时间范围内", "暂无法参与"} {
		h := newMiguHarness(t)
		h.handler = func(req *http.Request) (*http.Response, error) {
			if req.URL.Path == "/act-task/query-tasks" {
				return miguTaskReply(req, `{"code":"000000","data":[{"id":"zyd-sign","taskName":"前往中国移动APP","completeTime":0,"needCompleteTime":1}]}`), nil
			}
			return miguTaskReply(req, fmt.Sprintf(`{"code":"200013","info":%q}`, info)), nil
		}
		task := h.task(nil)
		err := task.Run("migu_mobile")
		outside := strings.Contains(info, "不在任务时间范围")
		if (err == nil) != outside || !task.Pending() || task.NotesGained() != 0 || h.calls["/act-task/task-report"] != 1 {
			t.Fatalf("window refusal was lost or a general rejection was hidden: err=%v pending=%v notes=%d calls=%v", err, task.Pending(), task.NotesGained(), h.calls)
		}
		if outside && (!strings.Contains(task.Message(), "本次未完成") || h.calls["/act-jbzx/ai-bean/query-available"] != 1) {
			t.Fatalf("rejected action booked a wallet change: message=%s calls=%v", task.Message(), h.calls)
		}
	}
}
