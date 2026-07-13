package cache

import (
	"reflect"
	"testing"
	"time"
)

func TestParseXAutoClaimReplyRedis6And7(t *testing.T) {
	message := []interface{}{
		"1718172000000-0",
		[]interface{}{
			"payload",
			`{"account_id":1001,"user_id":2002,"task_type":"signin"}`,
		},
	}

	tests := []struct {
		name string
		raw  interface{}
	}{
		{
			name: "redis6_two_parts",
			raw: []interface{}{
				"0-0",
				[]interface{}{message},
			},
		},
		{
			name: "redis7_three_parts_with_deleted_ids",
			raw: []interface{}{
				"0-0",
				[]interface{}{message},
				[]interface{}{"1718171999999-0"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			messages, nextStart, err := parseXAutoClaimReply(tt.raw)
			if err != nil {
				t.Fatalf("parseXAutoClaimReply() error = %v", err)
			}
			if nextStart != "0-0" {
				t.Fatalf("nextStart = %q, want 0-0", nextStart)
			}
			if len(messages) != 1 {
				t.Fatalf("len(messages) = %d, want 1", len(messages))
			}
			if messages[0].ID != "1718172000000-0" {
				t.Fatalf("message ID = %q", messages[0].ID)
			}
			if messages[0].Values["payload"] == "" {
				t.Fatalf("payload missing: %+v", messages[0].Values)
			}
		})
	}
}

func TestOrderedStreamValueArgsIsDeterministicAndValidatesInput(t *testing.T) {
	got, err := orderedStreamValueArgs(map[string]interface{}{"z": "last", "a": "first"})
	if err != nil {
		t.Fatalf("orderedStreamValueArgs() error = %v", err)
	}
	want := []interface{}{"a", "first", "z", "last"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ordered args = %#v, want %#v", got, want)
	}

	invalid := []map[string]interface{}{
		nil,
		{"": "value"},
		{"field": nil},
	}
	for index, values := range invalid {
		if _, err := orderedStreamValueArgs(values); err == nil {
			t.Fatalf("invalid values %d unexpectedly succeeded", index)
		}
	}
}

func TestParseAtomicStreamResult(t *testing.T) {
	id, moved, err := parseAtomicStreamResult([]interface{}{int64(1), []byte("1718172000000-0")})
	if err != nil || !moved || id != "1718172000000-0" {
		t.Fatalf("moved reply id=%q moved=%v err=%v", id, moved, err)
	}

	id, moved, err = parseAtomicStreamResult([]interface{}{int64(0), ""})
	if err != nil || moved || id != "" {
		t.Fatalf("not-moved reply id=%q moved=%v err=%v", id, moved, err)
	}

	for _, raw := range []interface{}{
		"not-an-array",
		[]interface{}{int64(1)},
		[]interface{}{int64(1), ""},
	} {
		if _, _, err := parseAtomicStreamResult(raw); err == nil {
			t.Fatalf("malformed reply %#v unexpectedly succeeded", raw)
		}
	}
}

func TestXAddBatchWithDedupeRejectsSubMillisecondTTLBeforeRedis(t *testing.T) {
	client := &RedisCache{}
	_, err := client.XAddBatchWithDedupe(
		"stream",
		100,
		[]StreamEnqueueItem{{
			Values:      map[string]interface{}{"payload": "value"},
			DedupeKey:   "dedupe:key",
			DedupeValue: "claim",
		}},
		500*time.Microsecond,
	)
	if err == nil {
		t.Fatal("sub-millisecond TTL unexpectedly succeeded")
	}
}
