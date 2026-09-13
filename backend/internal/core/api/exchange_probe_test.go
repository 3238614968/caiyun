package api

import (
	"net"
	"net/http"
	"os"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"
)

// 抢兑整点延迟真实探测（仅显式开启时运行，常规 CI/测试一律跳过）：
//
//	CAIYUN_EXCHANGE_PROBE=1            开启探测
//	CAIYUN_EXCHANGE_PROBE_WAVE=20      模拟整点并发波大小（默认 20）
//	CAIYUN_EXCHANGE_PROBE_TARGET=<url> 覆盖探测 URL
//
// 走只读的 timestamp 接口，不消耗兑换次数、无业务副作用，用于量化本机到
// 抢兑域名的连接/TLS/TTFB、本地与服务端时钟偏差，以及整点并发波的首发延迟。

const defaultProbeTarget = "https://m.mcloud.139.com/ycloud/api/timestamp"

func probeEnabled() bool { return os.Getenv("CAIYUN_EXCHANGE_PROBE") == "1" }

func probeTargetURL() string {
	if v := os.Getenv("CAIYUN_EXCHANGE_PROBE_TARGET"); v != "" {
		return v
	}
	return defaultProbeTarget
}

func newProbeClient() *http.Client {
	return &http.Client{
		Timeout: 15 * time.Second,
		Transport: &http.Transport{
			DialContext:         (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second,
			MaxIdleConnsPerHost: 64,
			IdleConnTimeout:     90 * time.Second,
		},
	}
}

type probeResult struct {
	ttfb      time.Duration
	serverNow time.Time
	hasTime   bool
	status    int
}

func probeOnce(client *http.Client, target string) (probeResult, error) {
	req, err := http.NewRequest(http.MethodGet, target, nil)
	if err != nil {
		return probeResult{}, err
	}
	req.Header.Set("User-Agent", MarketUserAgent)

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return probeResult{}, err
	}
	defer resp.Body.Close()

	result := probeResult{ttfb: time.Since(start), status: resp.StatusCode}
	if d, derr := http.ParseTime(resp.Header.Get("Date")); derr == nil {
		result.serverNow = d
		result.hasTime = true
	}
	return result, nil
}

func TestExchangeLatencyProbe(t *testing.T) {
	if !probeEnabled() {
		t.Skip("未设置 CAIYUN_EXCHANGE_PROBE=1，跳过整点延迟真实探测")
	}
	target := probeTargetURL()
	wave := 20
	if v := os.Getenv("CAIYUN_EXCHANGE_PROBE_WAVE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			wave = n
		}
	}

	client := newProbeClient()
	defer client.CloseIdleConnections()

	cold, err := probeOnce(client, target)
	if err != nil {
		t.Fatalf("冷连接探测失败: %v", err)
	}
	t.Logf("冷连接(含DNS/TCP/TLS) TTFB=%s status=%d", cold.ttfb.Round(time.Millisecond), cold.status)

	const samples = 10
	ttfr := make([]time.Duration, 0, samples)
	var skews []time.Duration
	for i := 0; i < samples; i++ {
		s, e := probeOnce(client, target)
		if e != nil {
			t.Errorf("热连接探测失败: %v", e)
			continue
		}
		ttfr = append(ttfr, s.ttfb)
		if s.hasTime {
			skews = append(skews, time.Since(s.serverNow))
		}
	}
	sort.Slice(ttfr, func(a, b int) bool { return ttfr[a] < ttfr[b] })
	if len(ttfr) > 0 {
		t.Logf("热连接 TTFB: min=%s p50=%s max=%s",
			ttfr[0].Round(time.Millisecond), percentile(ttfr, 0.5).Round(time.Millisecond), ttfr[len(ttfr)-1].Round(time.Millisecond))
	}
	if len(skews) > 0 {
		sort.Slice(skews, func(a, b int) bool { return absDur(skews[a]) < absDur(skews[b]) })
		t.Logf("本地相对服务端时钟偏差中位数≈%s（正值=本地慢，整点抢兑受此影响）",
			skews[len(skews)/2].Round(time.Millisecond))
	}

	boundary := time.Now().Truncate(time.Minute).Add(time.Minute)
	if time.Until(boundary) > 20*time.Second {
		boundary = time.Now().Truncate(10 * time.Second).Add(10 * time.Second)
	}
	t.Logf("对齐到 %s 发出 %d 并发请求…", boundary.Format("15:04:05.000"), wave)

	start := make(chan struct{})
	var wg sync.WaitGroup
	var latMu sync.Mutex
	latencies := make([]time.Duration, wave)
	firstAfterBoundary := make(chan time.Duration, wave)

	for i := 0; i < wave; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			t0 := time.Now()
			_, e := probeOnce(client, target)
			elapsed := time.Since(t0)
			if e != nil {
				elapsed = -1
			} else {
				firstAfterBoundary <- time.Since(boundary)
			}
			latMu.Lock()
			latencies[idx] = elapsed
			latMu.Unlock()
		}(i)
	}

	time.Sleep(time.Until(boundary))
	fireAt := time.Now()
	close(start)
	wg.Wait()
	close(firstAfterBoundary)

	var good []time.Duration
	for _, l := range latencies {
		if l >= 0 {
			good = append(good, l)
		}
	}
	sort.Slice(good, func(a, b int) bool { return good[a] < good[b] })

	var earliest time.Duration
	got := false
	for d := range firstAfterBoundary {
		if !got || d < earliest {
			earliest, got = d, true
		}
	}
	t.Logf("波发射点距边界 %s；成功 %d/%d", fireAt.Sub(boundary).Round(time.Millisecond), len(good), wave)
	if got {
		t.Logf("整点后首个响应到达耗时 ≈ %s", earliest.Round(time.Millisecond))
	}
	if len(good) > 0 {
		t.Logf("波内单请求往返: min=%s p50=%s max=%s",
			good[0].Round(time.Millisecond), percentile(good, 0.5).Round(time.Millisecond), good[len(good)-1].Round(time.Millisecond))
	}
	if got && earliest > 500*time.Millisecond {
		t.Logf("提示: 首发距整点 >500ms 主要受网络 RTT/时钟偏差限制；可结合上面的 skew 校时，或加大并发/burst。")
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
