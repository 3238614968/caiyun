package services

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
	"caiyun/internal/models"
)

type miguRunnerTransport func(*http.Request) (*http.Response, error)

func (f miguRunnerTransport) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestMiguTasksUseExistingCatalogAndResultFormat(t *testing.T) {
	catalog := NewTaskCatalog()
	batch := map[string]bool{}
	for _, code := range catalog.DefaultBatchCodes() {
		batch[code] = true
	}
	for _, definition := range miguTaskDefinitions() {
		def, ok := catalog.Get(definition.Code)
		if !ok || !def.DefaultEnabled || !def.RunInBatch || def.execute == nil || !batch[def.Code] {
			t.Fatalf("missing default activity definition: %+v", def)
		}
	}
	if len(miguTaskDefinitions()) != 9 {
		t.Fatal("expected all nine Migu task entries")
	}
	for _, code := range []string{"signin", "tasklist", "coc3", "coc_web6", "receive", "after_task"} {
		if _, ok := catalog.Get(code); !ok {
			t.Fatalf("existing task removed: %s", code)
		}
	}
	calls := map[string]int{}
	client := corehttp.NewClient(corehttp.WithTransport(miguRunnerTransport(func(req *http.Request) (*http.Response, error) {
		calls[req.URL.Path]++
		body := `{"code":"000000","data":{}}`
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			if req.Header.Get("Authorization") != "Basic complete-account-auth" {
				t.Fatal("runner did not use complete account Authorization")
			}
			body = `{"success":true,"code":"0000","data":{"token":"fixture-ticket"}}`
		case "/user/h5/token-validate/v3.0":
			body = `{"code":"000000","data":{"uid":"fixture-user","pacmtoken":"fixture-music-token"}}`
		case "/act-task/join-activity":
		case "/act-task/query-tasks":
			body = `{"code":"000000","data":[{"id":"gift","taskName":"登录即送","completeTime":1,"rceiveTime":1},{"id":"qsq","completeTime":1},{"id":"zyd-sign","completeTime":1},{"id":"noon-check","completeTime":1},{"id":"night-check","completeTime":1}]}`
		case "/act-jbzx/sign/status":
			body = `{"code":"000000","data":{"isSign":true}}`
		case "/act-jbzx/new-user-receive/status":
			body = `{"code":"200013","info":"不符合新人资格"}`
		case "/act-jbzx/song-task/v2.0":
			var tiers []map[string]interface{}
			for _, id := range []string{"SONG_1", "SONG_2", "SONG_3", "SONG_4", "SONG_5", "SONG_6", "SONG_7"} {
				tiers = append(tiers, map[string]interface{}{"id": id, "status": 2})
			}
			raw, _ := json.Marshal(map[string]interface{}{"songTime": 1800000, "tasks": tiers})
			body = `{"code":"000000","data":` + string(raw) + `}`
		case "/act-jbzx/music-bean-accumulate":
			body = `{"code":"000000","data":{"num":130}}`
		case "/act-jbzx/ai-bean/query-available":
			body = `{"code":"000000","data":{"availableNum":40}}`
		default:
			t.Fatalf("unexpected side effect: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})))
	runner := &TaskRunner{account: &models.Account{Phone: "13800000001", Auth: "Basic complete-account-auth", Token: "stale-raw-token"}, httpClient: client, logger: logger.NewLogger(logger.LevelError)}
	for _, def := range miguTaskDefinitions() {
		result, err := catalog.Execute(runner, def.Code)
		wantStatus := "success"
		if err != nil || result.TaskType != def.Code || result.Status != wantStatus || result.Message == "" || result.CloudGained != 0 || result.MiguNotesGained != 0 {
			t.Fatalf("%s: result=%+v err=%v", def.Code, result, err)
		}
	}
	if calls["/user/h5/token-validate/v3.0"] != 1 || calls["/act-task/join-activity"] != 1 {
		t.Fatalf("batch did not share one music session: %v", calls)
	}
}

func TestMiguRunnerKeepsConfirmedNotesSeparateFromClouds(t *testing.T) {
	signed := false
	client := corehttp.NewClient(corehttp.WithTransport(miguRunnerTransport(func(req *http.Request) (*http.Response, error) {
		body := `{"code":"000000","data":{}}`
		switch req.URL.Path {
		case "/orchestration/auth-rebuild/token/v1.0/querySpecToken":
			body = `{"success":true,"code":"0000","data":{"token":"fixture-ticket"}}`
		case "/user/h5/token-validate/v3.0":
			body = `{"code":"000000","data":{"uid":"fixture-user","pacmtoken":"fixture-music-token"}}`
		case "/act-jbzx/sign/status":
			if signed {
				body = `{"code":"000000","data":{"isSign":true}}`
			} else {
				body = `{"code":"000000","data":{"isSign":false}}`
			}
		case "/act-jbzx/new-user-receive/status":
			body = `{"code":"000000","data":{"signTask":true}}`
		case "/act-jbzx/sign":
			signed = true
		case "/act-jbzx/ai-bean/query-available":
			if signed {
				body = `{"code":"000000","data":{"availableNum":134}}`
			} else {
				body = `{"code":"000000","data":{"availableNum":94}}`
			}
		case "/act-task/join-activity":
		default:
			t.Fatalf("unexpected request: %s", req.URL.Path)
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(body)), Request: req}, nil
	})))
	runner := &TaskRunner{account: &models.Account{Phone: "13800000001", Auth: "Basic fixture-auth"}, httpClient: client}
	first := runner.runMiguTask("migu_signin")
	if first.Status != "success" || first.MiguNotesGained != 40 || first.CloudGained != 0 {
		t.Fatalf("notes were lost or counted as clouds: %+v", first)
	}
	if again := runner.runMiguTask("migu_signin"); again.MiguNotesGained != 0 || again.CloudGained != 0 {
		t.Fatalf("completed task repeated earnings: %+v", again)
	}
}

func TestMiguFailedExchangeIsNotRetriedForEveryTask(t *testing.T) {
	calls := 0
	client := corehttp.NewClient(corehttp.WithTransport(miguRunnerTransport(func(req *http.Request) (*http.Response, error) {
		calls++
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(`{"success":false,"code":"1010010015","message":"鉴权失效"}`)), Request: req}, nil
	})))
	runner := &TaskRunner{account: &models.Account{Phone: "13800000001", Auth: "Basic expired"}, httpClient: client}
	for _, def := range miguTaskDefinitions() {
		result := runner.runMiguTask(def.Code)
		if result.Status != "failed" || result.CloudGained != 0 {
			t.Fatalf("%+v", result)
		}
	}
	if calls != 1 {
		t.Fatalf("expired credentials repeatedly exchanged: %d calls", calls)
	}
	if result := (&TaskRunner{}).runMiguTask("migu_signin"); result.Status != "failed" {
		t.Fatal("missing account must fail cleanly")
	}
}

func TestMiguAssetsRefreshesWhileCompletedActionsAreDeduplicated(t *testing.T) {
	codes := []string{"migu_signin", "migu_assets", "migu_song"}
	remaining := remainingBatchTaskCodes(codes, codes)
	if len(remaining) != 1 || remaining[0] != "migu_assets" {
		t.Fatalf("remaining=%v", remaining)
	}
}
