package ws

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestMessageJSONCleansLiveAndHistoricalPayloadsWithoutMutatingDiagnostics(t *testing.T) {
	const raw = "您的云朵数不足；滑块识别offset=376 confidence=0.8511 method=内置多特征NCC；build=current commit=dirty"
	live := map[string]any{"task_type": "exchange", "task_id": uint64(9007199254740993), "success": false, "message": raw, "account_name": "小海", "product_name": "QQ音乐绿钻会员月卡"}
	historical, err := json.Marshal(live)
	if err != nil {
		t.Fatal(err)
	}
	for _, eventType := range []string{"exchange_complete", "exchange_result", "task_complete"} {
		for name, payload := range map[string]any{
			"live map":       live,
			"durable replay": json.RawMessage(historical),
			"typed payload": struct {
				TaskType string `json:"task_type"`
				Message  string `json:"message"`
			}{TaskType: "exchange", Message: raw},
		} {
			t.Run(eventType+"/"+name, func(t *testing.T) {
				message := Message{Type: eventType, Data: payload, UserID: 42, MessageID: "event-42", Sequence: 9007199254740995, CreatedAtMS: 1794440000000}
				encoded, err := json.Marshal(message)
				if err != nil {
					t.Fatal(err)
				}
				assertPublicExecutionEvent(t, encoded, eventType)
				writer := httptest.NewRecorder()
				if !writeSSE(writer, message) {
					t.Fatal("SSE delivery failed")
				}
				body := writer.Body.String()
				if !strings.HasPrefix(body, "id: 9007199254740995\ndata: ") {
					t.Fatalf("SSE envelope metadata changed: %s", body)
				}
				assertPublicExecutionEvent(t, []byte(strings.TrimSpace(strings.TrimPrefix(strings.SplitN(body, "\n", 2)[1], "data: "))), eventType)
			})
		}
	}
	if live["message"] != raw || !strings.Contains(string(historical), "offset=376") {
		t.Fatal("serializing an event changed its original diagnostic payload")
	}
}

func assertPublicExecutionEvent(t *testing.T, encoded []byte, eventType string) {
	t.Helper()
	var event struct {
		Type        string `json:"type"`
		UserID      uint   `json:"user_id"`
		MessageID   string `json:"message_id"`
		Sequence    uint64 `json:"sequence"`
		CreatedAtMS int64  `json:"created_at"`
		Data        struct {
			TaskID  uint64 `json:"task_id"`
			Success bool   `json:"success"`
			Message string `json:"message"`
		} `json:"data"`
	}
	if err := json.Unmarshal(encoded, &event); err != nil {
		t.Fatalf("event JSON shape changed: %v: %s", err, encoded)
	}
	if event.Type != eventType || event.UserID != 42 || event.MessageID != "event-42" || event.Sequence != 9007199254740995 || event.CreatedAtMS != 1794440000000 || event.Data.Success {
		t.Fatalf("event status or envelope metadata changed: %s", encoded)
	}
	if event.Data.TaskID != 0 && event.Data.TaskID != 9007199254740993 {
		t.Fatalf("event ID lost precision: %d", event.Data.TaskID)
	}
	if event.Data.Message != "云朵不足，兑换失败" {
		t.Fatalf("event still exposes diagnostic text: %s", encoded)
	}
}

func TestMessageJSONStillRejectsUnserializablePayload(t *testing.T) {
	if _, err := json.Marshal(Message{Type: "exchange_complete", Data: make(chan int)}); err == nil {
		t.Fatal("unserializable event payload accepted")
	}
}
