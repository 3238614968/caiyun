package services

import (
	"caiyun/internal/models"
	"encoding/base64"
	"errors"
	"testing"
	"time"
)

func TestChallengeGateCoversAliasesButNotDifferentAccounts(t *testing.T) {
	credential := func(phone string) string {
		return "Basic " + base64.StdEncoding.EncodeToString([]byte("mobile:"+phone+":fixture"))
	}
	one := &models.ExchangeAccount{ID: 1, AccountID: 11, Auth: credential("13800000001")}
	alias := &models.ExchangeAccount{ID: 2, AccountID: 22, Auth: one.Auth}
	other := &models.ExchangeAccount{ID: 3, AccountID: 33, Auth: credential("13800000002")}
	release, err := acquireExchangeChallengeGate(nil, one)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if r, err := acquireExchangeChallengeGate(nil, alias); err == nil {
		r()
		t.Fatal("same phone alias allowed concurrent captcha")
	}
	r, err := acquireExchangeChallengeGate(nil, other)
	if err != nil {
		t.Fatal("different account blocked", err)
	}
	r()
	release()
	r, err = acquireExchangeChallengeGate(nil, alias)
	if err != nil {
		t.Fatal("account lock not released", err)
	}
	r()
}

type challengeLeaseFixture struct {
	fail     bool
	busy     bool
	owner    string
	ttl      time.Duration
	released int
}

func (f *challengeLeaseFixture) SetNX(key string, value interface{}, ttl time.Duration) (bool, error) {
	f.owner = value.(string)
	f.ttl = ttl
	if f.fail {
		return false, errors.New("redis down")
	}
	return !f.busy, nil
}
func (f *challengeLeaseFixture) DelIfValue(key, owner string) (bool, error) {
	if owner != f.owner {
		return false, errors.New("wrong owner")
	}
	f.released++
	return true, nil
}
func TestChallengeLeaseOutlivesDeadlineAndFailsClosed(t *testing.T) {
	a := &models.ExchangeAccount{ID: 987}
	for _, f := range []*challengeLeaseFixture{{fail: true}, {busy: true}, {}} {
		release, err := acquireExchangeChallengeGate(f, a)
		if f.fail || f.busy {
			if err == nil {
				t.Fatal("failed lease allowed exchange")
			}
			r, e := acquireExchangeChallengeGate(nil, a)
			if e != nil {
				t.Fatal("failed distributed lease leaked local lock")
			}
			r()
			continue
		}
		if err != nil || f.ttl <= exchangeAttemptWindow {
			t.Fatal("lease could expire before submission ends")
		}
		release()
		release()
		if f.released != 1 {
			t.Fatal("lease not released once with owner token")
		}
	}
}
