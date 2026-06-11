package queue

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

type fakeQueueStore struct {
	mu    sync.Mutex
	lists map[string][]string
	zsets map[string]map[string]float64
}

func newFakeQueueStore() *fakeQueueStore {
	return &fakeQueueStore{
		lists: make(map[string][]string),
		zsets: make(map[string]map[string]float64),
	}
}

func (f *fakeQueueStore) LPush(key string, values ...interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, value := range values {
		f.lists[key] = append([]string{asString(value)}, f.lists[key]...)
	}
	return nil
}

func (f *fakeQueueStore) BRPopLPush(source, destination string, _ time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := f.lists[source]
	if len(items) == 0 {
		return "", errors.New("empty")
	}
	value := items[len(items)-1]
	f.lists[source] = items[:len(items)-1]
	f.lists[destination] = append([]string{value}, f.lists[destination]...)
	return value, nil
}

func (f *fakeQueueStore) LRange(key string, start, stop int64) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	items := append([]string(nil), f.lists[key]...)
	if stop == -1 {
		stop = int64(len(items)) - 1
	}
	if start < 0 {
		start = 0
	}
	if stop >= int64(len(items)) {
		stop = int64(len(items)) - 1
	}
	if len(items) == 0 || start > stop {
		return []string{}, nil
	}
	return items[start : stop+1], nil
}

func (f *fakeQueueStore) LRem(key string, count int64, value interface{}) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	target := asString(value)
	items := f.lists[key]
	removed := int64(0)
	next := make([]string, 0, len(items))
	for _, item := range items {
		if item == target && (count == 0 || removed < count) {
			removed++
			continue
		}
		next = append(next, item)
	}
	f.lists[key] = next
	return removed, nil
}

func (f *fakeQueueStore) LLen(key string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.lists[key]))
}

func (f *fakeQueueStore) ZAdd(key string, score float64, member interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.zsets[key] == nil {
		f.zsets[key] = make(map[string]float64)
	}
	f.zsets[key][asString(member)] = score
	return nil
}

func (f *fakeQueueStore) ZRangeByScore(key string, min, max string, count int64) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	minScore, err := parseRedisScore(min, math.Inf(-1))
	if err != nil {
		return nil, err
	}
	maxScore, err := parseRedisScore(max, math.Inf(1))
	if err != nil {
		return nil, err
	}
	type entry struct {
		member string
		score  float64
	}
	entries := make([]entry, 0)
	for member, score := range f.zsets[key] {
		if score >= minScore && score <= maxScore {
			entries = append(entries, entry{member: member, score: score})
		}
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].score < entries[j].score })
	if count > 0 && int64(len(entries)) > count {
		entries = entries[:count]
	}
	result := make([]string, len(entries))
	for i, entry := range entries {
		result[i] = entry.member
	}
	return result, nil
}

func (f *fakeQueueStore) ZRem(key string, members ...interface{}) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	removed := int64(0)
	for _, member := range members {
		target := asString(member)
		if _, ok := f.zsets[key][target]; ok {
			delete(f.zsets[key], target)
			removed++
		}
	}
	return removed, nil
}

func (f *fakeQueueStore) ZCard(key string) int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return int64(len(f.zsets[key]))
}

func (f *fakeQueueStore) Del(keys ...string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, key := range keys {
		delete(f.lists, key)
		delete(f.zsets, key)
	}
	return nil
}

func asString(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		data, _ := json.Marshal(v)
		return string(data)
	}
}

func parseRedisScore(value string, fallback float64) (float64, error) {
	if value == "-inf" || value == "+inf" {
		return fallback, nil
	}
	return strconv.ParseFloat(value, 64)
}

func TestRetryBackoffExponentialAndCapped(t *testing.T) {
	cases := []struct {
		name       string
		retryCount int
		want       time.Duration
	}{
		{name: "nil_or_zero", retryCount: 0, want: DefaultRetryBaseDelay},
		{name: "first_retry", retryCount: 1, want: DefaultRetryBaseDelay},
		{name: "second_retry", retryCount: 2, want: 2 * DefaultRetryBaseDelay},
		{name: "third_retry", retryCount: 3, want: 4 * DefaultRetryBaseDelay},
		{name: "capped", retryCount: 99, want: DefaultRetryMaxDelay},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := retryBackoff(&TaskMessage{RetryCount: tc.retryCount})
			if got != tc.want {
				t.Fatalf("retryBackoff(%d) = %s, want %s", tc.retryCount, got, tc.want)
			}
		})
	}
}

func TestTaskQueueMetadata(t *testing.T) {
	listQueue := newTaskQueueWithStore(newFakeQueueStore())
	listMetadata := MetadataOf(listQueue)
	if listMetadata.Backend != TaskQueueBackendList {
		t.Fatalf("list backend = %q, want %q", listMetadata.Backend, TaskQueueBackendList)
	}
	if listMetadata.PendingKey != TaskQueueKey || listMetadata.ProcessingKey != TaskProcessingKey {
		t.Fatalf("list metadata keys = %+v", listMetadata)
	}

	streamQueue := newStreamTaskQueueWithStore(nil, StreamTaskQueueOptions{
		StreamKey:     "stream:key",
		DelayedKey:    "stream:delayed",
		DeadLetterKey: "stream:dead",
		ConsumerGroup: "group",
		ConsumerName:  "consumer",
		MaxLenApprox:  1000,
	})
	streamMetadata := MetadataOf(streamQueue)
	if streamMetadata.Backend != TaskQueueBackendStreams {
		t.Fatalf("stream backend = %q, want %q", streamMetadata.Backend, TaskQueueBackendStreams)
	}
	if streamMetadata.StreamKey != "stream:key" || streamMetadata.ConsumerGroup != "group" {
		t.Fatalf("stream metadata = %+v", streamMetadata)
	}

	if metadata := MetadataOf(nil); metadata.Backend != "none" {
		t.Fatalf("nil metadata backend = %q, want none", metadata.Backend)
	}
}

func TestTaskQueueDequeueAckAndRecover(t *testing.T) {
	store := newFakeQueueStore()
	q := newTaskQueueWithStore(store)

	if err := q.Enqueue(10, 20, "signin"); err != nil {
		t.Fatalf("Enqueue() error = %v", err)
	}
	if got := store.LLen(TaskQueueKey); got != 1 {
		t.Fatalf("pending len = %d, want 1", got)
	}

	msg, err := q.Dequeue(time.Millisecond)
	if err != nil {
		t.Fatalf("Dequeue() error = %v", err)
	}
	if msg.AccountID != 10 || msg.UserID != 20 || msg.TaskType != "signin" {
		t.Fatalf("message = %+v, want account/user/type", msg)
	}
	if msg.ProcessingAt == 0 || msg.raw == "" {
		t.Fatalf("dequeued message should have ProcessingAt and raw: %+v", msg)
	}
	if got := store.LLen(TaskQueueKey); got != 0 {
		t.Fatalf("pending len after dequeue = %d, want 0", got)
	}
	if got := store.LLen(TaskProcessingKey); got != 1 {
		t.Fatalf("processing len after dequeue = %d, want 1", got)
	}

	if err := q.Ack(msg); err != nil {
		t.Fatalf("Ack() error = %v", err)
	}
	if got := store.LLen(TaskProcessingKey); got != 0 {
		t.Fatalf("processing len after ack = %d, want 0", got)
	}
}

func TestTaskQueueRecoverStaleProcessing(t *testing.T) {
	store := newFakeQueueStore()
	q := newTaskQueueWithStore(store)
	stale := TaskMessage{
		AccountID:    1,
		UserID:       2,
		TaskType:     "all",
		CreatedAt:    time.Now().Add(-time.Hour).Unix(),
		ProcessingAt: time.Now().Add(-time.Hour).Unix(),
	}
	data, err := json.Marshal(stale)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.LPush(TaskProcessingKey, string(data)); err != nil {
		t.Fatal(err)
	}

	recovered, err := q.RecoverStaleProcessing(time.Minute)
	if err != nil {
		t.Fatalf("RecoverStaleProcessing() error = %v", err)
	}
	if recovered != 1 {
		t.Fatalf("recovered = %d, want 1", recovered)
	}
	if got := store.LLen(TaskProcessingKey); got != 0 {
		t.Fatalf("processing len = %d, want 0", got)
	}
	if got := store.LLen(TaskQueueKey); got != 1 {
		t.Fatalf("pending len = %d, want 1", got)
	}
	pending, _ := store.LRange(TaskQueueKey, 0, -1)
	var restored TaskMessage
	if err := json.Unmarshal([]byte(pending[0]), &restored); err != nil {
		t.Fatal(err)
	}
	if restored.ProcessingAt != 0 {
		t.Fatalf("ProcessingAt after recover = %d, want 0", restored.ProcessingAt)
	}
}

func TestTaskQueueDelayedPromotionAndDeadLetter(t *testing.T) {
	store := newFakeQueueStore()
	q := newTaskQueueWithStore(store)

	if err := q.Enqueue(1, 2, "all"); err != nil {
		t.Fatal(err)
	}
	msg, err := q.Dequeue(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.RequeueDelayed(msg, time.Millisecond); err != nil {
		t.Fatalf("RequeueDelayed() error = %v", err)
	}
	if got := store.LLen(TaskProcessingKey); got != 0 {
		t.Fatalf("processing len after delayed requeue = %d, want 0", got)
	}
	if got := store.ZCard(TaskDelayedKey); got != 1 {
		t.Fatalf("delayed len = %d, want 1", got)
	}
	time.Sleep(1100 * time.Millisecond)
	promoted, err := q.PromoteDueDelayed(10)
	if err != nil {
		t.Fatalf("PromoteDueDelayed() error = %v", err)
	}
	if promoted != 1 {
		t.Fatalf("promoted = %d, want 1", promoted)
	}
	if got := store.LLen(TaskQueueKey); got != 1 {
		t.Fatalf("pending len after promote = %d, want 1", got)
	}

	msg, err = q.Dequeue(time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := q.DeadLetter(msg, "failed too many times"); err != nil {
		t.Fatalf("DeadLetter() error = %v", err)
	}
	if got := store.LLen(TaskProcessingKey); got != 0 {
		t.Fatalf("processing len after dead letter = %d, want 0", got)
	}
	if got := store.LLen(TaskDeadLetterKey); got != 1 {
		t.Fatalf("dead letter len = %d, want 1", got)
	}
}
