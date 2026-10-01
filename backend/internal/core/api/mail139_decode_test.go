package api

import (
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestMailboxSingleQuotedResponseAndEscapedStrings(t *testing.T) {
	var response struct {
		Code    string                 `json:"code"`
		Message string                 `json:"message"`
		Var     map[string]interface{} `json:"var"`
	}
	body := `{'code':'S_OK','message':'He said "yes", it\'s fine','var':{'tid':'delivery-1','ok':True,'missing':None,'jsonbool':true}}`
	if err := decode139MailResponse(body, &response); err != nil || response.Code != "S_OK" || response.Message != `He said "yes", it's fine` || response.Var["ok"] != true || response.Var["missing"] != nil {
		t.Fatalf("mailbox literal decoding: %+v %v", response, err)
	}
	for _, bad := range []string{`{'code':__import__('os').system('x')}`, `{'code':'S_OK'`, `<html>error</html>`} {
		if err := decode139MailResponse(bad, &response); err == nil {
			t.Fatal("accepted malformed or executable provider text")
		}
	}
	if err := decode139MailResponse(`{"code":"S_OK","var":{"tid":"1"}}`, &response); err != nil {
		t.Fatal(err)
	}
}

func TestMailboxSendReportAndBackupUseCapturedContracts(t *testing.T) {
	session := &Mail139Session{SID: "sid+a/b=", RMKey: "fixture-rmkey"}
	var sends, reports, backups int
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Query().Get("sid") != session.SID {
			t.Fatal("mail SID lost base64 characters")
		}
		switch req.URL.Query().Get("func") {
		case "mbox:compose":
			sends++
			return mailTestResponse(req, 200, `{'code':'S_OK','var':{'tid':'delivery-1'}}`), nil
		case "disk:mailReportToMQ", "disk:backupMail":
			if req.Method != "POST" || req.URL.Path != "/mw2/file/disk" || req.URL.Query().Get("comefrom") != "54" {
				t.Fatalf("wrong mailbox callback: %s %s", req.Method, req.URL)
			}
			if req.URL.Query().Get("func") == "disk:backupMail" {
				backups++
			} else {
				reports++
			}
			return mailTestResponse(req, 200, `{'code':'S_OK','var':null}`), nil
		default:
			t.Fatalf("unexpected mailbox request: %s", req.URL)
		}
		return nil, nil
	})))
	upstream := NewCaiyunAPI(client)
	if err := upstream.Send139Mail(session, "13800000001", "13800000002@139.com", "subject", "content"); err != nil {
		t.Fatal(err)
	}
	if err := upstream.Report139MailTask(session); err != nil {
		t.Fatal(err)
	}
	if err := upstream.Backup139Mail(session); err != nil {
		t.Fatal(err)
	}
	if sends != 1 || reports != 1 || backups != 1 {
		t.Fatalf("mail send was repeated: %d/%d/%d", sends, reports, backups)
	}
}
