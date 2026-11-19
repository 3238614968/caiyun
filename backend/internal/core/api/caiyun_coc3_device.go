package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"caiyun/internal/core/coc3wasm"
	corehttp "caiyun/internal/core/http"
)

// ExchangeCoc3DeviceToken is the single authenticated pre-draw SDK exchange.
// Only the official endpoint, current business session and two generated SDK
// headers are permitted; cloud Basic/JWT/device headers and shared cookies stay out.
func (api *Coc3API) ExchangeCoc3DeviceToken(ctx context.Context, token string, request coc3wasm.DeviceRequest) (coc3wasm.DeviceResponse, error) {
	var empty coc3wasm.DeviceResponse
	var body struct {
		Phone     string `json:"benefit_phone"`
		Encrypted string `json:"encryptedText"`
	}
	aurora := request.Headers["coc-aurora"]
	decoded, err := base64.StdEncoding.Strict().DecodeString(aurora)
	if token == "" || strings.ContainsAny(token, ";\r\n") || request.URL != coc3wasm.DeviceTokenURL || request.Method != "POST" || len(request.Body) > 32<<10 || json.Unmarshal(request.Body, &body) != nil || body.Phone != token || body.Encrypted == "" || err != nil || len(decoded) != 256 {
		return empty, fmt.Errorf("COC3 设备上报目标、当前会话或 SDK 报文无效")
	}
	resp, err := api.client.RequestWithContextOptions(ctx, "POST", coc3wasm.DeviceTokenURL, map[string]string{
		"Accept": "*/*", "Content-Type": "application/json", "coc-aurora": aurora,
		"User-Agent": coc3UserAgent, "Origin": coc3BaseURL, "Referer": coc3Referer,
		"Cookie": "COC3_LOGIN_TOKEN=" + token,
	}, request.Body, corehttp.WithoutDefaultHeaders(), corehttp.WithoutCookieJar(), corehttp.WithoutRetries(), corehttp.WithoutRedirects(), corehttp.WithRequestTimeout(10*time.Second))
	if err != nil {
		return empty, fmt.Errorf("COC3 设备上报请求失败")
	}
	defer resp.Body.Close()
	text, err := io.ReadAll(io.LimitReader(resp.Body, (64<<10)+1))
	if err != nil {
		return empty, fmt.Errorf("COC3 设备上报读取失败")
	}
	if len(text) > 64<<10 {
		return empty, fmt.Errorf("COC3 设备上报回执过大")
	}
	return coc3wasm.DeviceResponse{Status: resp.StatusCode, Body: text}, nil
}
