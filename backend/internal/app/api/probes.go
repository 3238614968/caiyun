package api

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"caiyun/internal/bootstrap"
	"caiyun/internal/queue"

	"github.com/gin-gonic/gin"
)

const readinessTimeout = 3 * time.Second

// configureTrustedProxies disables Gin's trust-all default. Operators must
// explicitly list reverse-proxy IP addresses or CIDRs in TRUSTED_PROXIES.
func configureTrustedProxies(engine *gin.Engine, raw string) error {
	proxies, err := parseTrustedProxies(raw)
	if err != nil {
		return err
	}
	if err := engine.SetTrustedProxies(proxies); err != nil {
		return fmt.Errorf("配置 TRUSTED_PROXIES 失败: %w", err)
	}
	return nil
}

func parseTrustedProxies(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, "none") {
		return nil, nil
	}
	parts := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\t' || r == '\n'
	})
	proxies := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		proxy := strings.TrimSpace(part)
		if proxy == "" {
			continue
		}
		if proxy == "*" || proxy == "0.0.0.0/0" || proxy == "::/0" {
			return nil, fmt.Errorf("TRUSTED_PROXIES 不允许信任所有来源: %s", proxy)
		}
		if ip := net.ParseIP(proxy); ip == nil {
			if _, _, err := net.ParseCIDR(proxy); err != nil {
				return nil, fmt.Errorf("TRUSTED_PROXIES 包含无效 IP/CIDR: %s", proxy)
			}
		}
		if _, exists := seen[proxy]; exists {
			continue
		}
		seen[proxy] = struct{}{}
		proxies = append(proxies, proxy)
	}
	return proxies, nil
}

func coreReadinessCheck(core *bootstrap.Core, taskQueue queue.ReliableTaskQueue) func(context.Context) error {
	return func(parent context.Context) error {
		ctx, cancel := context.WithTimeout(parent, readinessTimeout)
		defer cancel()
		return bootstrap.CheckReadiness(ctx, core, taskQueue)
	}
}
