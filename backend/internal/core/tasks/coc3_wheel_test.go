package tasks

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCoc3LotterySubmitsWithGoGeneratorWithoutExternalExecutables(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "")
	t.Setenv("CAIYUN_COC3_WASM_DIR", "")
	counts := map[int]int{}
	submits, devices := 0, 0
	task := newCoc3TestTask(t, func(req *http.Request) (*http.Response, error) {
		var payload map[string]interface{}
		_ = json.NewDecoder(req.Body).Decode(&payload)
		body := ""
		switch {
		case strings.HasSuffix(req.URL.Path, "/wasm/gettoken"):
			if payload["benefit_phone"] != "fixture-session" || req.Header.Get("coc-aurora") == "" {
				t.Fatal("official device preflight missing current session")
			}
			devices++
			body = `{"code":"0","success":true,"data":"fixture-device-token"}`
		case strings.HasSuffix(req.URL.Path, "/residueLotteryCount"):
			id := int(payload["activityId"].(float64))
			n := 1
			if counts[id] > 0 {
				n = 0
			}
			counts[id]++
			body = fmt.Sprintf(`{"code":200,"data":%d}`, n)
		case strings.HasSuffix(req.URL.Path, "/currentLottery"):
			body = `{"code":200,"data":{"skuId":123}}`
		case strings.HasSuffix(req.URL.Path, "/mixedOrder"):
			var pair []string
			header := req.Header.Get("wasmParams")
			if err := json.Unmarshal([]byte(header), &pair); err != nil || len(pair) != 2 || pair[0] == "" || pair[1] == "" || len(header) > 32<<10 {
				t.Fatal("request did not receive the Go-generated header")
			}
			submits++
			body = `{"code":0,"data":{"orderInfo":{"prizeName":"谢谢参与"}}}`
		default:
			t.Fatalf("unexpected request %s", req.URL)
			return nil, nil
		}
		return coc3Reply(req, body), nil
	})
	task.tokenValue = "fixture-session"
	message := strings.Join(task.runLottery(), "；")
	if task.Pending() || len(task.issues) != 0 || devices != len(coc3LotteryActs) || submits != len(coc3LotteryActs) || !strings.Contains(message, "共4次（已复查确认）") {
		t.Fatalf("pending=%v submits=%d issues=%v message=%s", task.Pending(), submits, task.issues, message)
	}
}

func TestCoc3CustomModuleNeedsNoJavaScriptAndRejectsCorruption(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	path := filepath.Join(dir, "wheel_bg.wasm")
	if err := os.WriteFile(path, coc3WheelWASM, 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := generateCoc3WasmParams(dir); err != nil || value == "" {
		t.Fatalf("wasm-only override failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("corrupt module"), 0600); err != nil {
		t.Fatal(err)
	}
	if value, err := generateCoc3WasmParams(dir); err == nil || value != "" {
		t.Fatal("corrupt module produced a token")
	}
}

func TestCoc3ExplicitStaticHeaderOverrideRemainsAvailable(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	t.Setenv("CAIYUN_COC3_WASM_DIR", filepath.Join(t.TempDir(), "missing"))
	t.Setenv("CAIYUN_COC3_WASM_PARAMS", "fixture-static-header")
	if value := (&Coc3Task{}).resolveWasmParams(); value != "fixture-static-header" {
		t.Fatal("explicit static override was lost")
	}
}
