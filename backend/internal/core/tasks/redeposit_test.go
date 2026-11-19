package tasks

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

type redepositRoundTrip func(*http.Request) (*http.Response, error)

func (f redepositRoundTrip) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func redepositReply(req *http.Request, body string) *http.Response {
	return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}
}

func redepositMockClient(t *testing.T, handler func(req *http.Request) *http.Response) *corehttp.Client {
	t.Helper()
	client := corehttp.NewClient(corehttp.WithTransport(redepositRoundTrip(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/portal/mobilecloud/index.html", "/ycloud/visitlog/journaling":
			return redepositReply(req, `{"code":0}`), nil
		}
		if strings.Contains(req.URL.Path, "/deviceprofile/") {
			return redepositReply(req, `{"code":1}`), nil
		}
		resp := handler(req)
		if resp == nil {
			t.Fatalf("unexpected redeposit endpoint: %s", req.URL)
		}
		return resp, nil
	})))
	client.SetDeviceID("Bfixture-device")
	client.SetJWTToken("fixture-jwt")
	return client
}

func TestRedepositRunsFullChainForLinkSlots(t *testing.T) {
	var events []string
	listQueries, clicks, saves, finishes := 0, 0, 0, 0
	client := redepositMockClient(t, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["marketname"] == "newsign_139mail" {
				return redepositReply(req, `{"code":0,"result":[{"id":910,"name":"mail redeposit","state":"WAIT"}]}`)
			}
			return redepositReply(req, `{"code":0,"result":[{"id":900,"name":"redeposit slot","state":"WAIT"},{"id":901,"name":"plain slot","state":"WAIT"},{"id":902,"name":"done slot","state":"FINISH"}]}`)
		case "/ycloud/redeposit/getLinkInfoList":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["pageNum"] != float64(1) || body["pageSize"] != float64(10) {
				t.Fatalf("getLinkInfoList paging = %v", body)
			}
			listQueries++
			switch body["linkType"] {
			case "900":
				if body["marketName"] != "sign_in_3" {
					t.Fatalf("getLinkInfoList market = %v", body["marketName"])
				}
				events = append(events, "list:900")
				return redepositReply(req, `{"code":0,"result":{"result":[{"linkid":"link-900"}]}}`)
			case "910":
				if body["marketName"] != "newsign_139mail" {
					t.Fatalf("getLinkInfoList market = %v", body["marketName"])
				}
				events = append(events, "list:910")
				return redepositReply(req, `{"code":0,"result":{"result":[{"linkid":"link-910"}]}}`)
			case "901":
				events = append(events, "list:901")
				return redepositReply(req, `{"code":0,"result":{"result":[]}}`)
			default:
				t.Fatalf("getLinkInfoList probed a non-WAIT task: %v", body["linkType"])
			}
		case "/ycloud/signin/task/click":
			if req.URL.Query().Get("key") != "task" {
				t.Fatalf("click key = %s", req.URL.Query().Get("key"))
			}
			clicks++
			events = append(events, "click:"+req.URL.Query().Get("id"))
			return redepositReply(req, `{"code":0}`)
		case "/ycloud/redeposit/saveLinkFile":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			saves++
			events = append(events, "save:"+body["linkId"].(string))
			return redepositReply(req, `{"code":0}`)
		case "/ycloud/signin/redeposit/finishRedepositTask":
			query := req.URL.Query()
			finishes++
			events = append(events, "finish:"+query.Get("taskId"))
			if query.Get("linkId") != "link-"+query.Get("taskId") {
				t.Fatalf("finish linkId = %s", query.Get("linkId"))
			}
			return redepositReply(req, `{"code":0}`)
		}
		return nil
	})

	task := NewRedepositTask(client, nil)
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if listQueries != 3 || clicks != 2 || saves != 2 || finishes != 2 {
		t.Fatalf("list=%d clicks=%d saves=%d finishes=%d", listQueries, clicks, saves, finishes)
	}
	want := []string{"list:900", "click:900", "save:link-900", "finish:900", "list:901", "list:910", "click:910", "save:link-910", "finish:910"}
	if strings.Join(events, ",") != strings.Join(want, ",") {
		t.Fatalf("event order = %v, want %v", events, want)
	}
	if task.Pending() {
		t.Fatal("fully confirmed redeposit should not be pending")
	}
	if !strings.Contains(task.Message(), "完成任务2项") || !strings.Contains(task.Message(), "非转存槽1个") {
		t.Fatalf("redeposit message = %s", task.Message())
	}
}

func TestRedepositEmptyListsHaveNoSideEffects(t *testing.T) {
	client := redepositMockClient(t, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			return redepositReply(req, `{"code":0,"result":[{"id":900,"name":"plain slot","state":"WAIT"}]}`)
		case "/ycloud/redeposit/getLinkInfoList":
			return redepositReply(req, `{"code":0,"result":{"result":[]}}`)
		case "/ycloud/signin/task/click", "/ycloud/redeposit/saveLinkFile", "/ycloud/signin/redeposit/finishRedepositTask":
			t.Fatalf("empty link list must not trigger %s", req.URL.Path)
		}
		return nil
	})
	task := NewRedepositTask(client, nil)
	if err := task.Run(); err != nil {
		t.Fatal(err)
	}
	if task.Pending() {
		t.Fatal("empty link list should not be pending")
	}
	if !strings.Contains(task.Message(), "非转存槽2个") || strings.Contains(task.Message(), "转存成功") {
		t.Fatalf("redeposit message = %s", task.Message())
	}
}

func TestRedepositUnconfirmedFinishStaysPending(t *testing.T) {
	client := redepositMockClient(t, func(req *http.Request) *http.Response {
		switch req.URL.Path {
		case "/ycloud/signin/task/taskListV3":
			var body map[string]interface{}
			_ = json.NewDecoder(req.Body).Decode(&body)
			if body["marketname"] == "newsign_139mail" {
				return redepositReply(req, `{"code":0,"result":[]}`)
			}
			return redepositReply(req, `{"code":0,"result":[{"id":900,"name":"redeposit slot","state":"WAIT"}]}`)
		case "/ycloud/redeposit/getLinkInfoList":
			return redepositReply(req, `{"code":0,"result":{"result":[{"linkid":"link-900"}]}}`)
		case "/ycloud/signin/task/click", "/ycloud/redeposit/saveLinkFile":
			return redepositReply(req, `{"code":0}`)
		case "/ycloud/signin/redeposit/finishRedepositTask":
			return redepositReply(req, `{"code":500,"msg":"server busy"}`)
		}
		return nil
	})
	task := NewRedepositTask(client, nil)
	if err := task.Run(); err == nil {
		t.Fatal("unconfirmed finish should surface an error")
	}
	if !task.Pending() {
		t.Fatal("saved but unconfirmed redeposit should stay pending")
	}
	if !strings.Contains(task.Message(), "转存成功1次") {
		t.Fatalf("redeposit message = %s", task.Message())
	}
}
