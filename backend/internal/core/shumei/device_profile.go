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
	"math"
	mathrand "math/rand"
	stdhttp "net/http"
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

type screenProfile struct {
	Width  int
	Height int
	DPR    float64
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

var screenProfiles = []screenProfile{
	{Width: 1080, Height: 2340, DPR: 2.625},
	{Width: 1080, Height: 2400, DPR: 2.75},
	{Width: 720, Height: 1280, DPR: 1.5},
	{Width: 1080, Height: 2160, DPR: 2.625},
	{Width: 1080, Height: 2310, DPR: 2.625},
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
	phone := androidModels[randomInt(0, len(androidModels)-1)]
	screen := screenProfiles[randomInt(0, len(screenProfiles)-1)]
	ua := userAgentForModel(phone)
	uid := uuid.NewString()

	ep, err := rsaEncrypt(uid)
	if err != nil {
		return "", err
	}

	nowMs := time.Now().UnixMilli()
	startTime := nowMs - int64(randomInt(1800000, 5400000))
	nowCST := time.Now().In(chinaTimeZone)
	availHeight := screen.Height - randomInt(48, 128)

	env := map[string]interface{}{
		"protocol":       242,
		"organization":   organization,
		"appId":          appID,
		"os":             "web",
		"version":        "3.0.0",
		"sdkver":         "3.0.0",
		"box":            "",
		"rtype":          "all",
		"smid":           getSMID(uid),
		"subVersion":     "1.0.0",
		"time":           nowMs - startTime,
		"cdp":            0,
		"maxTouchPoints": 5,
		"connectionRtt":  0,
		"cpucount":       8,
		"battery": map[string]interface{}{
			"charging": 0,
			"level":    roundTo(0.6+mathrand.Float64()*0.35, 2),
		},
		"dg":            "5.0 " + strings.TrimPrefix(ua, "Mozilla/"),
		"gj":            "zh-CN",
		"rr":            "Google Inc.",
		"sv":            "Netscape",
		"qc":            "Mozilla",
		"ye":            8,
		"jq":            8,
		"lo":            []interface{}{},
		"bw":            "",
		"lr":            "Etc/GMT-8",
		"nr":            1,
		"no":            0,
		"br":            1,
		"ra":            0,
		"gt":            screen.Width,
		"wy":            screen.Width,
		"cj":            availHeight,
		"wt":            randomInt(100, 180),
		"hu":            []string{"chrome"},
		"documentExist": 1,
		"yi":            []string{"location"},
		"dx":            "UTF-8",
		"ig":            nowCST.Format("Mon Jan 02 2006 15:04:05 ") + "(GMT+08:00)",
		"ii":            1,
		"fs":            0,
		"ga":            0,
		"tk":            0,
		"rm":            0,
		"kr":            0,
		"nk":            0,
		"by":            "srgb",
		"ar":            0,
		"or":            0,
		"et":            0,
		"zc":            0,
		"fj":            0,
		"dc":            0,
		"vd":            0,
		"ni":            "",
		"hn":            "",
		"hv":            "48000_2_1_0_2_explicit_speakers|______",
		"de":            md5Hex(uid)[:16] + "|10011011111000111100001100101101111100110101001110000000000100000",
		"xt":            1,
		"vh":            0,
		"xc":            map[string]string{"red": "0"},
		"pm": map[string]interface{}{
			"default": roundTo(120.5+mathrand.Float64()*20, 1),
			"apple":   roundTo(120.5+mathrand.Float64()*20, 1),
			"serif":   roundTo(100+mathrand.Float64()*20, 1),
			"sans":    roundTo(120.5+mathrand.Float64()*20, 1),
			"mono":    roundTo(100+mathrand.Float64()*20, 1),
			"min":     roundTo(10+mathrand.Float64()*2, 1),
			"system":  roundTo(120.5+mathrand.Float64()*20, 1),
		},
		"ob": map[string]interface{}{
			"maxTouchPoints": 5,
			"touchEvent":     true,
			"touchStart":     true,
		},
		"incognito": map[string]interface{}{
			"getDirectoryExist":                0,
			"getDirectoryIncognito":            0,
			"maxTouchPointsExist":              1,
			"indexedDBIncognito":               0,
			"openDatabaseExist":                0,
			"openDatabaseIncognito":            0,
			"localStorageExist":                1,
			"localStorageIncognito":            0,
			"promiseExist":                     1,
			"promiseAllSettledExist":           1,
			"queryUsageAndQuotaIncognito":      0,
			"webkitRequestFileSystemIncognito": 0,
			"serviceWorkerExist":               1,
			"indexedDBExist":                   1,
			"browserName":                      "Chrome",
		},
		"t":           nowCST.Format("Mon Jan 02 2006 15:04:05 GMT-0700") + " (GMT+08:00)",
		"collectTime": randomInt(50, 130),
	}

	data, err := json.Marshal(env)
	if err != nil {
		return "", err
	}

	requestBody := deviceProfileRequest{
		AppID:        appID,
		Organization: organization,
		EP:           ep,
		Data:         base64.StdEncoding.EncodeToString(data),
		OS:           "web",
		Encode:       1,
		Compress:     0,
	}
	payload, err := json.Marshal(requestBody)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

// FetchDeviceID 调用数美 deviceprofile/v4 接口并返回带 B 前缀的 deviceId。
func FetchDeviceID(ctx context.Context, client *stdhttp.Client, endpoint string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
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

	payload, err := GenerateDeviceProfile()
	if err != nil {
		return "", fmt.Errorf("generate shumei device profile failed: %w", err)
	}

	time.Sleep(time.Duration(randomInt(500, 1500)) * time.Millisecond)

	req, err := stdhttp.NewRequestWithContext(ctx, stdhttp.MethodPost, endpoint, bytes.NewBufferString(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", RandomMarketUserAgent())
	req.Header.Set("Content-Type", "application/json;charset=UTF-8")
	req.Header.Set("Origin", "https://m.mcloud.139.com")
	req.Header.Set("Referer", "https://m.mcloud.139.com/portal/mobilecloud/index.html?path=newsignin")

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

	return NormalizeDeviceID(result.Detail.DeviceID), nil
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

func roundTo(value float64, digits int) float64 {
	pow := math.Pow10(digits)
	return math.Round(value*pow) / pow
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
