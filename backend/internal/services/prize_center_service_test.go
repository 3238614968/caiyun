package services

import (
	"caiyun/internal/core/api"
	"caiyun/internal/models"
	"context"
	"errors"
	"testing"
)

type prizeAccountFixture struct{ accounts map[uint]*models.Account }

func (f prizeAccountFixture) FindMetadataByID(id uint) (*models.Account, error) {
	if a := f.accounts[id]; a != nil {
		return a, nil
	}
	return nil, ErrAccountNotFound
}

func TestPrizeCenterScopesCacheAndShowsAllPendingActivities(t *testing.T) {
	repo := prizeAccountFixture{accounts: map[uint]*models.Account{1: {ID: 1, UserID: 7}, 2: {ID: 2, UserID: 8}}}
	s := NewPrizeCenterService(repo, nil)
	reads := 0
	s.fetchEntries = func(context.Context, *models.Account) ([]api.PrizeCenterEntry, error) {
		reads++
		return []api.PrizeCenterEntry{{OID: "claimed", Flag: 2}, {OID: "late", Flag: 1, ExpireTime: "2026-10-10"}, {OID: "soon", Flag: 1, ExpireTime: "2026-09-30"}}, nil
	}
	list, err := s.GetPendingPrizes(context.Background(), 7, 1, false, false)
	if err != nil || list.Total != 2 || list.RecordCount != 3 || list.Prizes[0].OID != "soon" {
		t.Fatalf("pending prizes=%+v err=%v", list, err)
	}
	list.Prizes[0].OID = "mutated"
	cached, err := s.GetPendingPrizes(context.Background(), 7, 1, false, false)
	if err != nil || reads != 1 || cached.Prizes[0].OID != "soon" {
		t.Fatal("cache was not isolated or duplicated upstream reads")
	}
	if _, err := s.GetPendingPrizes(context.Background(), 8, 1, false, false); !errors.Is(err, ErrAccountNotFound) {
		t.Fatal("another user could read cached prize metadata")
	}
	if _, err := s.GetPendingPrizes(context.Background(), 7, 1, false, true); err != nil || reads != 2 {
		t.Fatal("manual refresh did not query current upstream state")
	}
	if _, err := s.GetPendingPrizes(context.Background(), 7, 2, true, false); err != nil {
		t.Fatal("administrator could not read selected managed account")
	}
}

func TestPrizeCenterCanceledWaitDoesNotStartAnotherFetch(t *testing.T) {
	s := NewPrizeCenterService(prizeAccountFixture{accounts: map[uint]*models.Account{1: {ID: 1, UserID: 7}}}, nil)
	s.flights[1] = &prizeFetchFlight{done: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.GetPendingPrizes(ctx, 7, 1, false, true); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled wait=%v", err)
	}
}

type failingPrizeAccountFixture struct{ err error }

func (f failingPrizeAccountFixture) FindMetadataByID(uint) (*models.Account, error) {
	return nil, f.err
}

func TestPrizeCenterPreservesAccountReadErrors(t *testing.T) {
	readError := errors.New("database connection unavailable")
	s := NewPrizeCenterService(failingPrizeAccountFixture{err: readError}, nil)
	_, err := s.GetPendingPrizes(context.Background(), 7, 1, false, false)
	if !errors.Is(err, readError) || errors.Is(err, ErrAccountNotFound) {
		t.Fatalf("database failure was hidden as account not found: %v", err)
	}
}

func TestPrizeCenterDoesNotCacheFailedCredentialReads(t *testing.T) {
	s := NewPrizeCenterService(prizeAccountFixture{accounts: map[uint]*models.Account{1: {ID: 1, UserID: 7}}}, nil)
	readError := errors.New("credentials unavailable")
	s.fetchEntries = func(context.Context, *models.Account) ([]api.PrizeCenterEntry, error) {
		return nil, readError
	}
	if result, err := s.GetPendingPrizes(context.Background(), 7, 1, false, false); result != nil || !errors.Is(err, readError) {
		t.Fatalf("failed credentials produced a prize list: %+v %v", result, err)
	}
	s.fetchEntries = func(context.Context, *models.Account) ([]api.PrizeCenterEntry, error) {
		return []api.PrizeCenterEntry{{OID: "restored", Flag: 1}}, nil
	}
	if result, err := s.GetPendingPrizes(context.Background(), 7, 1, false, false); err != nil || result.Total != 1 {
		t.Fatalf("updated credentials were blocked by failure cache: %+v %v", result, err)
	}
}
