package tasks

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
)

func TestAnnualBillAutoBackupPerformsRealActionBeforeRegistration(t *testing.T) {
	for _, actionFails := range []bool{false, true} {
		t.Run(fmt.Sprint(actionFails), func(t *testing.T) {
			backup, registered := false, false
			task := autumnFixture(t, "annual_bill", func(req *http.Request) string {
				switch req.URL.Path {
				case "/ycloud/bill/toDoTask/taskInfos":
					n := 0
					if registered {
						n = 1
					}
					return fmt.Sprintf(`{"code":0,"result":[{"taskCode":"AUTO_BACKUP","totalCount":1,"completedCount":%d}]}`, n)
				case "/ycloud/bill/toDoTask/executeTask":
					if !backup || actionFails {
						t.Fatal("registered backup without successful action")
					}
					registered = true
					return `{"code":0}`
				case "/ycloud/bill/taskLottery/queryCount":
					return `{"code":0,"result":0}`
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return ""
			})
			task.backup = func() error {
				backup = true
				if actionFails {
					return errors.New("backup rejected")
				}
				return nil
			}
			err := task.Run()
			if !backup || registered == actionFails || (err != nil) != actionFails || task.Pending() != actionFails {
				t.Fatalf("backup=%v registered=%v pending=%v err=%v", backup, registered, task.Pending(), err)
			}
		})
	}
}

func TestAnnualBillMissingRecordsCannotBecomeZeroOfZeroSuccess(t *testing.T) {
	for _, initial := range []string{`[]`, `[{"taskCode":"BROWSE_REPORT","totalCount":1}]`} {
		t.Run(initial, func(t *testing.T) {
			reads := 0
			task := autumnFixture(t, "annual_bill", func(req *http.Request) string {
				switch req.URL.Path {
				case "/ycloud/bill/toDoTask/taskInfos":
					reads++
					if reads == 1 {
						return `{"code":0,"result":` + initial + `}`
					}
					return `{"code":0,"result":[]}`
				case "/ycloud/bill/toDoTask/executeTask":
					return `{"code":0}`
				case "/ycloud/bill/taskLottery/queryCount":
					return `{"code":0,"result":0}`
				default:
					t.Fatalf("unexpected %s", req.URL)
				}
				return ""
			})
			if err := task.Run(); err != nil || !task.Pending() {
				t.Fatalf("pending=%v message=%s err=%v", task.Pending(), task.Message(), err)
			}
		})
	}
}

func TestFunAITimedTasksStillRunWhenMainModuleFails(t *testing.T) {
	registers := 0
	job := taskStepTestJob(t, func(req *http.Request) string {
		switch {
		case req.URL.Path == "/deviceprofile/v4":
			return `{"code":1}`
		case strings.HasPrefix(req.URL.Path, "/portal/"):
			return "<html></html>"
		case strings.HasSuffix(req.URL.Path, "/page/getmodule"):
			return `{"code":500,"msg":"main unavailable"}`
		case strings.HasSuffix(req.URL.Path, "/timed/getmodule"):
			return fmt.Sprintf(`{"code":0,"result":[{"moduleId":"timed","taskIds":[{"id":"ailuyinzhuanxie","taskName":"录音转写","isComplete":%d}]}]}`, registers)
		case strings.HasSuffix(req.URL.Path, "/timed/transcriptioncompleted"):
			registers++
			return `{"code":0}`
		case strings.HasSuffix(req.URL.Path, "/zhangtingLottery"):
			return `{"code":500,"msg":"次数已用完"}`
		default:
			t.Fatalf("unexpected %s", req.URL)
		}
		return ""
	})
	task := NewFunAITask(job.client, job.logger)
	err := task.Run()
	if err == nil || !strings.Contains(err.Error(), "main unavailable") || registers != 1 || !strings.Contains(task.Message(), "服务端确认新完成1项") {
		t.Fatalf("registers=%d message=%s err=%v", registers, task.Message(), err)
	}
}
