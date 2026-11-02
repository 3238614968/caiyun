package tasks

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestExpansionBackupUploadsPhotoAndCompletesSnapshot(t *testing.T) {
	var paths []string
	client := corehttp.NewClient(corehttp.WithTransport(hiddenRewardRoundTrip(func(r *http.Request) (*http.Response, error) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("x-yun-op-type") != "2" || r.Header.Get("x-yun-app-channel") != "10000023" {
			t.Error("expansion used a normal/PC upload rather than album backup")
		}
		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		switch r.URL.Path {
		case "/hcy/file/create":
			if body["contentType"] != "image/jpeg" || !strings.HasSuffix(body["name"].(string), ".jpg") {
				t.Error("backup did not upload a photo")
			}
			return hiddenRewardReply(r, `{"success":true,"data":{"fileId":"photo","rapidUpload":true}}`), nil
		case "/hcy/backup/createSnapshot":
			return hiddenRewardReply(r, `{"success":true,"data":{"fileId":"snapshot","uploadId":"upload","rapidUpload":true}}`), nil
		case "/hcy/backup/completeSnapshot":
			if body["fileId"] != "snapshot" || body["uploadId"] != "upload" {
				t.Fatal("completed a different snapshot")
			}
			return hiddenRewardReply(r, `{"success":true,"code":"0000"}`), nil
		default:
			t.Fatalf("unexpected backup endpoint: %s", r.URL.Path)
			return nil, nil
		}
	})))
	task := NewTaskListTask(client, logger.NewLogger(logger.LevelError))
	if err := task.tryBackupForTaskExpansion(); err != nil {
		t.Fatal(err)
	}
	if len(paths) != 3 || paths[2] != "/hcy/backup/completeSnapshot" {
		t.Fatalf("incomplete backup flow: %v", paths)
	}
}
