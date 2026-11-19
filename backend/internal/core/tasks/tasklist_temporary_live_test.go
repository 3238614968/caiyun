package tasks

import (
	"context"
	"encoding/json"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/api"
	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/core/logger"
)

// Explicitly opt in with a local credential file. Normal regression tests never
// contact the real service. Output contains task records, never credentials.
func TestTemporaryTasksLiveRecords(t *testing.T) {
	authPath := os.Getenv("CAIYUN_BEIYONG_LIVE_AUTH_FILE")
	if authPath == "" {
		t.Skip("live task verification requires an explicitly supplied credential file")
	}
	reportPath := os.Getenv("CAIYUN_BEIYONG_LIVE_REPORT")
	if reportPath == "" {
		t.Fatal("CAIYUN_BEIYONG_LIVE_REPORT is required")
	}
	data, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatal("cannot read the supplied credential file")
	}
	account, err := auth.ParseToken(string(data))
	if err != nil || account == nil || !strings.HasSuffix(account.Phone, os.Getenv("CAIYUN_BEIYONG_LIVE_ACCOUNT_SUFFIX")) || os.Getenv("CAIYUN_BEIYONG_LIVE_ACCOUNT_SUFFIX") == "" {
		t.Fatal("credential/account suffix validation failed")
	}
	client := corehttp.NewClient()
	client.SetAuth(account.Auth)
	client.SetMarketAccount(account.Phone)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	jwt, sso, err := auth.NewAuth(client).GetJWTTokenWithSSOTokenContext(ctx, account.Phone)
	if err != nil {
		t.Fatal("official JWT exchange failed; credentials and raw responses were not printed")
	}
	client.SetJWTToken(jwt)
	client.SetSSOToken(sso)
	job := NewTaskListTask(client, logger.NewLogger(logger.LevelError)).SetAccountContext(account.Phone, account.Token)
	type record struct {
		ID      int      `json:"id"`
		Name    string   `json:"name"`
		Group   string   `json:"group"`
		State   string   `json:"state"`
		Process int      `json:"process"`
		Step    int      `json:"currstep"`
		Enable  *int     `json:"enable,omitempty"`
		Types   []string `json:"stepTypeSet"`
	}
	row := func(task api.Task, group string) record {
		return record{task.ID, task.Name, group, task.State, task.Process, task.CurrStep, task.Enable, task.StepTypeSet}
	}
	type outcome struct {
		ID        int     `json:"id"`
		Before    *record `json:"before"`
		After     *record `json:"after"`
		Executed  bool    `json:"executed"`
		Verified  bool    `json:"server_finish_verified"`
		Result    string  `json:"result"`
		RunFailed bool    `json:"execution_failed"`
	}
	report := struct {
		ObservedAt  string    `json:"observed_at"`
		Suffix      string    `json:"account_suffix"`
		Execute     bool      `json:"execute_requested"`
		GroupCode   int       `json:"temporary_group_code"`
		GroupSent   bool      `json:"temporary_group_returned"`
		ResultKeys  []string  `json:"temporary_result_keys"`
		GroupTasks  []record  `json:"temporary_group_tasks"`
		MainIDs     []int     `json:"discovered_main_task_ids"`
		Outcomes    []outcome `json:"outcomes"`
		AllVerified bool      `json:"all_requested_tasks_live_verified"`
	}{ObservedAt: time.Now().UTC().Format(time.RFC3339), Suffix: os.Getenv("CAIYUN_BEIYONG_LIVE_ACCOUNT_SUFFIX"), Execute: os.Getenv("CAIYUN_BEIYONG_LIVE_EXECUTE") == "1"}
	defer func() {
		encoded, marshalErr := json.MarshalIndent(report, "", "  ")
		if marshalErr != nil || os.WriteFile(reportPath, append(encoded, '\n'), 0600) != nil {
			t.Error("cannot save sanitized task verification report")
		}
	}()
	group, err := job.api.GetTaskListV2("beiyong1")
	if err != nil || group == nil || group.Code != 0 {
		t.Fatal("temporary task group query failed")
	}
	report.GroupCode = group.Code
	for key := range group.Result {
		report.ResultKeys = append(report.ResultKeys, key)
	}
	sort.Strings(report.ResultKeys)
	_, report.GroupSent = group.Result["beiyong1"]
	for _, task := range group.Result["beiyong1"] {
		report.GroupTasks = append(report.GroupTasks, row(task, "beiyong1"))
	}
	items, err := job.fetchAllTaskItems()
	if err != nil {
		t.Fatal("task discovery failed")
	}
	for _, item := range items {
		if item.MarketName == "sign_in_3" {
			report.MainIDs = append(report.MainIDs, item.Task.ID)
		}
	}
	// Cleanup is scoped to file IDs created by this invocation, after collecting
	// the final server records. No listing/deleting files by filename prefix.
	defer job.cleanupTemporaryFiles()
	unavailable, verified := false, 0
	for _, id := range []int{587, 588} {
		result := outcome{ID: id, Result: "not_returned_by_server"}
		var selected *taskListItem
		for _, item := range items {
			if item.MarketName == "sign_in_3" && item.Task.ID == id {
				copy := item
				selected = &copy
				break
			}
		}
		if selected != nil {
			before := row(selected.Task, selected.GroupKey)
			result.Before = &before
			result.After = &before
			result.Result = "observed_only"
			switch {
			case isTaskDisabledByServer(selected.Task):
				result.Result = "disabled_by_server"
				unavailable = true
			case isTaskCompleted(selected.Task):
				result.Result, result.Verified = "already_finished", true
			case report.Execute:
				result.Executed = true
				handled, runErr := job.handleSpecialTask(*selected)
				result.RunFailed = !handled || runErr != nil
				result.Result = "not_finished"
				for poll := 0; poll < 4; poll++ {
					current, queryErr := job.queryTaskByMarket(selected.MarketName, selected.GroupKey, id)
					if queryErr != nil || current == nil {
						result.After = nil
						result.Result = "final_query_failed_or_record_disappeared"
						break
					}
					after := row(*current, selected.GroupKey)
					result.After = &after
					if isTaskCompleted(*current) {
						result.Result, result.Verified = "finished_after_execution", true
						break
					}
					if result.RunFailed || poll == 3 {
						break
					}
					time.Sleep(5 * time.Second)
				}
			}
		}
		if selected == nil {
			unavailable = true
		}
		if result.Verified {
			verified++
		}
		report.Outcomes = append(report.Outcomes, result)
		t.Logf("task %d: %s; executed=%t server_FINISH=%t", id, result.Result, result.Executed, result.Verified)
		if result.Executed && (!result.Verified || result.RunFailed) {
			t.Errorf("task %d did not pass live execution/FINISH verification", id)
		}
	}
	report.AllVerified = verified == 2
	if report.Execute && unavailable {
		t.Skip("requested activity tasks were not returned or were disabled; live completion remains unverified")
	}
}
