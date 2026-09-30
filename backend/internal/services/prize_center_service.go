package services

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"caiyun/internal/core/api"
	corehttp "caiyun/internal/core/http"
	"caiyun/internal/models"
	"caiyun/internal/repository"
	"gorm.io/gorm"
)

type prizeAccountRepository interface {
	FindMetadataByID(uint) (*models.Account, error)
}

type PendingPrizeList struct {
	AccountID   uint                   `json:"account_id"`
	Prizes      []api.PrizeCenterEntry `json:"prizes"`
	Total       int                    `json:"total"`
	RecordCount int                    `json:"record_count"`
	FetchedAt   time.Time              `json:"fetched_at"`
}

type prizeFetchFlight struct {
	done  chan struct{}
	value *PendingPrizeList
	err   error
}

// PrizeCenterService exposes only account-scoped prize metadata. Successful
// reads are cached briefly; concurrent refreshes share one upstream read.
type PrizeCenterService struct {
	accounts     prizeAccountRepository
	tokens       accountTokenProvider
	fetchEntries func(context.Context, *models.Account) ([]api.PrizeCenterEntry, error)
	mu           sync.Mutex
	cache        map[uint]*PendingPrizeList
	flights      map[uint]*prizeFetchFlight
}

func NewPrizeCenterService(accounts prizeAccountRepository, tokens accountTokenProvider) *PrizeCenterService {
	s := &PrizeCenterService{accounts: accounts, tokens: tokens, cache: make(map[uint]*PendingPrizeList), flights: make(map[uint]*prizeFetchFlight)}
	s.fetchEntries = s.loadEntries
	return s
}

func clonePendingPrizes(value *PendingPrizeList) *PendingPrizeList {
	if value == nil {
		return nil
	}
	copy := *value
	copy.Prizes = append([]api.PrizeCenterEntry{}, value.Prizes...)
	return &copy
}

func (s *PrizeCenterService) GetPendingPrizes(ctx context.Context, userID, accountID uint, isAdmin, refresh bool) (*PendingPrizeList, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if accountID == 0 {
		return nil, ErrAccountNotFound
	}
	accountRepo := s.accounts
	if contextual, ok := accountRepo.(interface {
		WithContext(context.Context) *repository.AccountRepository
	}); ok {
		accountRepo = contextual.WithContext(ctx)
	}
	account, err := accountRepo.FindMetadataByID(accountID)
	if errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrAccountNotFound) {
		return nil, ErrAccountNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("读取领奖账号信息失败: %w", err)
	}
	if account == nil || (!isAdmin && account.UserID != userID) {
		return nil, ErrAccountNotFound
	}
	s.mu.Lock()
	if cached := s.cache[accountID]; !refresh && cached != nil && time.Since(cached.FetchedAt) < 30*time.Second {
		s.mu.Unlock()
		return clonePendingPrizes(cached), nil
	}
	if flight := s.flights[accountID]; flight != nil {
		s.mu.Unlock()
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-flight.done:
			return clonePendingPrizes(flight.value), flight.err
		}
	}
	flight := &prizeFetchFlight{done: make(chan struct{})}
	s.flights[accountID] = flight
	s.mu.Unlock()
	entries, err := s.fetchEntries(ctx, account)
	var value *PendingPrizeList
	if err == nil {
		prizes := api.PrizeCenterUnclaimed(entries)
		value = &PendingPrizeList{AccountID: accountID, Prizes: prizes, Total: len(prizes), RecordCount: len(entries), FetchedAt: time.Now().UTC()}
	}
	s.mu.Lock()
	if err == nil {
		if len(s.cache) >= 256 {
			for id, item := range s.cache {
				if time.Since(item.FetchedAt) > 30*time.Second {
					delete(s.cache, id)
				}
			}
			if len(s.cache) >= 256 {
				for id := range s.cache {
					delete(s.cache, id)
					break
				}
			}
		}
		s.cache[accountID] = value
	}
	flight.value, flight.err = value, err
	delete(s.flights, accountID)
	close(flight.done)
	s.mu.Unlock()
	return clonePendingPrizes(value), err
}

func (s *PrizeCenterService) loadEntries(ctx context.Context, account *models.Account) ([]api.PrizeCenterEntry, error) {
	if s.tokens == nil {
		return nil, fmt.Errorf("账号凭据服务不可用")
	}
	token, err := s.tokens.GetToken(account.ID)
	if err != nil {
		return nil, err
	}
	if token == nil || token.JWTToken == "" {
		return nil, fmt.Errorf("账号没有可用的授权凭据")
	}
	client := corehttp.NewClient()
	client.SetMarketAccount(account.Phone)
	client.SetAuth(account.Auth)
	if token.Auth != "" {
		client.SetAuth(token.Auth)
	}
	client.SetJWTToken(token.JWTToken)
	client.SetSSOToken(token.SSOToken)
	return api.NewCaiyunAPI(client).PrizeCenterAllEntriesContext(ctx)
}
