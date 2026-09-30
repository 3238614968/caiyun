package tasks

import (
	"errors"
	"strings"
	"testing"
	"time"

	"caiyun/internal/core/api"
)

type memoryMailDedup struct{ keys map[string]string }

func (m *memoryMailDedup) SetNX(key string, value interface{}, _ time.Duration) (bool, error) {
	if _, exists := m.keys[key]; exists {
		return false, nil
	}
	m.keys[key] = value.(string)
	return true, nil
}

func (m *memoryMailDedup) DelIfValue(key, value string) (bool, error) {
	if m.keys[key] != value {
		return false, nil
	}
	delete(m.keys, key)
	return true, nil
}

type recordingMailSender struct {
	recipients    []string
	rejectOnce    bool
	uncertainOnce bool
}

func (s *recordingMailSender) Send139Mail(_ *api.Mail139Session, _, recipientEmail, _, _ string) error {
	if s.rejectOnce {
		s.rejectOnce = false
		return api.ErrMail139Rejected
	}
	if s.uncertainOnce {
		s.uncertainOnce = false
		return errors.New("connection lost after sending")
	}
	s.recipients = append(s.recipients, recipientEmail)
	return nil
}

func TestMailMutualAmbiguousFailureKeepsClaim(t *testing.T) {
	store := &memoryMailDedup{keys: make(map[string]string)}
	sender := &recordingMailSender{uncertainOnce: true}
	job := &MailMutualTask{
		sender: sender, dedup: store, userID: 4, accountID: 1, phone: "13800000001",
		peers: []MailPeer{{AccountID: 2, Phone: "13800000002"}},
	}
	_, _, failed, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 20, 0)
	if err != nil || failed != 1 || len(store.keys) != 1 {
		t.Fatalf("ambiguous outcome should retain claim: failed=%d keys=%v err=%v", failed, store.keys, err)
	}
	sent, already, _, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 20, 0)
	if err != nil || sent != 0 || already != 1 || len(sender.recipients) != 0 {
		t.Fatalf("uncertain delivery was repeated: sent=%d already=%d recipients=%v err=%v", sent, already, sender.recipients, err)
	}
}

func TestMailMutualSendsEveryOwnedPeerOncePerMonth(t *testing.T) {
	store := &memoryMailDedup{keys: make(map[string]string)}
	sender := &recordingMailSender{}
	job := &MailMutualTask{
		sender: sender, dedup: store, userID: 4, accountID: 1, phone: "13800000001",
		peers: []MailPeer{
			{AccountID: 1, Phone: "13800000001"},
			{AccountID: 2, Phone: "13800000002"},
			{AccountID: 3, Phone: "13800000003"},
		},
	}
	for run := 0; run < 2; run++ {
		sent, already, failed, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 20, 0)
		if err != nil || failed != 0 {
			t.Fatalf("run %d: sent=%d failed=%d err=%v", run, sent, failed, err)
		}
		if run == 0 && (sent != 2 || already != 0) {
			t.Fatalf("first run sent=%d already=%d", sent, already)
		}
		if run == 1 && (sent != 0 || already != 2) {
			t.Fatalf("second run sent=%d already=%d", sent, already)
		}
	}
	if len(sender.recipients) != 2 || sender.recipients[0] != "13800000002@139.com" || sender.recipients[1] != "13800000003@139.com" {
		t.Fatalf("recipients = %v", sender.recipients)
	}
}

func TestMailMutualExplicitRejectionCanRetry(t *testing.T) {
	store := &memoryMailDedup{keys: make(map[string]string)}
	sender := &recordingMailSender{rejectOnce: true}
	job := &MailMutualTask{
		sender: sender, dedup: store, userID: 4, accountID: 1, phone: "13800000001",
		peers: []MailPeer{{AccountID: 2, Phone: "13800000002"}},
	}
	_, _, failed, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 20, 0)
	if err != nil || failed != 1 || len(store.keys) != 0 {
		t.Fatalf("rejected compose should release claim: failed=%d keys=%v err=%v", failed, store.keys, err)
	}
	sent, _, _, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 20, 0)
	if err != nil || sent != 1 {
		t.Fatalf("retry after explicit rejection sent=%d err=%v", sent, err)
	}
}

func TestMailMutualRotatesPastRejectedPeers(t *testing.T) {
	store := &memoryMailDedup{keys: make(map[string]string)}
	sender := &recordingMailSender{rejectOnce: true}
	job := &MailMutualTask{
		sender: sender, dedup: store, userID: 4, accountID: 1, phone: "13800000001",
		peers: []MailPeer{{AccountID: 2, Phone: "13800000002"}, {AccountID: 3, Phone: "13800000003"}},
	}
	_, _, failed, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 1, 0)
	if err != nil || failed != 1 {
		t.Fatalf("first peer rejection = failed %d, err %v", failed, err)
	}
	sent, _, _, _, err := job.sendPairs(&api.Mail139Session{}, "202609", 24*time.Hour, 1, 1)
	if err != nil || sent != 1 || len(sender.recipients) != 1 || sender.recipients[0] != "13800000003@139.com" {
		t.Fatalf("later peer was starved: sent=%d recipients=%v err=%v", sent, sender.recipients, err)
	}
}

func TestMailMutualPeriodAndPairIsolation(t *testing.T) {
	zone := time.FixedZone("CST", 8*3600)
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, zone)
	period, ttl := mailMutualPeriod(now)
	if period != "202609" || ttl <= 0 || ttl > 7*24*time.Hour {
		t.Fatalf("period=%q ttl=%s", period, ttl)
	}
	forward := mailMutualPairKey(8, 1, 2, period)
	reverse := mailMutualPairKey(8, 2, 1, period)
	otherOwner := mailMutualPairKey(9, 1, 2, period)
	if forward == reverse || forward == otherOwner || !strings.Contains(forward, "202609") {
		t.Fatalf("mail pair keys are not directional and owner-scoped: %q %q %q", forward, reverse, otherOwner)
	}
}
