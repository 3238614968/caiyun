package tasks

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
)

// Opt-in diagnostics for an existing account. No credentials, raw URLs or
// headers are persisted; the execution path is the production MiguTask.Run.
func TestMiguExistingAccountLive(t *testing.T) {
	authFile := os.Getenv("CAIYUN_MIGU_LIVE_AUTH_FILE")
	if authFile == "" {
		t.Skip("explicit existing-account live test required")
	}
	mode := os.Getenv("CAIYUN_MIGU_LIVE_MODE")
	if mode == "" {
		mode = "snapshot"
	}
	if mode != "snapshot" && mode != "execute" {
		t.Fatal("live mode must be snapshot or execute")
	}
	reportPath := os.Getenv("CAIYUN_MIGU_LIVE_REPORT")
	if !filepath.IsAbs(reportPath) {
		t.Fatal("absolute sanitized report path required")
	}
	proxy, err := url.Parse(os.Getenv("CAIYUN_MIGU_LIVE_PROXY"))
	if err != nil || proxy.Host == "" || (proxy.Scheme != "http" && proxy.Scheme != "https") {
		t.Fatal("explicit working proxy required; no direct fallback")
	}
	raw, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatal("existing credential file unavailable")
	}
	credential := strings.TrimSpace(string(raw))
	if strings.HasPrefix(credential, "{") {
		var document struct {
			Authorization string `json:"authorization"`
			Auth          string `json:"auth"`
		}
		if json.Unmarshal(raw, &document) != nil {
			t.Fatal("existing JSON credential format invalid")
		}
		credential = document.Authorization
		if credential == "" {
			credential = document.Auth
		}
	}
	account, err := auth.ParseToken(credential)
	if err != nil || len(account.Phone) != 11 {
		t.Fatal("existing account credential format invalid")
	}
	redact := strings.NewReplacer(account.AuthFull, "[auth]", account.Auth, "[auth]", account.Token, "[token]", account.Phone, "[account]")
	report := map[string]interface{}{
		"account_suffix": account.Phone[7:], "mode": mode,
		"started_at": time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339),
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = http.ProxyURL(proxy)
	transport.DialContext = (&net.Dialer{Timeout: 15 * time.Second, KeepAlive: 30 * time.Second}).DialContext
	defer transport.CloseIdleConnections()
	trace := &miguLiveTransport{transport: transport, allowRefresh: os.Getenv("CAIYUN_MIGU_LIVE_REFRESH") == "1"}
	defer func() {
		report["requests"] = trace.observations
		report["finished_at"] = time.Now().In(time.FixedZone("Asia/Shanghai", 8*3600)).Format(time.RFC3339)
		out, err := json.MarshalIndent(report, "", "  ")
		if err != nil || os.WriteFile(reportPath, append(out, '\n'), 0600) != nil {
			t.Error("sanitized report write failed")
		}
	}()
	client := corehttp.NewClient(corehttp.WithTransport(trace))
	client.SetAuth(account.AuthFull)
	client.SetMarketAccount(account.Phone)
	store := NewMemoryStore()
	currentAuthorization := account.AuthFull
	task := NewMiguTask(client).SetAccountContext(account.Phone, currentAuthorization).SetStorage(store)
	exchangeErr := task.ensureSession()
	if errors.Is(exchangeErr, auth.ErrCredentialExpired) && trace.allowRefresh && !trace.failed {
		refreshed, refreshErr := auth.NewAuth(client).RefreshAuthorization(account.AuthFull, account.Phone, "")
		if refreshErr != nil {
			report["authorization_refresh_error"] = redact.Replace(refreshErr.Error())
		} else {
			report["authorization_refreshed"] = true
			currentAuthorization = refreshed.Auth
			client.SetAuth(refreshed.Auth)
			task = NewMiguTask(client).SetAccountContext(account.Phone, currentAuthorization).SetStorage(store)
			exchangeErr = task.ensureSession()
		}
	}
	if err := exchangeErr; err != nil {
		report["exchange_error"] = redact.Replace(err.Error())
		t.Fatal("existing-account exchange failed; see sanitized report")
	}
	report["exchange_succeeded"] = true
	snapshot := func() map[string]interface{} {
		result := map[string]interface{}{}
		if state, err := task.api.SignStatus(false); err == nil {
			result["sign"] = state
		} else {
			result["sign_error"] = redact.Replace(err.Error())
			t.Error("live sign readback failed; see sanitized report")
		}
		if state, err := task.api.SignStatus(true); err == nil {
			result["new_user"] = state
		} else {
			result["new_user_error"] = redact.Replace(err.Error())
			if !api.MiguErrorCode(err, "200013") {
				t.Error("live new-user readback failed; see sanitized report")
			}
		}
		if state, err := task.api.QueryTasks(); err == nil {
			result["tasks"] = state
		} else {
			result["tasks_error"] = redact.Replace(err.Error())
			t.Error("live task readback failed; see sanitized report")
		}
		if state, err := task.api.SongStatus(); err == nil {
			result["songs"] = state
		} else {
			result["songs_error"] = redact.Replace(err.Error())
			t.Error("live song readback failed; see sanitized report")
		}
		if notice, notes, err := task.api.Assets(); err == nil {
			result["reward_notice"], result["notes"] = notice, notes
		} else {
			result["assets_error"] = redact.Replace(err.Error())
			t.Error("live asset readback failed; see sanitized report")
		}
		if data, err := task.api.MusicBeanFlow(1, 30); err == nil {
			var value interface{}
			_ = json.Unmarshal(data, &value)
			result["coin_flow"] = miguLiveSafeData(value)
		} else {
			result["coin_flow_error"] = redact.Replace(err.Error())
		}
		return result
	}
	before := snapshot()
	report["before"] = before
	if mode == "snapshot" {
		t.Logf("existing account …%s: exchange and read-only snapshots collected", account.Phone[7:])
		return
	}
	var results []map[string]interface{}
	for _, code := range []string{"migu_login", "migu_signin", "migu_new_user", "migu_community", "migu_mobile", "migu_noon", "migu_night", "migu_song", "migu_assets"} {
		if trace.failed {
			t.Error("proxy/request failure stopped further account actions")
			break
		}
		started := time.Now()
		err := task.Run(code)
		status, message := "success", task.Message()
		if err != nil {
			status, message = "failed", redact.Replace(err.Error())
			t.Errorf("%s failed; see sanitized report", code)
		} else if task.Pending() {
			status = "pending"
		}
		results = append(results, map[string]interface{}{
			"task_type": code, "status": status, "message": message,
			"cloud_gained": 0, "migu_notes_gained": task.NotesGained(), "execution_time": time.Since(started).Milliseconds(),
		})
		t.Logf("%s: %s", code, status)
		time.Sleep(time.Second)
	}
	report["task_results"] = results
	if !trace.failed && os.Getenv("CAIYUN_MIGU_LIVE_WINDOW_REPORT_ONCE") == "1" {
		// Compare the reference client's unconditional daily report against the
		// service's real completion and reward state. One submission per ID;
		// do not alter the clock or treat an accepted report as completion.
		var probes []map[string]interface{}
		for _, id := range []string{"noon-check", "night-check"} {
			if trace.failed {
				break
			}
			state, err := task.findTask(func(item api.MiguTaskInfo) bool { return item.ID == id })
			if err != nil || state == nil {
				continue
			}
			done, stateErr := miguTaskDone(*state)
			if stateErr != nil || done {
				continue
			}
			notesBefore, err := task.api.NoteBalance()
			if err != nil {
				t.Error("window probe balance unavailable")
				break
			}
			submitErr := task.api.ReportTask(id)
			time.Sleep(time.Second)
			after, readErr := task.findTask(func(item api.MiguTaskInfo) bool { return item.ID == id })
			notesAfter, balanceErr := task.api.NoteBalance()
			row := map[string]interface{}{"id": id, "accepted": submitErr == nil, "notes_before": notesBefore}
			if submitErr != nil {
				row["submit_error"] = redact.Replace(submitErr.Error())
			}
			if readErr == nil && after != nil {
				row["completion_after"] = after.Complete
				row["completed"], _ = miguTaskDone(*after)
			}
			if balanceErr == nil {
				row["notes_after"], row["notes_delta"] = notesAfter, notesAfter-notesBefore
			}
			probes = append(probes, row)
		}
		report["window_reference_probes"] = probes
	}
	if !trace.failed {
		time.Sleep(2 * time.Second)
		after := snapshot()
		report["after"] = after
		delta := map[string]int64{}
		for _, currency := range []string{"notes"} {
			if start, ok := before[currency].(int64); ok {
				if finish, ok := after[currency].(int64); ok {
					delta[currency] = finish - start
				}
			}
		}
		report["reward_delta"] = delta
		cached := NewMiguTask(client).SetAccountContext(account.Phone, currentAuthorization).SetStorage(store)
		err := cached.Run("migu_assets")
		report["cached_session_succeeded"] = err == nil
		if err != nil {
			report["cached_session_error"] = redact.Replace(err.Error())
			t.Error("cached live session did not work; see sanitized report")
		}
	}
}

type miguLiveTransport struct {
	transport    http.RoundTripper
	mu           sync.Mutex
	failed       bool
	allowRefresh bool
	observations []map[string]interface{}
}

func (t *miguLiveTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.failed {
		return nil, fmt.Errorf("live request transport stopped after prior failure")
	}
	refreshAllowed := t.allowRefresh && req.URL.Hostname() == "user-njs.yun.139.com" && req.URL.Path == "/user/auth/refreshToken"
	if req.URL.Scheme != "https" || (req.URL.Hostname() != "orches.yun.139.com" && req.URL.Hostname() != "app.c.nf.migu.cn" && !refreshAllowed) {
		return nil, fmt.Errorf("live request outside selected protocol scope")
	}
	row := map[string]interface{}{"method": req.Method, "host": req.URL.Hostname(), "path": req.URL.Path}
	resp, err := t.transport.RoundTrip(req)
	if err != nil {
		t.failed = true
		row["transport_error"] = true
		t.observations = append(t.observations, row)
		return nil, fmt.Errorf("proxied live request failed; no direct fallback")
	}
	row["http_status"] = resp.StatusCode
	raw, readErr := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	_ = resp.Body.Close()
	if readErr != nil {
		t.failed = true
		return nil, fmt.Errorf("proxied live response read failed")
	}
	resp.Body = io.NopCloser(bytes.NewReader(raw))
	var envelope struct {
		Code interface{} `json:"code"`
		Data interface{} `json:"data"`
	}
	if json.Unmarshal(raw, &envelope) == nil {
		row["code"] = envelope.Code
		row["data_shape"] = miguLiveShape(envelope.Data)
		if strings.HasPrefix(req.URL.Path, "/act-") {
			row["data"] = miguLiveSafeData(envelope.Data)
		}
	} else {
		row["json_response"] = false
	}
	t.observations = append(t.observations, row)
	return resp, nil
}

func miguLiveShape(value interface{}) interface{} {
	switch data := value.(type) {
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, value := range data {
			out[key] = miguLiveShape(value)
		}
		return out
	case []interface{}:
		if len(data) > 0 {
			return []interface{}{miguLiveShape(data[0])}
		}
		return []interface{}{}
	default:
		return fmt.Sprintf("%T", data)
	}
}

func miguLiveSafeData(value interface{}) interface{} {
	allowed := " signTask isSign signNum currentDay days day sign type scoreNum receiveStatus id taskName taskDesc needCompleteTime completeTime rceiveTime songTime allCompleted tasks currentTaskId name num needTime showTime status aiBeanNum availableNum list rows records total totalPage totalCount pageNumber pageSize amount description createTime updateTime flowType changeNum musicBeanNum businessType title time date "
	switch data := value.(type) {
	case map[string]interface{}:
		out := map[string]interface{}{}
		for key, value := range data {
			if strings.Contains(allowed, " "+key+" ") {
				out[key] = miguLiveSafeData(value)
			}
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(data))
		for _, value := range data {
			out = append(out, miguLiveSafeData(value))
		}
		return out
	case string:
		if len(data) > 200 {
			return "[省略长字段]"
		}
		return data
	default:
		return value
	}
}
