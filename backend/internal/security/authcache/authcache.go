package authcache

import (
	"sync"
	"time"
)

type Entry struct {
	Username     string
	Role         string
	TokenVersion int
	ExpiresAt    time.Time
}

type Snapshot struct {
	Username     string
	Role         string
	TokenVersion int
}

var cache sync.Map // map[uint]Entry

func Load(userID uint, tokenVersion int, now time.Time) (*Snapshot, bool) {
	cached, ok := cache.Load(userID)
	if !ok {
		return nil, false
	}
	entry, ok := cached.(Entry)
	if !ok || entry.TokenVersion != tokenVersion || !now.Before(entry.ExpiresAt) {
		cache.Delete(userID)
		return nil, false
	}
	return &Snapshot{Username: entry.Username, Role: entry.Role, TokenVersion: entry.TokenVersion}, true
}

func Store(userID uint, entry Entry) *Snapshot {
	cache.Store(userID, entry)
	return &Snapshot{Username: entry.Username, Role: entry.Role, TokenVersion: entry.TokenVersion}
}

func Delete(userID uint) {
	cache.Delete(userID)
}
