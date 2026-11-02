package tasks

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestMailActivityRegistersBeforeSendingAndConfirmsBothTasks(t *testing.T) {
	registered, sent, reported, backedUp := false, false, false, false
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			state1, state2 := "WAIT", "WAIT"
			if reported {
				state1 = "FINISH"
			}
			if backedUp {
				state2 = "FINISH"
			}
			return hiddenRewardReply(req, fmt.Sprintf(`{"code":0,"result":[{"id":1004,"state":%q},{"id":1020,"state":%q}]}`, state1, state2)), nil
		case "/ycloud/signin/task/click":
			registered = true
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			return hiddenRewardReply(req, `{"data":{"token":"fixture-mail-sso"}}`), nil
		case "/login/inlogin.action":
			return hiddenRewardReply(req, `{"code":"S_OK","var":{"sid":"fixture-sid","rmkey":"fixture-key"}}`), nil
		case "/RmWeb/mail":
			if !registered || sent {
				t.Fatal("email sent before prerequisite or sent twice")
			}
			sent = true
			return hiddenRewardReply(req, `{'code':'S_OK','var':{'tid':'fixture-delivery'}}`), nil
		case "/mw2/file/disk":
			if !sent {
				t.Fatal("mail activity reported before real send")
			}
			if req.URL.Query().Get("func") == "disk:backupMail" {
				backedUp = true
			} else {
				reported = true
			}
			return hiddenRewardReply(req, `{'code':'S_OK'}`), nil
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling":
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			if strings.Contains(req.URL.Path, "/deviceprofile/") {
				return hiddenRewardReply(req, `{"code":1}`), nil
			}
			t.Fatalf("unexpected mail flow: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetJWTToken("fixture-jwt")
	client.SetDeviceID("Bfixture-device")
	job := NewMailMutualTask(client, logger.NewLogger(logger.LevelError)).
		SetAccount(7, 1, "13800000001", "fixture-auth").SetPeers([]MailPeer{{AccountID: 2, Phone: "13800000002"}}).
		SetDedupStore(&memoryMailDedup{keys: make(map[string]string)})
	if err := job.Run(); err != nil {
		t.Fatal(err)
	}
	if !sent || !reported || !backedUp || job.Pending() {
		t.Fatalf("mail task not confirmed: sent=%v report=%v backup=%v pending=%v", sent, reported, backedUp, job.Pending())
	}
}

func TestMailTaskPerformsPDFAndBillMailActions(t *testing.T) {
	pdfCalled, listCalled, readCalled, billBackupCalled, allBillCalled := false, false, false, false, false
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			return hiddenRewardReply(req, `{"code":0,"result":[{"id":1014,"state":"WAIT"},{"id":1029,"state":"WAIT"}]}`), nil
		case "/ycloud/signin/task/click":
			return hiddenRewardReply(req, `{"code":0}`), nil
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			return hiddenRewardReply(req, `{"data":{"token":"fixture-mail-sso"}}`), nil
		case "/login/inlogin.action":
			return hiddenRewardReply(req, `{"code":"S_OK","var":{"sid":"fixture-sid","rmkey":"fixture-key"}}`), nil
		case "/bmail/s":
			if req.URL.Query().Get("func") != "mail:exportPdf" ||
				req.URL.Query().Get("mid") != "bill-mid-1234567890" {
				t.Fatalf("export pdf query = %s", req.URL.RawQuery)
			}
			pdfCalled = true
			return hiddenRewardReply(req, "%PDF-1.7\nfixture"), nil
		case "/mw2/file/disk":
			if req.URL.Query().Get("func") == "disk:backupAllBillMail" {
				allBillCalled = true
			}
			if strings.Contains(readRequestBody(t, req), "bill-mid-1234567890") {
				billBackupCalled = true
			}
			return hiddenRewardReply(req, `{'code':'S_OK'}`), nil
		case "/RmWeb/mail":
			switch req.URL.Query().Get("func") {
			case "mbox:listMessages":
				listCalled = true
				return hiddenRewardReply(req, `{'code':'S_OK','var':[{'mid':"bill-mid-1234567890",'subject':"您的8月账单"}]}`), nil
			case "mbox:readMessage":
				readCalled = true
				return hiddenRewardReply(req, `{'code':'S_OK'}`), nil
			}
			t.Fatalf("unexpected mail operation: %s", req.URL.RawQuery)
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling":
			return hiddenRewardReply(req, `{"code":0}`), nil
		default:
			if strings.Contains(req.URL.Path, "/deviceprofile/") {
				return hiddenRewardReply(req, `{"code":1}`), nil
			}
			t.Fatalf("unexpected mail flow: %s", req.URL.Path)
		}
		return nil, nil
	})))
	client.SetJWTToken("fixture-jwt")
	client.SetDeviceID("Bfixture-device")
	job := NewMailMutualTask(client, logger.NewLogger(logger.LevelError)).
		SetAccount(7, 1, "13800000001", "fixture-auth").
		SetDedupStore(&memoryMailDedup{keys: make(map[string]string)})
	if err := job.Run(); err != nil {
		t.Fatal(err)
	}
	if !pdfCalled || !listCalled || !readCalled || !billBackupCalled || !allBillCalled {
		t.Fatalf("pdf=%v list=%v read=%v backup=%v allBill=%v", pdfCalled, listCalled, readCalled, billBackupCalled, allBillCalled)
	}
	if !job.Pending() || !strings.Contains(job.Message(), "PDF 导出成功") ||
		!strings.Contains(job.Message(), "账单邮件全量转存接口已接受") ||
		!strings.Contains(job.Message(), "账单邮件已阅读并转存") {
		t.Fatalf("pending=%v message=%s", job.Pending(), job.Message())
	}
}

func readRequestBody(t *testing.T, req *http.Request) string {
	t.Helper()
	if req.Body == nil {
		return ""
	}
	data, err := io.ReadAll(req.Body)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
