package cache

import "testing"

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
