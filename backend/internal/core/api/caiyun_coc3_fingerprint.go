package api

import (
	"bytes"
	"context"
	"crypto/md5"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"time"

	corehttp "caiyun/internal/core/http"
)

const coc3FingerprintURL = coc3BaseURL + "/coc3/gr/risk-ability/dfpRisk/getDfpOuterCode?channel=WEB"

// An HTTP success without a receipt is not evidence of a valid fingerprint,
// nor does it identify why the upstream service withheld that receipt.
var ErrCoc3FingerprintReceiptMissing = errors.New("COC3 官方 BSFIT 服务未返回指纹回执")

// Coc3Fingerprint obtains only a real server receipt. Unavailable native-host
// rendering, fonts, plugins and audio remain unavailable. No browser/App device
// fingerprint is copied and no timeout-generated serial number is accepted.
func (api *Coc3API) Coc3Fingerprint(ctx context.Context, token string) (string, error) {
	if token == "" || len(token) > 512 || strings.ContainsAny(token, ";\r\n") {
		return "", fmt.Errorf("COC3 指纹查询缺少有效当前会话")
	}
	var nonce [16]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return "", fmt.Errorf("COC3 指纹加密随机源失败")
	}
	bsk := strings.ToUpper(hex.EncodeToString(nonce[:]))
	now := time.Now()
	fields := coc3FingerprintFields(now)
	plaintext, err := coc3FingerprintJSON(fields)
	if err != nil {
		return "", err
	}
	encrypted, err := coc3SM4CBC(plaintext, bsk)
	if err != nil {
		return "", err
	}
	headers := api.coc3Headers(token, "")
	headers["Content-Type"] = "application/json"
	response, err := api.client.RequestWithContextOptions(ctx, "POST", coc3FingerprintURL, headers, struct {
		Key   string `json:"bsk"`
		Input string `json:"inputItem"`
		Mode  int    `json:"bse"`
	}{bsk, encrypted, 1}, corehttp.WithoutDefaultHeaders(), corehttp.WithoutCookieJar(), corehttp.WithoutRetries(), corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(8*time.Second))
	if err != nil {
		return "", fmt.Errorf("COC3 官方指纹请求失败")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (64<<10)+1))
	if err != nil || len(body) > 64<<10 || response.StatusCode != 200 {
		return "", fmt.Errorf("COC3 官方指纹回执 HTTP/大小未确认")
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return "", fmt.Errorf("%w（HTTP 200，正文 %d 字节）；不能确认风控通过", ErrCoc3FingerprintReceiptMissing, len(body))
	}
	var receipt struct {
		DFP     string          `json:"dfp"`
		Expiry  json.RawMessage `json:"exp"`
		Code    json.RawMessage `json:"code"`
		Success *bool           `json:"success"`
	}
	if json.Unmarshal(body, &receipt) != nil {
		kind := "empty"
		if trimmed := bytes.TrimSpace(body); len(trimmed) > 0 {
			switch trimmed[0] {
			case '<':
				kind = "markup"
			case '"':
				kind = "json-string"
			case '{':
				kind = "json-object"
			case '[':
				kind = "json-array"
			default:
				kind = "other"
			}
		}
		return "", fmt.Errorf("COC3 官方指纹回执格式无效（类型=%q，首类=%s，字节=%d）", response.Header.Get("Content-Type"), kind, len(body))
	}
	expiry, expiryErr := coc3Scalar(receipt.Expiry)
	expiresAt, parseErr := strconv.ParseInt(expiry, 10, 64)
	code, codeErr := coc3Scalar(receipt.Code)
	if codeErr != nil || (code != "" && code != "0" && code != "200") || (receipt.Success != nil && !*receipt.Success) || expiryErr != nil || parseErr != nil || expiresAt <= time.Now().Add(30*time.Second).UnixMilli() || len(receipt.DFP) <= 64 || len(receipt.DFP) > 4096 || strings.ContainsAny(receipt.DFP, "\r\n") || strings.HasPrefix(receipt.DFP, "#") {
		return "", fmt.Errorf("COC3 官方指纹未取得有效且未过期的服务端记录")
	}
	return receipt.DFP, nil
}

func coc3FingerprintJSON(fields []coc3DfpField) ([]byte, error) {
	var result bytes.Buffer
	result.WriteByte('{')
	for i, field := range fields {
		if i > 0 {
			result.WriteByte(',')
		}
		key, _ := json.Marshal(field.Key)
		value, err := json.Marshal(field.Value)
		if err != nil {
			return nil, err
		}
		result.Write(key)
		result.WriteByte(':')
		result.Write(value)
	}
	result.WriteByte('}')
	return result.Bytes(), nil
}

func coc3FingerprintFields(now time.Time) []coc3DfpField {
	_, offset := now.Zone()
	zone := now.Location().String()
	if zone == "Local" || zone == "CST" {
		switch offset {
		case 28800:
			zone = "Asia/Shanghai"
		case 0:
			zone = "UTC"
		default:
			zone = "not available"
		}
	}
	// Only the isolated navigator and in-memory host state are available.
	// A native worker has no rendered pixels, GPU, fonts, plugins or audio.
	components := []coc3DfpField{
		{"isIncognito", "0"}, {"isRiskBrowser", "1"},
		{"userAgent", coc3UserAgent}, {"webdriver", "not available"}, {"language", "zh-CN"},
		{"colorDepth", "not available"}, {"deviceMemory", "not available"}, {"devicePixelRatio", "not available"},
		{"hardwareConcurrency", "4"}, {"screenResolution", ","}, {"availableScreenResolution", "not available"},
		{"timezoneOffset", strconv.Itoa(-offset / 60)}, {"timezone", zone},
		{"sessionStorage", "true"}, {"localStorage", "true"}, {"indexedDb", "false"}, {"addBehavior", "false"}, {"openDatabase", "false"},
		{"cpuClass", "not available"}, {"navigatorPlatform", "Linux aarch64"}, {"plugins", "not available"},
		{"canvas", "not available"}, {"webgl", "not available"}, {"adBlock", "false"},
		{"hasLiedLanguages", "false"}, {"hasLiedResolution", "false"}, {"hasLiedOs", "true"}, {"hasLiedBrowser", "false"},
		{"touchSupport", "0,false,false"}, {"fonts", "not available"}, {"audio", "not available"},
	}
	var all, cross strings.Builder
	crossKeys := map[string]bool{"colorDepth": true, "pixelRatio": true, "timezoneOffset": true, "cpuClass": true, "navigatorPlatform": true, "touchSupport": true, "fontsHash": true, "webglVendorAndRenderer": true}
	for _, field := range components {
		all.WriteString(field.Value)
		if crossKeys[field.Key] {
			cross.WriteString(field.Value)
		}
	}
	fields := append([]coc3DfpField(nil), components...)
	// The official parser derives these labels from the configured HTTP UA;
	// they describe that logical request profile, not physical hardware.
	emptyMD5 := md5.Sum(nil)
	fields = append(fields, []coc3DfpField{{"javaEnabled", "0"}, {"mimeTypesHash", fmt.Sprintf("%x", emptyMD5)},
		{"cookieEnabled", "0"}, {"flashVersion", "0"}, {"doNotTrack", "unknown"},
		{"browserName", "Android Browser"}, {"browserVersion", "4.0"}, {"os", "Android"}, {"osVersion", "16"},
		{"browserEngine", "Blink"}, {"deviceType", "mobile"},
		{"webSmartID", coc3Murmur128([]byte(cross.String()))[:16] + coc3Murmur128([]byte(all.String()))[16:]},
		{"isSelenium", "0"}, {"custID", "JYZX"}, {"platform", "WEB"}}...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Key < fields[j].Key })
	fields = append(fields, coc3DfpField{"hashCode", coc3DfpHash(fields)}, coc3DfpField{"algID", "l4ehhNODhH"}, coc3DfpField{"timestamp", strconv.FormatInt(now.UnixMilli(), 10)})
	return fields
}
