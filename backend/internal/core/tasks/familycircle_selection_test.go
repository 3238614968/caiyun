package tasks

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"testing"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

func TestFamilyCircleDoesNotTreatTheOnlyFriendCircleAsFamily(t *testing.T) {
	job := taskStepTestJob(t, func(req *http.Request) string {
		if req.URL.Path != "/hcy/group/manage/circle/myGroupList" {
			t.Fatal("must not submit family actions for a friend circle")
		}
		return `{"code":"0000","data":{"pageInfo":{"list":[{"id":"friend","type":1}],"isLastPage":true}}}`
	})
	job.client.SetMarketAccount("13800000000")
	task := NewFamilyCircleTask(job.client, job.logger)
	if err := task.Run(); err != nil || !task.Pending() {
		t.Fatalf("pending=%v err=%v", task.Pending(), err)
	}
}

func TestFamilyCircleExistingGroupStillRequiresMarketingCompletionRecord(t *testing.T) {
	for _, status := range []int{http.StatusOK, http.StatusNotFound} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			queries := 0
			client := corehttp.NewClient(corehttp.WithTransport(coc3RoundTrip(func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/hcy/group/manage/circle/myGroupList" {
					return coc3Reply(req, `{"code":"0000","data":{"pageInfo":{"list":[{"id":"fixture-family","type":0,"memberNum":1}],"isLastPage":true}}}`), nil
				}
				if !strings.Contains(req.URL.Path, "/familyCircle/query") {
					t.Fatal("unexpected family action")
				}
				queries++
				response := coc3Reply(req, `{"code":0,"result":1}`)
				response.StatusCode = status
				return response, nil
			})))
			client.SetMarketAccount("13800000000")
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture-user"}}`))
			client.SetJWTToken("h." + claims + ".s")
			task := NewFamilyCircleTask(client, logger.NewLogger(logger.LevelError))
			err := task.Run()
			if !task.Pending() || queries != 2 || errors.Is(err, api.ErrFamilyCircleEndpointUnavailable) != (status == http.StatusNotFound) {
				t.Fatalf("pending=%v queries=%d err=%v", task.Pending(), queries, err)
			}
			if status == http.StatusNotFound && !strings.Contains(task.Message(), "HTTP 404") {
				t.Fatal("lost the unavailable-route reason despite an existing family circle")
			}
			if strings.Contains(task.Message(), "已完成") || strings.Contains(task.Message(), "没有群组") {
				t.Fatal("state query was converted to a completion or missing-group claim")
			}
		})
	}
}
