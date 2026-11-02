// Exchange eligibility pre-check.
//
// 上游兑换前有一道「资格校验」，App 的 H5 会先调
//
//	GET /ycloud/signin/page/receivePrizeDetails?marketId=<marketName>&prizeId=<id>
//
// 拿 `result.exchangeCode`，非 0 时**直接拦住按钮、根本不发兑换请求**。
// 已知资格不满足时应停止兑换；这与通用的 `610 / Error Code：GK` 拒绝分别处理，
// 不能由资格码直接推断所有 GK 的服务端原因。
//
// 语义取自 App 的 H5 bundle（03_协议/h5/js/newsignin.75080fd5.js）：
//
//	2302 -> 当月兑换第三方奖品数量已达上限，下月再来
//	2303 -> 需开启自动备份且本月成功备份照片
//	2305 -> 非目标用户（例如「仅限中国移动用户」的奖品，非移动号会被拦）
//
// 2026-10-05 线上核对：A 账号（133 号段，电信）对「QQ音乐绿钻会员月卡 / 哔哩哔哩会员月卡」
// 返回 `exchangeCode=2305`，对「菜鸟裹裹5元寄件券」返回 `0`；B 账号（157 号段，移动）三项全 `0`。
package services

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"caiyun/internal/utils"
)

// 兑换奖品的 marketName（兑换列表里每个商品的 `marketName` 字段）。
const exchangePrizeMarketID = "sign_in_3_ex"

type exchangeEligibility struct {
	code string // result.exchangeCode，空串表示没读到
	memo string // result.memo（奖品规则 HTML，截断后用于日志）
}

// exchangeEligibilityCodeMessages 对齐 App H5 的文案，便于日志一眼看懂。
var exchangeEligibilityCodeMessages = map[string]string{
	"2302": "当月兑换第三方奖品数量已达上限，下月再来",
	"2303": "需开启自动备份且本月成功备份照片才可兑换",
	"2305": "非目标用户（该奖品有用户范围限制，例如仅限中国移动用户）",
}

func fetchExchangeEligibilityContext(ctx context.Context, session *exchangeHTTPSession, authCtx *exchangeAuthContext, prizeID string) (*exchangeEligibility, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if session == nil || session.client == nil {
		return nil, fmt.Errorf("HTTP 会话为空")
	}
	values := url.Values{}
	values.Set("marketId", exchangePrizeMarketID)
	values.Set("prizeId", strings.TrimSpace(prizeID))
	endpoint := "https://m.mcloud.139.com/ycloud/signin/page/receivePrizeDetails?" + values.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Host = "m.mcloud.139.com"
	for key, value := range buildExchangeHeaders(authCtx, session, map[string]string{
		"Content-Type": "application/json",
	}) {
		req.Header.Set(key, value)
	}
	// 这一步不要求设备标识，去掉以免上游把它当成一次「带设备的动作」。
	req.Header.Del("isDeviceId")

	resp, err := session.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("兑换资格校验请求失败: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := utils.ReadLimitedBody(resp.Body, utils.DefaultMaxResponseBodyBytes)
	if err != nil {
		return nil, fmt.Errorf("读取兑换资格校验响应失败: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("兑换资格校验 HTTP %d: %s", resp.StatusCode, summarizeExchangeBody(string(bodyBytes)))
	}

	var raw struct {
		Code   json.Number `json:"code"`
		Msg    string      `json:"msg"`
		Result struct {
			ExchangeCode interface{} `json:"exchangeCode"`
			Memo         string      `json:"memo"`
		} `json:"result"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(bodyBytes)))
	decoder.UseNumber()
	if err := decoder.Decode(&raw); err != nil {
		return nil, fmt.Errorf("解析兑换资格校验响应失败: %w", err)
	}
	if raw.Code.String() != "0" {
		return nil, fmt.Errorf("兑换资格校验失败: code=%s msg=%s", raw.Code, strings.TrimSpace(raw.Msg))
	}
	code := exchangeStringFromAny(raw.Result.ExchangeCode)
	if _, ok := raw.Result.ExchangeCode.(bool); ok {
		return nil, fmt.Errorf("资格响应 exchangeCode 类型错误")
	}
	if _, err := strconv.ParseInt(code, 10, 64); err != nil {
		return nil, fmt.Errorf("资格响应缺少有效 exchangeCode")
	}
	memo := strings.TrimSpace(raw.Result.Memo)
	if len(memo) > 200 {
		memo = memo[:200] + "…"
	}
	return &exchangeEligibility{
		code: code,
		memo: memo,
	}, nil
}

// exchangeEligibilityBlockReason 返回非空表示「上游明确不允许兑换」，重试无意义。
func exchangeEligibilityBlockReason(e *exchangeEligibility) string {
	if e == nil || e.code == "" || e.code == "0" {
		return ""
	}
	reason := exchangeEligibilityCodeMessages[e.code]
	if reason == "" {
		reason = "上游未给出具体原因"
	}
	return fmt.Sprintf("兑换资格校验未通过：exchangeCode=%s（%s）", e.code, reason)
}

func exchangeStringFromAny(value interface{}) string {
	switch v := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(v)
	case float64:
		return fmt.Sprintf("%.0f", v)
	case json.Number:
		return v.String()
	case bool:
		if v {
			return "1"
		}
		return "0"
	default:
		return strings.TrimSpace(fmt.Sprintf("%v", v))
	}
}

// exchangeEligibilityNote 给失败日志补一句资格信息（不拦截，只加上下文）。
func exchangeEligibilityNote(e *exchangeEligibility) string {
	if e == nil || e.code == "" {
		return ""
	}
	note := "资格 exchangeCode=" + e.code
	if e.code != "0" {
		if reason := exchangeEligibilityCodeMessages[e.code]; reason != "" {
			note += "（" + reason + "）"
		}
	}
	return note
}
