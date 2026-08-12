package queue

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ErrDeadLetterNotFound is returned when another administrator already
// archived/replayed the dead-letter entry selected by an approval request.
var ErrDeadLetterNotFound = errors.New("dead-letter entry not found")

// ErrOperationDeadLetterReplay is deliberately separate from generic queue
// replay: the Operation row must transition from failed to queued first, so
// execution fencing and the database outbox remain authoritative.
var ErrOperationDeadLetterReplay = errors.New("operation dead-letter requires operation replay")

// DeadLetterMessage is the redacted administrative view of one failed queue
// delivery. It contains only TaskMessage's public routing identifiers, never
// upstream credentials or an execution lease token.
type DeadLetterMessage struct {
	ID        string       `json:"id"`
	Reason    string       `json:"reason"`
	FailedAt  time.Time    `json:"failed_at"`
	Task      *TaskMessage `json:"task,omitempty"`
	Malformed bool         `json:"malformed"`
}

// DeadLetterReplayQueue is an optional capability over ReliableTaskQueue.
// Keeping it separate avoids breaking alternate queue implementations while
// allowing the admin transport to expose approval-gated replay consistently.
type DeadLetterReplayQueue interface {
	GetDeadLetter(id string) (*DeadLetterMessage, error)
	ListDeadLetters(limit int) ([]*DeadLetterMessage, error)
	ReplayDeadLetter(id string) (*TaskMessage, error)
	ArchiveDeadLetter(id string) error
}

type listDeadLetterEnvelope struct {
	DeadLetterID string       `json:"dead_letter_id"`
	Message      *TaskMessage `json:"message,omitempty"`
	RawMessage   string       `json:"raw_message,omitempty"`
	Reason       string       `json:"reason"`
	FailedAt     int64        `json:"failed_at"`
}

func encodeListDeadLetter(message *TaskMessage, raw, reason string) (string, error) {
	payload, err := json.Marshal(listDeadLetterEnvelope{
		DeadLetterID: uuid.NewString(),
		Message:      message,
		RawMessage:   raw,
		Reason:       strings.TrimSpace(reason),
		FailedAt:     time.Now().Unix(),
	})
	if err != nil {
		return "", fmt.Errorf("serialize dead-letter message: %w", err)
	}
	return string(payload), nil
}

func parseListDeadLetter(raw string) (*DeadLetterMessage, *TaskMessage, error) {
	var envelope listDeadLetterEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil {
		return nil, nil, err
	}
	if envelope.DeadLetterID == "" {
		sum := sha256.Sum256([]byte(raw))
		envelope.DeadLetterID = "legacy-" + hex.EncodeToString(sum[:])
	}
	item := &DeadLetterMessage{
		ID:        envelope.DeadLetterID,
		Reason:    envelope.Reason,
		FailedAt:  time.Unix(envelope.FailedAt, 0).UTC(),
		Task:      envelope.Message,
		Malformed: envelope.Message == nil,
	}
	return item, envelope.Message, nil
}

func resetReplayMessage(message *TaskMessage) *TaskMessage {
	if message == nil {
		return nil
	}
	clone := *message
	clone.RetryCount = 0
	clone.ProcessingAt = 0
	clone.StreamID = ""
	clone.raw = ""
	clone.CreatedAt = time.Now().Unix()
	return &clone
}
