package api

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"time"

	corehttp "caiyun/internal/core/http"

	"github.com/google/uuid"
)

const (
	nativeEventCollectorURL = "https://dc.cmicapm.com:7080"
	// Public attribution key shipped with the official App H5 (not a secret).
	nativeEventPublicKey = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC7UEOgJK80WRLyfndV9m99Py+rdTf0FEGEBe8swW7yg2bXYyVfLwzp3v23BLauaWKYCLlqUVaS+r44HelWptNxzF/Xxatj1Lwc1VNiDQKGQv3RNYb1H3bZZ4SYwGCderumDup4F2PsEk7aT95TLiU8H4S/jz3LrSiF7yD4yqRmDQIDAQAB"
)

// nativeEventIdentity matches forge.encrypt(phone, "NONE") in the official
// H5. This encoding is only for its attribution endpoint, never authentication.
func nativeEventIdentity(phone string) (string, error) {
	if len(phone) != 11 || phone[0] != '1' || strings.IndexFunc(phone, func(r rune) bool { return r < '0' || r > '9' }) >= 0 {
		return "", fmt.Errorf("官方行为上报缺少有效的当前账号")
	}
	der, err := base64.StdEncoding.DecodeString(nativeEventPublicKey)
	if err != nil {
		return "", fmt.Errorf("官方行为公钥无效")
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", fmt.Errorf("官方行为公钥无效")
	}
	public, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("官方行为公钥类型无效")
	}
	ciphertext := new(big.Int).Exp(new(big.Int).SetBytes([]byte(phone)), big.NewInt(int64(public.E)), public.N)
	return base64.StdEncoding.EncodeToString(ciphertext.FillBytes(make([]byte, public.Size()))), nil
}

// nativeAIEvent describes an actual feature action. Callers must verify its
// real output before reporting success; collector receipts are not task records.
type nativeAIEvent struct {
	Name   string
	Params map[string]interface{}
}

func (api *CaiyunAPI) reportNativeAIEvents(now time.Time, page, sdkVersion string, customFields map[string]string, events []nativeAIEvent) error {
	if len(events) == 0 || len(events) > 4 || !(strings.HasPrefix(page, "https://yun.139.com/ai-helper/") || strings.HasPrefix(page, "https://yun.139.com/aiTools/")) {
		return fmt.Errorf("官方行为上报上下文无效")
	}
	identity, err := nativeEventIdentity(api.client.GetMarketAccount())
	if err != nil {
		return err
	}
	uid := strings.TrimSpace(api.client.GetUserDomainID())
	if uid == "" {
		return fmt.Errorf("官方行为上报缺少 userDomainId")
	}
	batchEvents := make([]map[string]interface{}, 0, len(events))
	session := uuid.NewString()
	for index, event := range events {
		if event.Name != "aidialogue_click" && event.Name != "aitool_success" && event.Name != "aitool_picsave_result" {
			return fmt.Errorf("官方行为事件无效")
		}
		params := map[string]interface{}{"page_path": page, "user_domain_id": uid, "event_index": now.UnixMilli() + int64(index), "$user_unique_id_type": "rsa_phone_id"}
		for key, value := range event.Params {
			params[key] = value
		}
		encoded, e := json.Marshal(params)
		if e != nil {
			return fmt.Errorf("官方行为事件参数无效")
		}
		batchEvents = append(batchEvents, map[string]interface{}{"event": event.Name, "params": string(encoded), "local_time_ms": now.UnixMilli(), "is_bav": 0, "session_id": session})
	}
	custom, err := json.Marshal(customFields)
	if err != nil {
		return fmt.Errorf("官方行为来源参数无效")
	}
	web, err := api.postNativeEvent("/webid", map[string]interface{}{"app_id": 10000005, "url": page, "user_agent": MarketUserAgent, "referer": "", "user_unique_id": identity})
	if err != nil {
		return err
	}
	if strings.TrimSpace(web.WebID) == "" {
		return fmt.Errorf("官方行为服务未返回 web_id")
	}
	parts := strings.Split(api.client.GetClientInfo(), "|")
	field := func(index int) string {
		if index < len(parts) {
			return parts[index]
		}
		return ""
	}
	osName, osVersion, _ := strings.Cut(strings.ToLower(field(8)), " ")
	w, h, _ := strings.Cut(strings.ToLower(field(9)), "x")
	width, _ := strconv.Atoi(w)
	height, _ := strconv.Atoi(h)
	header := map[string]interface{}{
		"app_id": 10000005, "os_name": osName, "os_version": osVersion, "device_model": field(5),
		"language": "zh-CN", "platform": "wap", "sdk_version": sdkVersion, "sdk_lib": "js",
		"timezone": 8, "tz_offset": -28800, "resolution": fmt.Sprintf("%dx%d", width, height),
		"browser": "Chrome", "browser_version": "108.0.0.0", "referrer": "", "referrer_host": "",
		"width": width, "height": height, "screen_width": width, "screen_height": height, "custom": string(custom),
	}
	_, err = api.postNativeEvent("/list", []interface{}{map[string]interface{}{"events": batchEvents, "user": map[string]string{"user_unique_id": identity, "web_id": web.WebID, "user_unique_id_type": "rsa_phone_id"}, "header": header, "local_time": now.Unix(), "verbose": 1}})
	return err
}

type nativeEventResponse struct {
	Error *int   `json:"e"`
	WebID string `json:"web_id"`
}

func (api *CaiyunAPI) postNativeEvent(path string, payload interface{}) (*nativeEventResponse, error) {
	if path != "/webid" && path != "/list" {
		return nil, fmt.Errorf("官方行为上报路径无效")
	}
	resp, err := api.client.RequestWithOptions("POST", nativeEventCollectorURL+path, map[string]string{
		"Content-Type": "application/json; charset=UTF-8", "User-Agent": MarketUserAgent,
		"Origin": "https://yun.139.com", "Referer": "https://yun.139.com/", "Accept": "*/*",
	}, payload, corehttp.WithoutDefaultHeaders(), corehttp.WithoutCookieJar(), corehttp.WithoutRetries(), corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(20*time.Second))
	if err != nil {
		return nil, fmt.Errorf("官方行为上报%s请求失败: %w", path, err)
	}
	body, err := api.client.ReadResponseBody(resp)
	if err != nil {
		return nil, fmt.Errorf("官方行为上报%s读取失败: %w", path, err)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("官方行为上报%s HTTP %d", path, resp.StatusCode)
	}
	var result nativeEventResponse
	if json.Unmarshal([]byte(body), &result) != nil {
		return nil, fmt.Errorf("官方行为上报%s响应格式无效", path)
	}
	if result.Error == nil || *result.Error != 0 {
		return nil, fmt.Errorf("官方行为上报%s未确认接收", path)
	}
	return &result, nil
}
