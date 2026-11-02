package shumei

import (
	"bytes"
	"context"
	"crypto/md5"
	cryptorand "crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	mathrand "math/rand"
	stdhttp "net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

const (
	defaultDeviceProfileURL = "https://slw.h5cmpassport.com:9090/deviceprofile/v4"
	publicKeyBase64         = "MIGfMA0GCSqGSIb3DQEBAQUAA4GNADCBiQKBgQC8KHAcHbkCn5rxGgGJE+07tY+pt86D/oZ7sA51FaEBv2jgno2TI9zHJVYKJynmiKpixgwUcv93EfWIrU/p/UCs5Vu+odS3I4UBp3R7IZ1A0W01FkumAHYW2PQpMm8ueQKPLUq/idkpG/9b2JDv/qU+Ks36nbUPwlW4CjdfrV+V9QIDAQAB"
	organization            = "FXlyfmWg2AzwbrxDKSv5"
	appID                   = "default"
)

var chinaTimeZone = time.FixedZone("GMT+8", 8*60*60)

type androidModel struct {
	Model   string
	Build   string
	Android string
	Chrome  string
}

var androidModels = []androidModel{
	{Model: "23127HN0CC", Build: "UKQ1.230917.001", Android: "14", Chrome: "143.0.7499.146"},
	{Model: "24053PY09C", Build: "UP1A.231005.007", Android: "14", Chrome: "142.0.6522.118"},
	{Model: "23049RAD8C", Build: "TKQ1.221114.001", Android: "13", Chrome: "143.0.7499.146"},
	{Model: "PGP110", Build: "UKQ1.230917.001", Android: "14", Chrome: "141.0.6464.127"},
	{Model: "RMXP4721", Build: "UKQ1.230917.001", Android: "14", Chrome: "143.0.7499.146"},
	{Model: "M2012K10C", Build: "RP1A.200720.011", Android: "11", Chrome: "142.0.6522.118"},
	{Model: "V2324A", Build: "UP1A.231005.007", Android: "14", Chrome: "143.0.7499.146"},
	{Model: "RE58B1", Build: "TKQ1.221114.001", Android: "13", Chrome: "140.0.6385.82"},
	{Model: "22081212C", Build: "UKQ1.230917.001", Android: "14", Chrome: "143.0.7499.146"},
	{Model: "LLY-AN00", Build: "HONORLLY-AN00", Android: "14", Chrome: "142.0.6522.118"},
}

type deviceProfileRequest struct {
	AppID        string `json:"appId"`
	Organization string `json:"organization"`
	EP           string `json:"ep"`
	Data         string `json:"data"`
	OS           string `json:"os"`
	Encode       int    `json:"encode"`
	Compress     int    `json:"compress"`
}

type deviceProfileResponse struct {
	Code   int    `json:"code"`
	Msg    string `json:"msg"`
	Detail struct {
		DeviceID string `json:"deviceId"`
	} `json:"detail"`
}

// RandomMarketUserAgent 返回与新版移动云盘 H5 活动页匹配的 Android WebView UA。
func RandomMarketUserAgent() string {
	return userAgentForModel(androidModels[randomInt(0, len(androidModels)-1)])
}

// GenerateDeviceProfile 生成数美 deviceprofile/v4 所需的 JSON 请求体。
func GenerateDeviceProfile() (string, error) {
	return generateDeviceProfile(RandomMarketUserAgent())
}

func generateDeviceProfile(ua string) (string, error) {
	return generateDeviceProfileForPage(ua, defaultDevicePageURL)
}

const defaultDevicePageURL = "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin"

func generateDeviceProfileForPage(ua, pageURL string) (string, error) {
	payload, _, err := prepareDeviceProfileForPage(ua, pageURL)
	return payload, err
}

func prepareDeviceProfileForPage(ua, pageURL string) (string, string, error) {
	started := time.Now()
	uid := uuid.NewString()
	ep, err := rsaEncrypt(uid)
	if err != nil {
		return "", "", err
	}
	if pageURL == "" {
		pageURL = defaultDevicePageURL
	}
	// Supply the context this HTTP client actually has. Do not fabricate canvas,
	// audio, browser plugins, battery levels, screen dimensions or browsing age.
	smid := getSMID(uid)
	profile := map[string]interface{}{
		"protocol": officialSDKConfig.Protocol, "organization": organization,
		"appId": appID, "os": "web", "version": "3.0.0", "sdkver": "3.0.0",
		"box": "", "rtype": "all", "smid": smid, "subVersion": "1.0.0",
		"time": time.Since(started).Milliseconds(), "ua": ua, "url": pageURL,
	}
	encoded, err := sdkData(profile, uid)
	if err != nil {
		return "", "", fmt.Errorf("encode device SDK profile: %w", err)
	}
	payload, err := json.Marshal(deviceProfileRequest{
		AppID: appID, Organization: organization, EP: ep, Data: encoded,
		OS: "web", Encode: 5, Compress: 2,
	})
	if err != nil {
		return "", "", err
	}
	return string(payload), smid, nil
}

// FetchDeviceID 调用数美 deviceprofile/v4 接口并返回带 B 前缀的 deviceId。
func FetchDeviceID(ctx context.Context, client *stdhttp.Client, endpoint string) (string, error) {
	return FetchDeviceIDWithUserAgent(ctx, client, endpoint, RandomMarketUserAgent())
}

// FetchDeviceIDWithUserAgent keeps the device registration and subsequent
// activity requests in the same WebView identity.
func FetchDeviceIDWithUserAgent(ctx context.Context, client *stdhttp.Client, endpoint, userAgent string) (string, error) {
	return FetchDeviceIDForPage(ctx, client, endpoint, userAgent, defaultDevicePageURL)
}

// FetchDeviceIDForPage binds the encrypted URL and HTTP Referer to the same
// current activity page. No captured device or account credentials are reused.
func FetchDeviceIDForPage(ctx context.Context, client *stdhttp.Client, endpoint, userAgent, pageURL string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if client == nil {
		client = stdhttp.DefaultClient
	}
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		endpoint = strings.TrimSpace(os.Getenv("CAIYUN_SHUMEI_DEVICE_API"))
	}
	if endpoint == "" {
		endpoint = defaultDeviceProfileURL
	}

	userAgent = strings.TrimSpace(userAgent)
	if userAgent == "" {
		userAgent = RandomMarketUserAgent()
	}
	if strings.TrimSpace(pageURL) == "" {
		pageURL = defaultDevicePageURL
	}
	payload, smid, err := prepareDeviceProfileForPage(userAgent, pageURL)
	if err != nil {
		return "", fmt.Errorf("generate shumei device profile failed: %w", err)
	}

	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, endpoint, bytes.NewBufferString(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Origin", "https://m.mcloud.139.com")
	req.Header.Set("Referer", pageURL)
	req.Header.Set("X-Requested-With", "com.chinamobile.mcloud")
	req.Header.Set("Accept-Language", "zh,zh-CN;q=0.9,en-US;q=0.8,en;q=0.7")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := readLimited(resp.Body, 1<<20)
	if err != nil {
		return "", err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("deviceprofile http status=%d, body=%s", resp.StatusCode, string(body))
	}

	var result deviceProfileResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return "", fmt.Errorf("decode deviceprofile response failed: %w, body=%s", err, string(body))
	}
	if result.Code != 1100 || strings.TrimSpace(result.Detail.DeviceID) == "" {
		msg := strings.TrimSpace(result.Msg)
		if msg == "" {
			msg = string(body)
		}
		return "", fmt.Errorf("deviceprofile returned code=%d, msg=%s", result.Code, msg)
	}

	// SMSdk.getDeviceId returns "B" + the raw server value unconditionally.
	// A raw identifier that itself starts with B still needs the protocol prefix.
	deviceID := "B" + strings.TrimSpace(result.Detail.DeviceID)
	SeedDeviceCookies(client.Jar, pageURL, deviceID, smid)
	return deviceID, nil
}

// DeviceCookieName is keyed by SDK organization, not account phone/name.
func DeviceCookieName() string { return ".thumbcache_" + md5Hex(organization) }

// SeedDeviceCookies mirrors the SDK's document-cookie storage. Device values
// are encodeURIComponent encoded; the same smid was included in registration.
func SeedDeviceCookies(jar stdhttp.CookieJar, pageURL, deviceID, smid string) {
	if jar == nil || deviceID == "" {
		return
	}
	u, err := url.Parse(pageURL)
	if err != nil || u.Hostname() == "" {
		return
	}
	cookies := []*stdhttp.Cookie{{Name: DeviceCookieName(), Value: url.QueryEscape(CookieDeviceValue(deviceID)), Path: "/", Secure: u.Scheme == "https"}}
	if smid != "" {
		cookies = append(cookies, &stdhttp.Cookie{Name: "smidV2", Value: smid, Path: "/", Secure: u.Scheme == "https"})
	}
	jar.SetCookies(u, cookies)
}

// NormalizeDeviceID 统一返回移动云盘 market 接口需要的 B 前缀形式。
func NormalizeDeviceID(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToUpper(deviceID[:1]), "B") {
		return deviceID
	}
	return "B" + deviceID
}

// CookieDeviceValue 返回 .thumbcache_* cookie 中使用的不带 B 前缀形式。
func CookieDeviceValue(deviceID string) string {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return ""
	}
	if strings.HasPrefix(strings.ToUpper(deviceID[:1]), "B") {
		return deviceID[1:]
	}
	return deviceID
}

func userAgentForModel(model androidModel) string {
	return fmt.Sprintf(
		"Mozilla/5.0 (Linux; Android %s; %s Build/%s; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/%s Mobile Safari/537.36 MCloudApp/12.5.4 AppLanguage/zh-CN",
		model.Android,
		model.Model,
		model.Build,
		model.Chrome,
	)
}

func rsaEncrypt(plaintext string) (string, error) {
	der, err := base64.StdEncoding.DecodeString(publicKeyBase64)
	if err != nil {
		return "", err
	}
	key, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", err
	}
	publicKey, ok := key.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("invalid rsa public key")
	}
	ciphertext, err := rsa.EncryptPKCS1v15(cryptorand.Reader, publicKey, []byte(plaintext))
	if err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

func getSMID(uid string) string {
	base := time.Now().In(chinaTimeZone).Format("20060102150405") + md5Hex(uid) + "00"
	check := md5Hex("smsk_web_" + base)[:14]
	return base + check + "0"
}

func md5Hex(value string) string {
	sum := md5.Sum([]byte(value))
	return fmt.Sprintf("%x", sum)
}

func randomInt(min, max int) int {
	if max <= min {
		return min
	}
	return min + mathrand.Intn(max-min+1)
}

func readLimited(r io.Reader, maxBytes int64) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxBytes {
		return nil, fmt.Errorf("response body exceeds %d bytes", maxBytes)
	}
	return body, nil
}
