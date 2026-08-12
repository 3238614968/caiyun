package services

import (
	"context"
	"sync"
	"time"

	"caiyun/internal/cache"
	"caiyun/internal/core/auth"
	"caiyun/internal/repository"
)

// TokenManager 统一管理账号 JWT Token 的缓存、刷新与健康状态。
type TokenManager struct {
	accountRepo  *repository.AccountRepository
	exchangeRepo *repository.ExchangeAccountRepository
	authMgr      *auth.Auth

	tokenCache     sync.Map // map[uint]*TokenInfo
	accountLocks   sync.Map // map[uint]*sync.Mutex
	preRefreshChan chan uint
	lockCache      *cache.RedisCache
	ctx            context.Context
	cancel         context.CancelFunc
}

// TokenInfo 描述账号当前 Token 状态。
type TokenInfo struct {
	JWTToken     string
	SSOToken     string
	Auth         string
	ExpiresAt    time.Time
	LastRefresh  time.Time
	HealthStatus string // healthy, warning, error
	ErrorMsg     string
}

type tokenRefreshSession struct {
	authStr    string
	authForJWT *auth.Auth
	jwtToken   string
	ssoToken   string
}

const (
	tokenRefreshErrorTTL          = 2 * time.Minute
	maxJWTRefreshFailures         = 3
	tokenRefreshHealthySkew       = 2 * time.Minute
	tokenPreRefreshSkew           = 5 * time.Minute
	tokenRefreshLockTTL           = 30 * time.Second
	tokenRefreshLockWait          = 10 * time.Second
	tokenRefreshPollDelay         = 500 * time.Millisecond
	defaultTokenPreRefreshMaxScan = 50
)

// NewTokenManager 创建 Token 管理器，并启动后台维护协程。
func NewTokenManager(
	accountRepo *repository.AccountRepository,
	exchangeRepo *repository.ExchangeAccountRepository,
	authMgr *auth.Auth,
) *TokenManager {
	ctx, cancel := context.WithCancel(context.Background())
	tm := &TokenManager{
		accountRepo:    accountRepo,
		exchangeRepo:   exchangeRepo,
		authMgr:        authMgr,
		preRefreshChan: make(chan uint, 100),
		ctx:            ctx,
		cancel:         cancel,
	}

	// 启动预刷新协程（包含扫描与消费预刷新队列）。
	go tm.preRefreshLoop()
	// 启动健康检查协程。
	go tm.healthCheckLoop()

	return tm
}

// SetDistributedLockCache 启用 Redis 分布式刷新锁，避免多副本同时刷新同一账号 Token。
func (tm *TokenManager) SetDistributedLockCache(redisCache *cache.RedisCache) {
	if tm == nil {
		return
	}
	tm.lockCache = redisCache
}

// Stop 停止 TokenManager 后台维护协程。
func (tm *TokenManager) Stop() {
	if tm == nil || tm.cancel == nil {
		return
	}
	tm.cancel()
}
