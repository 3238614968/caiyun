package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type mailRoundTripFunc func(*http.Request) (*http.Response, error)

func (f mailRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func mailTestResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
		Request:    req,
	}
}

func TestMail139LoginComposeAndReportProtocol(t *testing.T) {
	var ssoSeen, loginSeen, composeSeen, reportSeen bool
	transport := mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Fatal(err)
		}
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			ssoSeen = true
			var payload map[string]string
			if err := json.Unmarshal(body, &payload); err != nil || payload["toSourceId"] != "001003" || payload["account"] != "13800000001" {
				t.Fatalf("mail SSO payload = %s, err = %v", body, err)
			}
			if req.Header.Get("Main-Authorization") != "Basic test-auth" {
				t.Fatal("mail SSO request missing account authorization")
			}
			return mailTestResponse(req, 200, `{"data":{"token":"mail-sso"}}`), nil
		case "/login/inlogin.action":
			loginSeen = true
			if !strings.Contains(string(body), `<string name="clientId">10805</string>`) ||
				!strings.Contains(string(body), `<string name="token">mail-sso</string>`) {
				t.Fatalf("mail login XML = %s", body)
			}
			return mailTestResponse(req, 200, `{"code":"S_OK","var":{"sid":"mail-sid","rmkey":"mail-rmkey"}}`), nil
		case "/RmWeb/mail":
			composeSeen = true
			if req.Header.Get("Cookie") != "RMKEY=mail-rmkey" || !strings.Contains(req.URL.RawQuery, "func=mbox:compose") ||
				!strings.Contains(string(body), `13800000002@139.com`) || !strings.Contains(string(body), `&amp;`) {
				t.Fatalf("compose protocol mismatch: url=%s body=%s", req.URL, body)
			}
			return mailTestResponse(req, 200, `{"code":"S_OK","var":{"tid":"delivery-id"}}`), nil
		case "/mw2/file/disk":
			reportSeen = true
			return mailTestResponse(req, 500, `upstream unavailable`), nil
		default:
			t.Fatalf("unexpected upstream path: %s", req.URL.Path)
			return nil, nil
		}
	})
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	api := NewCaiyunAPI(client)
	session, err := api.Login139Mail("13800000001", "test-auth")
	if err != nil {
		t.Fatal(err)
	}
	if err := api.Send139Mail(session, "13800000001", "13800000002@139.com", "问候", "A&B"); err != nil {
		t.Fatal(err)
	}
	if err := api.Report139MailTask(session); err == nil {
		t.Fatal("HTTP 500 must be reported separately from successful compose")
	}
	if !ssoSeen || !loginSeen || !composeSeen || !reportSeen {
		t.Fatalf("incomplete mail flow: sso=%t login=%t compose=%t report=%t", ssoSeen, loginSeen, composeSeen, reportSeen)
	}
}

func TestMail139PDFAndBillMailActions(t *testing.T) {
	var pdfSeen, listSeen, readSeen, billBackupSeen, allBillBackupSeen bool
	transport := mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		var body []byte
		if req.Body != nil {
			var err error
			body, err = io.ReadAll(req.Body)
			if err != nil {
				t.Fatal(err)
			}
		}
		switch req.URL.Path {
		case "/bmail/s":
			if req.URL.Query().Get("func") != "mail:exportPdf" ||
				req.URL.Query().Get("mid") != "bill-mid-1234567890" ||
				req.URL.Query().Get("sid") != "session" {
				t.Fatalf("export pdf query = %s", req.URL.RawQuery)
			}
			pdfSeen = true
			return mailTestResponse(req, 200, "%PDF-1.7\nfixture"), nil
		case "/mw2/file/disk":
			switch req.URL.Query().Get("func") {
			case "disk:backupAllBillMail":
				allBillBackupSeen = true
			case "disk:backupMail":
				if strings.Contains(string(body), `<string name="mailIds">bill-mid-1234567890</string>`) &&
					strings.Contains(string(body), `<string name="channel">10242000</string>`) {
					billBackupSeen = true
				}
			}
			return mailTestResponse(req, 200, `{"code":"S_OK"}`), nil
		case "/RmWeb/mail":
			switch req.URL.Query().Get("func") {
			case "mbox:listMessages":
				listSeen = true
				if req.URL.Query().Get("sid") != "session" {
					t.Fatalf("list sid = %s", req.URL.RawQuery)
				}
				return mailTestResponse(req, 200,
					`{'code':'S_OK','var':[{'mid':"bill-mid-1234567890",'subject':"您的8月账单"}]}`), nil
			case "mbox:readMessage":
				readSeen = true
				if !strings.Contains(string(body), `<string name="mid">bill-mid-1234567890</string>`) {
					t.Fatalf("read body = %s", body)
				}
				return mailTestResponse(req, 200, `{'code':'S_OK'}`), nil
			}
			return mailTestResponse(req, 200, `{'code':'S_OK'}`), nil
		default:
			t.Fatalf("unexpected mail path: %s", req.URL.Path)
			return nil, nil
		}
	})
	client := corehttp.NewClient(corehttp.WithTransport(transport))
	api := NewCaiyunAPI(client)
	session := &Mail139Session{SID: "session", RMKey: "rmkey"}

	raw, err := api.List139MailMessages(session, 1, 30)
	if err != nil {
		t.Fatal(err)
	}
	messages := Parse139MailList(raw)
	bill := Find139BillMail(messages)
	if bill == nil || bill.MID != "bill-mid-1234567890" {
		t.Fatalf("parsed messages = %+v", messages)
	}
	if err := api.Export139MailPDF(session, bill.MID, "mcloud-task"); err != nil {
		t.Fatal(err)
	}
	if err := api.BackupAllBillMail(session); err != nil {
		t.Fatal(err)
	}
	if err := api.Read139MailMessage(session, bill.MID); err != nil {
		t.Fatal(err)
	}
	if err := api.Backup139MailMessages(session, bill.MID, "10242000"); err != nil {
		t.Fatal(err)
	}
	if !pdfSeen || !listSeen || !readSeen || !billBackupSeen || !allBillBackupSeen {
		t.Fatalf("pdf=%v list=%v read=%v backup=%v allBill=%v", pdfSeen, listSeen, readSeen, billBackupSeen, allBillBackupSeen)
	}
}

func TestParse139MailListHandlesQuotedAndDoubleQuotedKeys(t *testing.T) {
	raw := `{"mailList":[{"mid":"0123456789abcdef","subject":"话费提醒"},` +
		`{'mid':'fedcba9876543210','subject':'促销'}]}`
	messages := Parse139MailList(raw)
	if len(messages) != 2 || messages[0].MID != "0123456789abcdef" || messages[0].Subject != "话费提醒" ||
		messages[1].MID != "fedcba9876543210" || messages[1].Subject != "促销" {
		t.Fatalf("messages = %+v", messages)
	}
	if bill := Find139BillMail(messages); bill == nil || bill.MID != "0123456789abcdef" {
		t.Fatalf("bill mail = %+v", bill)
	}
	if Find139BillMail([]Mail139ListMessage{{Subject: "欢迎使用"}}) != nil {
		t.Fatal("non-bill mail was matched")
	}
}

func TestParse139MailListRealRmWebShape(t *testing.T) {
	// 取自 2026-10-08 的真实 mbox:listMessages 响应形状：
	// 外层 'var' 数组、单引号键、双引号 mid，字段之间夹着大量无关键。
	raw := `{'code':'S_OK','var':[{'mid':"6d82efd1db80e73000000452",` +
		`'fid':1,'imapId':10150,'mailSession':80,'size':683,` +
		`'sendDate':1791456839,'subject':'您的8月账单',` +
		`"flagLongValue":16385},` +
		`{'mid':"6a04bd15bb749f0b00000001",'fid':1,'subject':'促销'}]}`
	messages := Parse139MailList(raw)
	if len(messages) != 2 ||
		messages[0].MID != "6d82efd1db80e73000000452" || messages[0].Subject != "您的8月账单" ||
		messages[1].MID != "6a04bd15bb749f0b00000001" || messages[1].Subject != "促销" {
		t.Fatalf("messages = %+v", messages)
	}
	if bill := Find139BillMail(messages); bill == nil || bill.MID != "6d82efd1db80e73000000452" {
		t.Fatalf("bill mail = %+v", bill)
	}
}

func TestParse139MailListKeepsAlignmentWhenSubjectMissing(t *testing.T) {
	raw := `[{'mid':"aaaaaaaaaaaaaaaaaaaa",'fid':1},` +
		`{'mid':"bbbbbbbbbbbbbbbbbbbb",'subject':'账单提醒'}]`
	messages := Parse139MailList(raw)
	if len(messages) != 2 || messages[0].Subject != "" || messages[1].Subject != "账单提醒" {
		t.Fatalf("messages = %+v", messages)
	}
	if bill := Find139BillMail(messages); bill == nil || bill.MID != "bbbbbbbbbbbbbbbbbbbb" {
		t.Fatalf("bill mail = %+v", bill)
	}
}

func TestParse139MailListAcceptsLegacyShortMid(t *testing.T) {
	// 旧邮件 mid 可能是 8~14 位纯数字；参考探针单独用 \d{8,} 匹配过。
	raw := `[{'mid':"12345678",'subject':'话费账单'}]`
	messages := Parse139MailList(raw)
	if len(messages) != 1 || messages[0].MID != "12345678" || messages[0].Subject != "话费账单" {
		t.Fatalf("messages = %+v", messages)
	}
}
