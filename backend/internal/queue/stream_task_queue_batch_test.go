package queue

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/models"
)

type streamMoveCall struct {
	source, group, id, destination, member string
	maxLen                                 int64
	score                                  float64
	values                                 map[string]interface{}
}

type fakeStreamQueueStore struct {
	groupCreateCalls      int
	xaddCalls             int
	xaddMaxLen            int64
	xaddBatchCalls        int
	xaddBatchMaxLen       int64
	batchValues           []map[string]interface{}
	xaddDedupeCalls       int
	xaddDedupeMaxLen      int64
	xaddBatchDedupeCalls  int
	xaddBatchDedupeMaxLen int64
	dedupeItems           []cache.StreamEnqueueItem
	ackCalls              int
	ackStream, ackGroup   string
	ackIDs                []string
	ackResult             int64
	ackResultSet          bool
	moveStreamCalls       []streamMoveCall
	moveStreamMoved       bool
	moveZSetCalls         []streamMoveCall
	moveZSetMoved         bool
	promoteCalls          []streamMoveCall
	promoteMoved          bool
	readMessages          []cache.StreamMessage
	autoClaimMessages     []cache.StreamMessage
	renewCalls            int
	renewStream           string
	renewGroup            string
	renewConsumer         string
	renewID               string
	renewed               bool
	rangeMessages         []cache.StreamMessage
	moveEntryMoved        bool
	deletedEntries        int64
	dueItems              []string
}

func (f *fakeStreamQueueStore) XGroupCreateMkStream(string, string, string) error {
	f.groupCreateCalls++
	return nil
}

func (f *fakeStreamQueueStore) XAdd(_ string, maxLen int64, _ map[string]interface{}) (string, error) {
	f.xaddCalls++
	f.xaddMaxLen = maxLen
	return fmt.Sprintf("%d-0", f.xaddCalls), nil
}

func (f *fakeStreamQueueStore) XAddBatch(_ string, maxLen int64, values []map[string]interface{}) ([]string, error) {
	f.xaddBatchCalls++
	f.xaddBatchMaxLen = maxLen
	f.batchValues = append([]map[string]interface{}(nil), values...)
	ids := make([]string, len(values))
	for i := range ids {
		ids[i] = fmt.Sprintf("%d-0", i+1)
	}
	return ids, nil
}

func (f *fakeStreamQueueStore) XAddWithDedupe(_ string, maxLen int64, item cache.StreamEnqueueItem, _ time.Duration) (string, bool, error) {
	f.xaddDedupeCalls++
	f.xaddDedupeMaxLen = maxLen
	f.dedupeItems = append(f.dedupeItems, item)
	return "1-0", true, nil
}

func (f *fakeStreamQueueStore) XAddBatchWithDedupe(_ string, maxLen int64, items []cache.StreamEnqueueItem, _ time.Duration) ([]string, error) {
	f.xaddBatchDedupeCalls++
	f.xaddBatchDedupeMaxLen = maxLen
	f.dedupeItems = append([]cache.StreamEnqueueItem(nil), items...)
	ids := make([]string, len(items))
	for i := range ids {
		ids[i] = fmt.Sprintf("%d-0", i+1)
	}
	return ids, nil
}

func (f *fakeStreamQueueStore) XReadGroup(string, string, string, string, int64, time.Duration) ([]cache.StreamMessage, error) {
	return append([]cache.StreamMessage(nil), f.readMessages...), nil
}

func (f *fakeStreamQueueStore) XAckAndDelete(stream, group string, ids ...string) (int64, error) {
	f.ackCalls++
	f.ackStream, f.ackGroup = stream, group
	f.ackIDs = append([]string(nil), ids...)
	if f.ackResultSet {
		return f.ackResult, nil
	}
	return int64(len(ids)), nil
}

func (f *fakeStreamQueueStore) XRange(string, string, string, int64) ([]cache.StreamMessage, error) {
	return append([]cache.StreamMessage(nil), f.rangeMessages...), nil
}

func (f *fakeStreamQueueStore) XMoveEntryToStream(source, id, destination string, maxLen int64, values map[string]interface{}) (string, bool, error) {
	f.moveStreamCalls = append(f.moveStreamCalls, streamMoveCall{source: source, id: id, destination: destination, maxLen: maxLen, values: values})
	if !f.moveEntryMoved {
		return "", false, nil
	}
	return "dead-replay-1", true, nil
}

func (f *fakeStreamQueueStore) XDel(string, ...string) (int64, error) { return f.deletedEntries, nil }

func (f *fakeStreamQueueStore) XMoveToStream(source, group, id, destination string, maxLen int64, values map[string]interface{}) (string, bool, error) {
	f.moveStreamCalls = append(f.moveStreamCalls, streamMoveCall{
		source: source, group: group, id: id, destination: destination, maxLen: maxLen, values: values,
	})
	if !f.moveStreamMoved {
		return "", false, nil
	}
	return "2-0", true, nil
}

func (f *fakeStreamQueueStore) XMoveToZSet(source, group, id, destination, member string, score float64) (bool, error) {
	f.moveZSetCalls = append(f.moveZSetCalls, streamMoveCall{
		source: source, group: group, id: id, destination: destination, member: member, score: score,
	})
	return f.moveZSetMoved, nil
}

func (f *fakeStreamQueueStore) XPromoteZSetToStream(source, destination, member string, maxScore float64, maxLen int64, values map[string]interface{}) (string, bool, error) {
	f.promoteCalls = append(f.promoteCalls, streamMoveCall{
		source: source, destination: destination, member: member, score: maxScore, maxLen: maxLen, values: values,
	})
	if !f.promoteMoved {
		return "", false, nil
	}
	return "3-0", true, nil
}

func (f *fakeStreamQueueStore) XAutoClaim(string, string, string, time.Duration, string, int64) ([]cache.StreamMessage, string, error) {
	return append([]cache.StreamMessage(nil), f.autoClaimMessages...), "0-0", nil
}

func (f *fakeStreamQueueStore) XRenewPending(stream, group, consumer, id string) (bool, error) {
	f.renewCalls++
	f.renewStream, f.renewGroup, f.renewConsumer, f.renewID = stream, group, consumer, id
	return f.renewed, nil
}

func (f *fakeStreamQueueStore) XPendingCount(string, string) (int64, error) { return 0, nil }
func (f *fakeStreamQueueStore) XLen(string) int64                           { return 0 }

func (f *fakeStreamQueueStore) ZRangeByScore(string, string, string, int64) ([]string, error) {
	return append([]string(nil), f.dueItems...), nil
}

func (f *fakeStreamQueueStore) ZCard(string) int64  { return 0 }
func (f *fakeStreamQueueStore) Del(...string) error { return nil }

func TestStreamTaskQueueEnqueueBatchUsesSingleBatchWrite(t *testing.T) {
	t.Setenv("TASK_QUEUE_DEDUPE_ENABLED", "false")
	store := &fakeStreamQueueStore{}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{})
	accounts := []*models.Account{{ID: 101, UserID: 201}, {ID: 102, UserID: 202}}

	if err := q.EnqueueBatch(accounts, "signin"); err != nil {
		t.Fatalf("EnqueueBatch() error = %v", err)
	}
	if store.groupCreateCalls != 1 || store.xaddBatchCalls != 1 || store.xaddCalls != 0 || store.xaddBatchDedupeCalls != 0 {
		t.Fatalf("writes group=%d batch=%d single=%d dedupe=%d", store.groupCreateCalls, store.xaddBatchCalls, store.xaddCalls, store.xaddBatchDedupeCalls)
	}
	if len(store.batchValues) != len(accounts) {
		t.Fatalf("batchValues len = %d, want %d", len(store.batchValues), len(accounts))
	}
	if store.xaddBatchMaxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("live stream batch maxlen = %d, want disabled", store.xaddBatchMaxLen)
	}

	store = &fakeStreamQueueStore{}
	q = newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{MaxLenApprox: 1})
	if err := q.Enqueue(103, 203, "signin"); err != nil {
		t.Fatal(err)
	}
	if store.xaddCalls != 1 || store.xaddMaxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("plain live stream writes=%d maxlen=%d, want one untrimmed write", store.xaddCalls, store.xaddMaxLen)
	}
}

func TestStreamTaskQueueEnqueueUsesAtomicDedupeWrites(t *testing.T) {
	t.Setenv("TASK_QUEUE_DEDUPE_ENABLED", "true")
	t.Setenv("TASK_QUEUE_DEDUPE_TTL", "2m")
	store := &fakeStreamQueueStore{}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{})
	accounts := []*models.Account{{ID: 101, UserID: 201}, {ID: 102, UserID: 202}}

	if err := q.EnqueueBatch(accounts, "signin"); err != nil {
		t.Fatal(err)
	}
	if store.xaddBatchDedupeCalls != 1 || store.xaddBatchCalls != 0 || len(store.dedupeItems) != 2 {
		t.Fatalf("dedupe batch=%d plain=%d items=%d", store.xaddBatchDedupeCalls, store.xaddBatchCalls, len(store.dedupeItems))
	}
	if store.xaddBatchDedupeMaxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("deduplicated live stream batch maxlen = %d, want disabled", store.xaddBatchDedupeMaxLen)
	}
	for i, item := range store.dedupeItems {
		payload, ok := item.Values[streamPayloadField].(string)
		if !ok || !strings.HasPrefix(item.DedupeKey, taskDedupeKeyPrefix) {
			t.Fatalf("invalid dedupe item %d: %+v", i, item)
		}
		var message TaskMessage
		if err := json.Unmarshal([]byte(payload), &message); err != nil {
			t.Fatal(err)
		}
		if message.IdempotencyKey != item.DedupeKey {
			t.Fatalf("message key=%q, claim key=%q", message.IdempotencyKey, item.DedupeKey)
		}
	}

	store = &fakeStreamQueueStore{}
	q = newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{})
	if err := q.Enqueue(103, 203, "signin"); err != nil {
		t.Fatal(err)
	}
	if store.xaddDedupeCalls != 1 || store.xaddCalls != 0 {
		t.Fatalf("single dedupe=%d plain=%d", store.xaddDedupeCalls, store.xaddCalls)
	}
	if store.xaddDedupeMaxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("deduplicated live stream maxlen = %d, want disabled", store.xaddDedupeMaxLen)
	}
}

func TestStreamTaskQueueEnqueueBatchValidatesBeforeWriting(t *testing.T) {
	store := &fakeStreamQueueStore{}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{})
	err := q.EnqueueBatch([]*models.Account{{ID: 101, UserID: 201}, nil}, "signin")
	if err == nil {
		t.Fatal("expected validation error")
	}
	if store.groupCreateCalls != 0 || store.xaddBatchCalls != 0 || store.xaddBatchDedupeCalls != 0 {
		t.Fatalf("unexpected write before validation")
	}
}

func TestStreamTaskQueueAckUsesAtomicAckAndDelete(t *testing.T) {
	store := &fakeStreamQueueStore{}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", ConsumerGroup: "group"})
	if err := q.Ack(&TaskMessage{StreamID: "1-0"}); err != nil {
		t.Fatal(err)
	}
	if store.ackCalls != 1 || store.ackStream != "stream" || store.ackGroup != "group" || len(store.ackIDs) != 1 || store.ackIDs[0] != "1-0" {
		t.Fatalf("atomic ack not used correctly")
	}
}

func TestStreamTaskQueueAckReportsMissingPendingConflict(t *testing.T) {
	store := &fakeStreamQueueStore{ackResultSet: true, ackResult: 0}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", ConsumerGroup: "group"})
	err := q.Ack(&TaskMessage{StreamID: "already-acked-0"})
	if err == nil || !strings.Contains(err.Error(), "冲突") || !strings.Contains(err.Error(), "acked=0") {
		t.Fatalf("Ack() error = %v, want explicit zero-ack conflict", err)
	}
}

func TestStreamTaskQueueRenewVisibilityUsesCurrentConsumerOnly(t *testing.T) {
	store := &fakeStreamQueueStore{renewed: true}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{
		StreamKey: "stream", ConsumerGroup: "group", ConsumerName: "worker-a",
	})
	renewed, err := q.RenewVisibility(&TaskMessage{StreamID: "1-0"})
	if err != nil || !renewed {
		t.Fatalf("RenewVisibility() = (%t, %v), want true/nil", renewed, err)
	}
	if store.renewCalls != 1 || store.renewStream != "stream" || store.renewGroup != "group" || store.renewConsumer != "worker-a" || store.renewID != "1-0" {
		t.Fatalf("renew call=%+v", store)
	}

	store.renewed = false
	renewed, err = q.RenewVisibility(&TaskMessage{StreamID: "already-recovered"})
	if err != nil || renewed {
		t.Fatalf("missing current pending delivery = (%t, %v), want false/nil", renewed, err)
	}
}

func TestStreamTaskQueueExplicitMovesAreAtomic(t *testing.T) {
	message := &TaskMessage{StreamID: "1-0", AccountID: 10, UserID: 20, TaskType: "signin"}

	immediate := &fakeStreamQueueStore{moveStreamMoved: true}
	q := newStreamTaskQueueWithStore(immediate, StreamTaskQueueOptions{StreamKey: "stream", ConsumerGroup: "group"})
	if err := q.RequeueDelayed(message, 0); err != nil {
		t.Fatal(err)
	}
	if len(immediate.moveStreamCalls) != 1 || immediate.moveStreamCalls[0].destination != "stream" || immediate.moveStreamCalls[0].maxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("immediate requeue calls=%+v", immediate.moveStreamCalls)
	}

	delayed := &fakeStreamQueueStore{moveZSetMoved: true}
	q = newStreamTaskQueueWithStore(delayed, StreamTaskQueueOptions{StreamKey: "stream", DelayedKey: "delayed", ConsumerGroup: "group"})
	if err := q.RequeueDelayed(message, time.Second); err != nil {
		t.Fatal(err)
	}
	if len(delayed.moveZSetCalls) != 1 || delayed.moveZSetCalls[0].destination != "delayed" || delayed.moveZSetCalls[0].member == "" {
		t.Fatalf("delayed requeue calls=%+v", delayed.moveZSetCalls)
	}

	dead := &fakeStreamQueueStore{moveStreamMoved: true}
	q = newStreamTaskQueueWithStore(dead, StreamTaskQueueOptions{StreamKey: "stream", DeadLetterKey: "dead", ConsumerGroup: "group", MaxLenApprox: 321})
	if err := q.DeadLetter(message, "exhausted"); err != nil {
		t.Fatal(err)
	}
	if len(dead.moveStreamCalls) != 1 || dead.moveStreamCalls[0].destination != "dead" || dead.moveStreamCalls[0].maxLen != 321 || dead.moveStreamCalls[0].values[streamDeadReasonField] != "exhausted" {
		t.Fatalf("dead letter calls=%+v", dead.moveStreamCalls)
	}
}

func TestStreamTaskQueueExplicitMoveReportsMissingPendingMessage(t *testing.T) {
	tests := []struct {
		name string
		run  func(*StreamTaskQueue, *TaskMessage) error
	}{
		{"immediate", func(q *StreamTaskQueue, m *TaskMessage) error { return q.RequeueDelayed(m, 0) }},
		{"delayed", func(q *StreamTaskQueue, m *TaskMessage) error { return q.RequeueDelayed(m, time.Second) }},
		{"dead_letter", func(q *StreamTaskQueue, m *TaskMessage) error { return q.DeadLetter(m, "failed") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q := newStreamTaskQueueWithStore(&fakeStreamQueueStore{}, StreamTaskQueueOptions{})
			err := tt.run(q, &TaskMessage{StreamID: "missing-0", AccountID: 10})
			if err == nil || !strings.Contains(err.Error(), "不存在") {
				t.Fatalf("error=%v, want missing pending message error", err)
			}
		})
	}
}

func TestStreamTaskQueueRecoveryAndPromotionUseAtomicMoves(t *testing.T) {
	payload, err := encodeTaskMessage(&TaskMessage{AccountID: 10, UserID: 20, TaskType: "signin"})
	if err != nil {
		t.Fatal(err)
	}
	recovery := &fakeStreamQueueStore{
		moveStreamMoved:   true,
		autoClaimMessages: []cache.StreamMessage{{ID: "1-0", Values: map[string]interface{}{streamPayloadField: payload}}},
	}
	q := newStreamTaskQueueWithStore(recovery, StreamTaskQueueOptions{})
	recovered, err := q.RecoverStaleProcessing(time.Second)
	if err != nil || recovered != 1 || len(recovery.moveStreamCalls) != 1 {
		t.Fatalf("recovered=%d calls=%d err=%v", recovered, len(recovery.moveStreamCalls), err)
	}
	if recovery.moveStreamCalls[0].maxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("recovered live stream maxlen = %d, want disabled", recovery.moveStreamCalls[0].maxLen)
	}

	promotion := &fakeStreamQueueStore{dueItems: []string{payload}, promoteMoved: true}
	q = newStreamTaskQueueWithStore(promotion, StreamTaskQueueOptions{})
	promoted, err := q.PromoteDueDelayed(10)
	if err != nil || promoted != 1 || len(promotion.promoteCalls) != 1 {
		t.Fatalf("promoted=%d calls=%d err=%v", promoted, len(promotion.promoteCalls), err)
	}
	if promotion.promoteCalls[0].maxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("promoted live stream maxlen = %d, want disabled", promotion.promoteCalls[0].maxLen)
	}

	raced := &fakeStreamQueueStore{dueItems: []string{payload}}
	q = newStreamTaskQueueWithStore(raced, StreamTaskQueueOptions{})
	promoted, err = q.PromoteDueDelayed(10)
	if err != nil || promoted != 0 {
		t.Fatalf("raced promotion promoted=%d err=%v, want 0/nil", promoted, err)
	}
}

func TestStreamTaskQueueDequeueMovesMalformedMessageToDeadLetter(t *testing.T) {
	store := &fakeStreamQueueStore{
		moveStreamMoved: true,
		readMessages: []cache.StreamMessage{{
			ID:     "bad-1",
			Values: map[string]interface{}{streamPayloadField: "{not-json"},
		}},
	}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", DeadLetterKey: "dead", ConsumerGroup: "group"})
	message, err := q.Dequeue(time.Millisecond)
	if err == nil || message != nil || !strings.Contains(err.Error(), "死信队列") {
		t.Fatalf("Dequeue() message=%+v error=%v", message, err)
	}
	if len(store.moveStreamCalls) != 1 {
		t.Fatalf("move calls = %d, want 1", len(store.moveStreamCalls))
	}
	call := store.moveStreamCalls[0]
	if call.id != "bad-1" || call.destination != "dead" || call.values[streamDeadOriginalDataField] != "{not-json" {
		t.Fatalf("dead-letter call = %+v", call)
	}
	sum := sha256.Sum256([]byte("{not-json"))
	if call.values[streamDeadOriginalLengthField] != len("{not-json") || call.values[streamDeadOriginalHashField] != fmt.Sprintf("%x", sum[:]) || call.values[streamDeadOriginalTruncatedField] != false {
		t.Fatalf("dead-letter forensic metadata = %+v", call.values)
	}
}

func TestStreamTaskQueueMalformedDeadLetterPayloadIsBoundedAndHasForensics(t *testing.T) {
	raw := strings.Repeat("x", maxMalformedDeadLetterPayloadBytes+123)
	store := &fakeStreamQueueStore{
		moveStreamMoved: true,
		readMessages: []cache.StreamMessage{{
			ID:     "bad-large-1",
			Values: map[string]interface{}{streamPayloadField: raw},
		}},
	}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", DeadLetterKey: "dead", ConsumerGroup: "group", MaxLenApprox: 77})
	if message, err := q.Dequeue(time.Millisecond); err == nil || message != nil {
		t.Fatalf("Dequeue() message=%+v error=%v, want malformed error", message, err)
	}
	if len(store.moveStreamCalls) != 1 {
		t.Fatalf("move calls = %d, want 1", len(store.moveStreamCalls))
	}
	call := store.moveStreamCalls[0]
	captured, ok := call.values[streamDeadOriginalDataField].(string)
	if !ok || len([]byte(captured)) != maxMalformedDeadLetterPayloadBytes {
		t.Fatalf("captured payload bytes = %d, want %d", len([]byte(captured)), maxMalformedDeadLetterPayloadBytes)
	}
	sum := sha256.Sum256([]byte(raw))
	if call.maxLen != 77 || call.values[streamDeadOriginalLengthField] != len([]byte(raw)) || call.values[streamDeadOriginalHashField] != fmt.Sprintf("%x", sum[:]) || call.values[streamDeadOriginalTruncatedField] != true {
		t.Fatalf("bounded dead-letter metadata = %+v; call=%+v", call.values, call)
	}
}

func TestStreamTaskQueueRecoveryMovesMalformedClaimToDeadLetter(t *testing.T) {
	store := &fakeStreamQueueStore{
		moveStreamMoved: true,
		autoClaimMessages: []cache.StreamMessage{{
			ID:     "bad-2",
			Values: map[string]interface{}{"unexpected": "value"},
		}},
	}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", DeadLetterKey: "dead", ConsumerGroup: "group"})
	recovered, err := q.RecoverStaleProcessing(time.Millisecond)
	if err != nil || recovered != 0 {
		t.Fatalf("RecoverStaleProcessing() recovered=%d error=%v", recovered, err)
	}
	if len(store.moveStreamCalls) != 1 || store.moveStreamCalls[0].destination != "dead" {
		t.Fatalf("move calls = %+v", store.moveStreamCalls)
	}
}

func TestStreamTaskQueueDeadLetterReplayUsesAtomicMoveAndGuardsOperations(t *testing.T) {
	payload, err := encodeTaskMessage(&TaskMessage{AccountID: 10, UserID: 20, TaskType: "signin", RetryCount: 2})
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeStreamQueueStore{
		moveEntryMoved: true,
		rangeMessages: []cache.StreamMessage{{
			ID: "dead-1",
			Values: map[string]interface{}{
				streamDeadReasonField:       "upstream unavailable",
				streamDeadFailedAtField:     "1700000000",
				streamDeadOriginalDataField: payload,
			},
		}},
	}
	q := newStreamTaskQueueWithStore(store, StreamTaskQueueOptions{StreamKey: "stream", DeadLetterKey: "dead"})
	items, err := q.ListDeadLetters(10)
	if err != nil || len(items) != 1 || items[0].Task == nil || items[0].ID != "dead-1" {
		t.Fatalf("ListDeadLetters() items=%+v err=%v", items, err)
	}
	replayed, err := q.ReplayDeadLetter("dead-1")
	if err != nil || replayed.RetryCount != 0 || len(store.moveStreamCalls) != 1 {
		t.Fatalf("ReplayDeadLetter() message=%+v calls=%d err=%v", replayed, len(store.moveStreamCalls), err)
	}
	if call := store.moveStreamCalls[0]; call.source != "dead" || call.destination != "stream" || call.maxLen != mainTaskStreamMaxLenApprox {
		t.Fatalf("atomic replay call = %+v", call)
	}

	operationPayload, err := encodeTaskMessage(&TaskMessage{OperationID: "operation-1", UserID: 20})
	if err != nil {
		t.Fatal(err)
	}
	store.rangeMessages = []cache.StreamMessage{{ID: "dead-operation", Values: map[string]interface{}{streamDeadOriginalDataField: operationPayload}}}
	if _, err := q.ReplayDeadLetter("dead-operation"); !errors.Is(err, ErrOperationDeadLetterReplay) {
		t.Fatalf("operation replay error = %v, want ErrOperationDeadLetterReplay", err)
	}
}
