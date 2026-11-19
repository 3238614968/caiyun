package dto

import (
	"encoding/json"
	"strings"
	"testing"

	"caiyun/internal/models"
)

func TestExecutionResponsesHideDiagnosticsWithoutChangingStoredResults(t *testing.T) {
	const raw = `您的云朵数不足 | http_status=200 | code=1001 | body={"code":1001} | 滑块识别offset=376 confidence=0.8511 资格 exchangeCode=0 | build=current commit=dirty built_at=2026-11-12`
	const want = "云朵不足，兑换失败"
	log := &models.TaskLog{ID: 7, TaskType: "exchange", Status: "failed", Message: raw, ExecutionTime: 123}
	task := &models.ExchangeTask{ID: 8, Status: "failed", LastResult: raw, RetryCount: 2, FailCount: 3}
	record := &models.ExchangeRecord{ID: 9, Status: "failed", Message: raw, ExecutionTimeMs: 123}
	logResponse := ToTaskLogResponse(log)
	taskResponse := ToExchangeTaskResponse(task)
	recordResponse := ToExchangeRecordResponse(record)
	if logResponse.Message != want || taskResponse.LastResult != want || recordResponse.Message != want {
		t.Fatalf("raw diagnostics exposed by response: log=%q task=%q record=%q", logResponse.Message, taskResponse.LastResult, recordResponse.Message)
	}
	if logResponse.Status != "failed" || taskResponse.Status != "failed" || recordResponse.Status != "failed" || taskResponse.RetryCount != 2 || taskResponse.FailCount != 3 || recordResponse.ExecutionTimeMs != 123 {
		t.Fatal("presentation changed status, counters or timing")
	}
	if log.Message != raw || task.LastResult != raw || record.Message != raw {
		t.Fatal("diagnostics needed by server-side retry decisions were lost")
	}
	for name, response := range map[string]any{"log": ToTaskLogResponses([]*models.TaskLog{log}), "task": ToExchangeTaskResponses([]*models.ExchangeTask{task}), "record/export": ToExchangeRecordResponses([]*models.ExchangeRecord{record})} {
		encoded, err := json.Marshal(response)
		if err != nil {
			t.Fatal(err)
		}
		for _, forbidden := range []string{"http_status", "code=", "body=", "offset=", "confidence=", "exchangeCode=", "build=", "commit=", "built_at="} {
			if strings.Contains(string(encoded), forbidden) {
				t.Fatalf("%s exposes %q: %s", name, forbidden, encoded)
			}
		}
	}
	if got := ToExchangeTaskResponse(&models.ExchangeTask{Status: "pending"}); got.LastResult != "" || got.Status != "pending" {
		t.Fatalf("unexecuted task acquired a result: %+v", got)
	}
}
