package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"strconv"
	"strings"
	"time"

	corehttp "caiyun/internal/core/http"
)

const (
	coc3RiskURL      = "https://yxfk.market.chinamobile.com:8081/mgt/decision-engine/udid/c1"
	coc3RiskAppID    = "50c71040541846d2e70e393e73271c7f"
	coc3RiskAlphabet = "S0DOZN9bBJyPV-qczRa3oYvhGlUMrdjW7m2CkE5_FuKiTQXnwe6pg8fs4HAtIL1x"
)

// Coc3ServiceRiskToken implements ConstID's lookup and one requested detection
// stage. Detection describes only the restricted WASM host, with absent browser
// features explicitly unavailable; it never fabricates canvas/GPU/App evidence.
// No account, login token or physical-device identifier is sent to this host.
func (api *Coc3API) Coc3ServiceRiskToken(ctx context.Context) (string, error) {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ"
	var suffix strings.Builder
	for i := 0; i < 32; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(alphabet))))
		if err != nil {
			return "", fmt.Errorf("COC3 匿名 SDK 标识生成失败")
		}
		suffix.WriteByte(alphabet[n.Int64()])
	}
	return api.coc3ServiceRiskToken(ctx, strconv.FormatInt(time.Now().UnixMilli(), 10)+suffix.String())
}

func coc3RiskLookupParam(localID string) (string, error) {
	if len(localID) != 45 || strings.IndexFunc(localID, func(r rune) bool { return !(r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z') }) >= 0 {
		return "", fmt.Errorf("COC3 匿名 SDK 标识无效")
	}
	data, err := json.Marshal(struct {
		ID     string `json:"lid"`
		Type   string `json:"lidType"`
		Cache  bool   `json:"cache"`
		AppKey string `json:"appKey"`
	}{localID, "0", true, coc3RiskAppID})
	if err != nil {
		return "", err
	}
	return base64.NewEncoding(coc3RiskAlphabet).EncodeToString(data), nil
}

func (api *Coc3API) coc3ServiceRiskToken(ctx context.Context, localID string) (string, error) {
	param, err := coc3RiskLookupParam(localID)
	if err != nil {
		return "", err
	}
	for stage := 0; stage < 2; stage++ {
		receipt, err := api.coc3RiskExchange(ctx, param)
		if err != nil {
			return "", err
		}
		if !validCoc3RiskValue(receipt.Data) {
			return "", fmt.Errorf("COC3 官方风险标识无效")
		}
		if receipt.Status == 1 || receipt.Status == 2 {
			return receipt.Data, nil
		}
		if receipt.Status != -4 || stage != 0 {
			return "", fmt.Errorf("COC3 官方风险令牌未确认（status=%d），不能替代浏览器核验", receipt.Status)
		}
		// This is a distinct, server-requested protocol stage, not a replay of
		// the lookup. Bind it only to this exchange's newly issued local ID.
		param, err = coc3RiskDetectionParam(receipt.Data, time.Now())
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("COC3 官方风险检测未确认")
}

type coc3RiskReceipt struct {
	Status int
	Data   string
}

func validCoc3RiskValue(value string) bool {
	return strings.TrimSpace(value) != "" && len(value) <= 1000 && !strings.ContainsAny(value, "\r\n") && !strings.HasPrefix(value, "#")
}

func coc3RiskDetectionParam(localID string, now time.Time) (string, error) {
	if !validCoc3RiskValue(localID) {
		return "", fmt.Errorf("COC3 检测缺少本次服务端标识")
	}
	_, offset := now.Zone()
	// These are the same logical navigator/storage capabilities exposed by
	// our isolated host. Unknown rendering/screen/function-source properties
	// stay unknown; hasLiedOs=true preserves the SDK's own mismatch finding.
	data, err := json.Marshal(map[string]interface{}{
		"lid": localID, "lidType": 1, "cache": true, "LANConnected": true,
		"ua": coc3UserAgent, "np": "Linux aarch64", "appName": "unknown", "appVersion": "unknown",
		"appCodeName": "unknown", "product": "unknown", "productSub": "unknown", "vendor": "unknown",
		"vendorSub": "unknown", "buildID": "unknown", "dm": -1, "cc": "unknown", "javaEnabled": "unknown",
		"flashVersion": "unknown", "hc": 4, "lug": "zh-CN", "lugs": "zh-CN", "dnt": "unknown", "ce": -1,
		"cd": -1, "res": "0;0", "ar": "0;0", "to": -offset / 60, "pr": -1, "ls": 1, "ss": 1, "ind": 0, "ab": 0, "od": 0,
		"ts": "0;false;false", "can": "unknown", "canvasSpoofed": false, "web": "unknown", "gi": "unknown",
		"hlb": false, "hlo": true, "hlr": false, "hll": false, "hl": 0, "vs": ";", "ws": ";", "db": 0, "cf": "unknown", "ct": 0, "appKey": coc3RiskAppID,
	})
	if err != nil {
		return "", err
	}
	return base64.NewEncoding(coc3RiskAlphabet).EncodeToString(data), nil
}

func (api *Coc3API) coc3RiskExchange(ctx context.Context, param string) (*coc3RiskReceipt, error) {
	response, err := api.client.RequestWithContextOptions(ctx, "GET", coc3RiskURL+"?", map[string]string{
		"appId": coc3RiskAppID, "param": param, "Accept": "application/json, text/plain, */*",
		"User-Agent": coc3UserAgent, "Origin": coc3BaseURL, "Referer": coc3Referer,
	}, nil, corehttp.WithoutDefaultHeaders(), corehttp.WithoutCookieJar(), corehttp.WithoutRetries(), corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(5*time.Second))
	if err != nil {
		return nil, fmt.Errorf("COC3 官方风险令牌查询失败")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || response.StatusCode != 200 {
		return nil, fmt.Errorf("COC3 官方风险令牌回执无效")
	}
	var receipt struct {
		Status *int   `json:"status"`
		Data   string `json:"data"`
	}
	// The SDK also uses the service's fixed JSONP fallback. Parse its single
	// JSON argument; never evaluate executable response text.
	text := strings.TrimSpace(string(body))
	if strings.HasPrefix(text, "_callback(") && strings.HasSuffix(text, ");") {
		text = strings.TrimSuffix(strings.TrimPrefix(text, "_callback("), ");")
	}
	if json.Unmarshal([]byte(text), &receipt) != nil || receipt.Status == nil {
		return nil, fmt.Errorf("COC3 官方风险令牌响应格式未确认")
	}
	return &coc3RiskReceipt{Status: *receipt.Status, Data: receipt.Data}, nil
}
