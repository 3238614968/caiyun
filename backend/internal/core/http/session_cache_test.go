package http

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestPreparedSessionsCoalesceAndRemainAccountAndCredentialScoped(t *testing.T) {
	a, b := NewClient(), NewClient()
	var calls atomic.Int32
	init := func() error { calls.Add(1); return nil }
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := a.PrepareSessionOnce("portal", init); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if calls.Load() != 1 {
		t.Fatalf("duplicate preparations=%d", calls.Load())
	}
	_ = b.PrepareSessionOnce("portal", init)
	a.SetJWTToken("new-jwt")
	_ = a.PrepareSessionOnce("portal", init)
	a.SetSSOToken("new-sso")
	_ = a.PrepareSessionOnce("portal", init)
	if calls.Load() != 4 {
		t.Fatal("account or credentials shared stale preparation")
	}
}

func TestFailedPreparationDoesNotBlockRecovery(t *testing.T) {
	c := NewClient()
	calls := 0
	init := func() error {
		calls++
		if calls == 1 {
			return errors.New("temporary outage")
		}
		return nil
	}
	if c.PrepareSessionOnce("portal", init) == nil {
		t.Fatal("failure hidden")
	}
	if c.PrepareSessionOnce("portal", init) != nil || c.PrepareSessionOnce("portal", init) != nil || calls != 2 {
		t.Fatal("failed preparation cached")
	}
}
