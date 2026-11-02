package services

import (
	"caiyun/internal/core/auth"
	"caiyun/internal/models"
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"
	"time"
)

const exchangeAttemptWindow = 90 * time.Second

var exchangeChallengeGates = struct {
	sync.Mutex
	active map[string]bool
}{active: map[string]bool{}}

// getSlide has no independent challenge identifier. Serialize an account's
// whole challenge/submit sequence across products and exchange-account aliases.
func exchangeChallengeKey(account *models.ExchangeAccount) string {
	if account == nil {
		return ""
	}
	identity := ""
	if token, err := auth.ParseToken(account.Auth); err == nil && token.Phone != "" {
		identity = "phone:" + token.Phone
	}
	if identity == "" && len(strings.TrimSpace(account.Phone)) == 11 {
		identity = "phone:" + strings.TrimSpace(account.Phone)
	}
	if identity == "" && account.AccountID > 0 {
		identity = fmt.Sprintf("account:%d", account.AccountID)
	}
	if identity == "" {
		identity = fmt.Sprintf("exchange-account:%d", account.ID)
	}
	return fmt.Sprintf("exchange:challenge:%x", sha256.Sum256([]byte(identity)))
}

func acquireExchangeChallengeGate(store exchangeLockStore, account *models.ExchangeAccount) (func(), error) {
	key := exchangeChallengeKey(account)
	if key == "" {
		return func() {}, nil
	}
	exchangeChallengeGates.Lock()
	if exchangeChallengeGates.active[key] {
		exchangeChallengeGates.Unlock()
		return nil, fmt.Errorf("同一云盘账号正在抢兑，跳过本次执行以免覆盖滑块")
	}
	exchangeChallengeGates.active[key] = true
	exchangeChallengeGates.Unlock()
	var releaseOnce sync.Once
	releaseLocal := func() {
		releaseOnce.Do(func() {
			exchangeChallengeGates.Lock()
			delete(exchangeChallengeGates.active, key)
			exchangeChallengeGates.Unlock()
		})
	}
	if store == nil {
		return releaseLocal, nil
	}
	owner := randomLockValue(0)
	locked, err := store.SetNX(key, owner, exchangeAttemptWindow+20*time.Second)
	if err != nil {
		releaseLocal()
		return nil, fmt.Errorf("账号挑战会话锁不可用，未提交兑换")
	}
	if !locked {
		releaseLocal()
		return nil, fmt.Errorf("同一云盘账号正在抢兑，跳过本次执行以免覆盖滑块")
	}
	var releaseRemote sync.Once
	return func() { releaseRemote.Do(func() { _, _ = store.DelIfValue(key, owner); releaseLocal() }) }, nil
}
