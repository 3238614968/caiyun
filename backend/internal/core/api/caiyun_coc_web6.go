package api

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"caiyun/internal/core/auth"
	corehttp "caiyun/internal/core/http"
)

// Public SPKI from the official dailyLoginEvent aurora implementation. This is
// independent of the COC3 wheel format and needs neither Node nor a browser.
const cocWeb6PublicKey = "MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA0a66eNBd/ioaGnEM2AV+ctQX8VYt6dszzI8zOv26Kc1A7ex/" +
	"cu/txmbB6WrU5ARoTeS0gk3KS21e/xP1BR1FUqL/+YJ4v8gpFY21/2gWuEPVBoHEd34FMpDRuiQkpmrvuuRF+QJQIg5ASE" +
	"Bpwz4n15ltl7Fd4E+NLp/JRJxzNR55ZeVLLFaucWYe8FIvX5BKx8i+sJWZYDoxTbjrBTy2CZDZObT+PxhOBXsCkolsG8Ux" +
	"+tIAoU+UAVSQ0qT62z/sHPVPngHAqmLP/sF3IM6Mrub7BqhwJe3rvddqrw+Q2Jg8upZ/oUlZvXtQZCxJXtg0eJVdKTmnE6" +
	"hD+/M99ukEoQIDAQAB"

const (
	CocWeb6SignActivity = "2095709942208745472"
	CocWeb6DrawActivity = "2095755977334779904"
	cocWeb6TouchCode    = "P00000001090"
)

type CocWeb6API struct {
	client *corehttp.Client
	codec  *Coc3API
}

func NewCocWeb6API(client *corehttp.Client) *CocWeb6API {
	return &CocWeb6API{client: client, codec: NewCoc3API(client)}
}

func encryptCocWeb6Login(ticket string, publicKey *rsa.PublicKey, entropy io.Reader) (map[string]string, map[string]string, error) {
	var raw [8]byte
	if _, err := io.ReadFull(entropy, raw[:]); err != nil {
		return nil, nil, err
	}
	key := []byte(hex.EncodeToString(raw[:]))
	plain, err := json.Marshal(struct {
		VerifyType int    `json:"verifyType"`
		Ext        string `json:"ext"`
	}{VerifyType: 18, Ext: ticket})
	if err != nil {
		return nil, nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	padding := block.BlockSize() - len(plain)%block.BlockSize()
	plain = append(plain, bytes.Repeat([]byte{byte(padding)}, padding)...)
	encrypted := make([]byte, len(plain))
	for offset := 0; offset < len(plain); offset += block.BlockSize() {
		block.Encrypt(encrypted[offset:], plain[offset:])
	}
	wrappedKey, err := rsa.EncryptPKCS1v15(entropy, publicKey, key)
	if err != nil {
		return nil, nil, err
	}
	return map[string]string{"cocEnContent": base64.StdEncoding.EncodeToString(encrypted)}, map[string]string{
		"Coc-En-Version": "1486234121417629696", "coc-aurora-v": "2", "coc-aurora": base64.StdEncoding.EncodeToString(wrappedKey),
	}, nil
}

func cocWeb6Headers(token string) map[string]string {
	headers := map[string]string{
		"Accept": "application/json, text/plain, */*", "Content-Type": "application/json;charset=utf-8",
		"Origin": coc3BaseURL, "Referer": coc3BaseURL + "/coc/web6/dailyLoginEvent/",
		"User-Agent": coc3UserAgent, "X-Requested-With": coc3AppPackage,
	}
	if token != "" {
		headers["Cookie"] = "User-Token=" + token
	}
	return headers
}

func (api *CocWeb6API) Login(authorization, phone string) (string, error) {
	if strings.TrimSpace(authorization) == "" || strings.TrimSpace(phone) == "" {
		return "", fmt.Errorf("COC web6 缺少账号凭据")
	}
	ctx, cancel := context.WithTimeout(context.Background(), coc3LoginTimeout)
	defer cancel()
	ticket, err := auth.NewAuth(api.client).QuerySpecTokenForSourceContext(ctx, authorization, phone, coc3SourceID)
	if err != nil {
		return "", fmt.Errorf("COC web6 出票失败: %w", err)
	}
	der, err := base64.StdEncoding.DecodeString(cocWeb6PublicKey)
	if err != nil {
		return "", err
	}
	parsed, err := x509.ParsePKIXPublicKey(der)
	if err != nil {
		return "", err
	}
	key, ok := parsed.(*rsa.PublicKey)
	if !ok {
		return "", fmt.Errorf("COC web6 公钥不是RSA")
	}
	body, extra, err := encryptCocWeb6Login(ticket, key, rand.Reader)
	if err != nil {
		return "", err
	}
	headers := cocWeb6Headers("")
	for key, value := range extra {
		headers[key] = value
	}
	resp, err := api.client.RequestWithContextOptions(ctx, "POST", coc3BaseURL+"/coc/user/loginCheck", headers, body,
		corehttp.WithoutCookieJar(), corehttp.WithoutDefaultHeaders(), corehttp.WithoutRetries(), corehttp.WithoutRedirects())
	if err != nil {
		return "", err
	}
	cookies := resp.Cookies()
	if _, err := api.codec.decodeLoginResponse(resp, "COC web6 登录"); err != nil {
		return "", err
	}
	for _, cookie := range cookies {
		active := cookie.MaxAge > 0 || cookie.MaxAge == 0 && (cookie.Expires.IsZero() || cookie.Expires.After(time.Now()))
		if cookie.Name == "User-Token" && strings.TrimSpace(cookie.Value) != "" && active {
			return cookie.Value, nil
		}
	}
	return "", fmt.Errorf("COC web6 登录未返回 User-Token，不能确认登录")
}

func (api *CocWeb6API) request(token, operation string, payload interface{}) (*Coc3Response, error) {
	resp, err := api.client.RequestWithOptions("POST", coc3BaseURL+"/coc/activities/"+operation, cocWeb6Headers(token), payload,
		corehttp.WithoutCookieJar(), corehttp.WithoutDefaultHeaders(), corehttp.WithoutRetries(), corehttp.WithoutRedirects())
	if err != nil {
		return nil, err
	}
	envelope, err := api.codec.decodeResponse(resp, "COC web6/"+operation, "0")
	if err != nil {
		return nil, err
	}
	return envelope, envelope.BusinessError("COC web6/" + operation)
}

func (api *CocWeb6API) ClockInDates(token string) ([]string, error) {
	resp, err := api.request(token, "prize/consecutiveClockInQuery", map[string]string{"activityId": CocWeb6SignActivity})
	if err != nil {
		return nil, err
	}
	var data struct {
		Dates []string `json:"clockInDateList"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil || data.Dates == nil {
		return nil, fmt.Errorf("7日打卡缺少有效日期记录")
	}
	return data.Dates, nil
}

func (api *CocWeb6API) Prize(token, activityID string) (*Coc3Response, error) {
	if activityID != CocWeb6SignActivity && activityID != CocWeb6DrawActivity {
		return nil, fmt.Errorf("不支持的COC web6奖池")
	}
	return api.request(token, "prize/templatePrize", map[string]string{"activityId": activityID, "touchCode": cocWeb6TouchCode})
}

func (api *CocWeb6API) DrawCount(token string) (int, error) {
	resp, err := api.request(token, "prize/drawDayNum", map[string]string{"activityId": CocWeb6DrawActivity})
	if err != nil {
		return 0, err
	}
	var data struct {
		Count json.RawMessage `json:"dayRemainNum"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return 0, err
	}
	return coc3Integer(data.Count, "7日接力剩余次数")
}

func (api *CocWeb6API) GrowthLevel(token, province string) (string, error) {
	resp, err := api.request(token, "growth/query", map[string]string{"touchCode": "P00000106669", "provinceCode": province})
	if err != nil {
		return "", err
	}
	var data struct {
		Level string `json:"level"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil || data.Level == "" {
		return "", fmt.Errorf("网龄成长未返回等级")
	}
	return data.Level, nil
}

// Official page checkEvaluate shows the form when data == 0. The Python
// reference inverted this condition; false means not evaluated, not completed.
func (api *CocWeb6API) Evaluated(token string) (bool, error) {
	resp, err := api.request(token, "growth/query/evaluate", map[string]interface{}{})
	if err != nil {
		return false, err
	}
	value := strings.Trim(string(resp.Data), `"`)
	switch value {
	case "true", "1":
		return true, nil
	case "false", "0":
		return false, nil
	default:
		return false, fmt.Errorf("网龄评价未返回有效状态")
	}
}

func (api *CocWeb6API) Evaluate(token string, score int, label, content string) error {
	if score < 1 || score > 5 || strings.TrimSpace(label) == "" {
		return fmt.Errorf("评价需有效的星级和评语标签")
	}
	_, err := api.request(token, "growth/submit/evaluate", map[string]interface{}{"score": score, "label": label, "content": content})
	return err
}
