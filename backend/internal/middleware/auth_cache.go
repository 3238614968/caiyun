package middleware

import (
	"caiyun/internal/envutil"
	"caiyun/internal/repository"
	"caiyun/internal/security/authcache"
	"caiyun/pkg/jwt"
	"context"
	"time"
)

// authUserSnapshot is the short-lived server-side view used to reconcile JWT
// claims with mutable user state such as a role or token-version change.
type authUserSnapshot = authcache.Snapshot

func getAuthUserSnapshot(ctx context.Context, userRepo *repository.UserRepository, claims *jwt.Claims) (*authUserSnapshot, error) {
	now := time.Now()
	if snapshot, ok := authcache.Load(claims.UserID, claims.TokenVersion, now); ok {
		return snapshot, nil
	}

	user, err := userRepo.WithContext(ctx).FindByID(claims.UserID)
	if err != nil {
		authcache.Delete(claims.UserID)
		return nil, err
	}
	return authcache.Store(user.ID, authcache.Entry{
		Username:     user.Username,
		Role:         user.Role,
		TokenVersion: user.TokenVersion,
		ExpiresAt:    now.Add(authUserCacheTTL()),
	}), nil
}

func authUserCacheTTL() time.Duration {
	// 缩短默认 TTL 到 10 秒，降低改密/重置后旧会话仍可用的窗口。
	return envutil.Duration("AUTH_USER_CACHE_TTL", 10*time.Second)
}

// InvalidateAuthUserCache 在用户改密、重置密码、删除等场景主动清除缓存，
// 让 token_version 变更立即生效。
func InvalidateAuthUserCache(userID uint) {
	authcache.Delete(userID)
}
