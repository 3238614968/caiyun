package api

import (
	corehttp "caiyun/internal/core/http"
	"encoding/json"
	"fmt"
	"math/big"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Coc3Eligibility struct {
	Eligible bool
	Reason   string
}

// The arrange endpoints are GETs. Fingerprint headers belong only on writes.
func (api *Coc3API) coc3Arrange(token, operation string, query url.Values) (*Coc3Response, error) {
	resp, err := api.client.RequestWithOptions("GET", coc3BaseURL+"/coc3/coc3-market-activity/arrange/"+operation+"?"+query.Encode(), api.coc3Headers(token, ""), nil, corehttp.WithoutCookieJar(), corehttp.WithoutDefaultHeaders())
	if err != nil {
		return nil, err
	}
	return api.decodeResponse(resp, operation)
}

func (api *Coc3API) Coc3LotteryEligibility(token string, activityID int) (*Coc3Eligibility, error) {
	resp, err := api.coc3Arrange(token, "checkQualificByActivityId", url.Values{"activityId": {strconv.Itoa(activityID)}})
	if err != nil {
		return nil, err
	}
	var data struct {
		OutOfStock      []json.RawMessage `json:"outOfStockList"`
		NoQualification []json.RawMessage `json:"noQualificationList"`
	}
	if len(resp.Data) > 0 && string(resp.Data) != "null" {
		if err := json.Unmarshal(resp.Data, &data); err != nil {
			return nil, fmt.Errorf("抽奖资格数据无效: %w", err)
		}
	}
	if len(data.OutOfStock)+len(data.NoQualification) > 0 {
		return &Coc3Eligibility{Reason: fmt.Sprintf("缺货%d项、无资格%d项", len(data.OutOfStock), len(data.NoQualification))}, nil
	}
	if !resp.Success {
		if resp.Code == "-403" || resp.resultCode == "-403" || resp.Code == "-9009" || resp.resultCode == "-9009" {
			return &Coc3Eligibility{Reason: resp.BusinessError("资格预检").Error()}, nil
		}
		return nil, resp.BusinessError("资格预检")
	}
	return &Coc3Eligibility{Eligible: true}, nil
}

// SubmitCoc3Lottery is the canvas PicLottery endpoint, distinct from mixedOrder.
// Only this endpoint accepts 60000/60010; those codes must not authorise login,
// task registration or an order. The caller verifies the remaining count too.
func (api *Coc3API) SubmitCoc3Lottery(token, wasmParams, riskToken, deviceID string, activityID int) (*Coc3Response, error) {
	if strings.TrimSpace(wasmParams) == "" || activityID <= 0 {
		return nil, fmt.Errorf("抽奖缺少令牌或有效活动ID")
	}
	headers := api.coc3Headers(token, "")
	headers["wasmParams"] = strings.TrimSpace(wasmParams)
	if strings.TrimSpace(riskToken) != "" {
		headers["gr-risk-p3"] = strings.TrimSpace(riskToken)
	}
	resp, err := api.client.RequestWithOptions("POST", coc3LotteryBase+"lottery", headers, map[string]interface{}{
		"activityId": activityID, "channelNo": "P00000123844", "operDfp": deviceID, "referer": coc3Referer,
	}, corehttp.WithoutCookieJar(), corehttp.WithoutDefaultHeaders())
	if err != nil {
		return nil, err
	}
	return api.decodeResponse(resp, "画布抽奖", "60000", "60010")
}

// Bean orders are an explicit account opt-in, never a fallback for a free draw.
func (api *Coc3API) SubmitCoc3BeanOrder(token, wasmParams string, activityID, skuID int) (*Coc3Response, error) {
	if strings.TrimSpace(wasmParams) == "" || activityID != 17761 || skuID <= 0 {
		return nil, fmt.Errorf("权益豆抽奖参数无效")
	}
	headers := api.coc3Headers(token, "")
	headers["wasmParams"] = strings.TrimSpace(wasmParams)
	resp, err := api.client.RequestWithOptions("POST", coc3OrderBase+"secKill/cmtOrder", headers, map[string]interface{}{
		"skuId": strconv.Itoa(skuID), "channelCode": "P00000011939", "selfChannelCode": "P00000123844",
		"smsCode": "", "dealType": 0, "basePayType": 32, "basePayAmount": 0,
		"actId": activityID, "activityId": activityID, "mixPaymentInfo": map[string]int{"digitalBean": 1},
	}, corehttp.WithoutCookieJar(), corehttp.WithoutDefaultHeaders())
	if err != nil {
		return nil, err
	}
	return api.decodeResponse(resp, "权益豆抽奖")
}

type Coc3ActivityConfig struct {
	Name          string
	Start, End    int64
	ZeroPriceSKUs map[int]bool
}

func (c *Coc3ActivityConfig) ActiveAt(now time.Time) bool {
	return c.Start > 0 && c.End >= c.Start && c.Start <= now.Unix() && now.Unix() <= c.End
}

func (api *Coc3API) Coc3ActivityConfig(token string, activityID int) (*Coc3ActivityConfig, error) {
	resp, err := api.coc3Arrange(token, "getProByActId", url.Values{
		"activityId": {strconv.Itoa(activityID)}, "batchId": {"0"}, "showLimit": {"3"},
		"seckillType": {"1"}, "from": {"page"}, "selfChannelCode": {"P00000067264"}, "t": {strconv.FormatInt(time.Now().Unix(), 10)},
	})
	if err != nil {
		return nil, err
	}
	if err := resp.BusinessError("活动配置"); err != nil {
		return nil, err
	}
	var data struct {
		Name  string          `json:"name"`
		Start json.RawMessage `json:"startTime"`
		End   json.RawMessage `json:"endTime"`
		Subs  []struct {
			Goods []struct {
				SKU   json.RawMessage `json:"skuid"`
				SKU2  json.RawMessage `json:"skuId"`
				Price json.RawMessage `json:"price"`
			} `json:"goodsList"`
		} `json:"subActivityList"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, fmt.Errorf("活动配置数据无效: %w", err)
	}
	parseTime := func(raw json.RawMessage) (int64, error) {
		value, err := coc3Scalar(raw)
		if err != nil {
			return 0, err
		}
		n, err := strconv.ParseInt(value, 10, 64)
		if err != nil || n <= 0 {
			return 0, fmt.Errorf("活动时间无效")
		}
		// Config snapshots include both epoch seconds and epoch milliseconds.
		if n > 1e12 {
			n /= 1000
		}
		return n, nil
	}
	config := &Coc3ActivityConfig{Name: data.Name, ZeroPriceSKUs: map[int]bool{}}
	config.Start, err = parseTime(data.Start)
	if err == nil {
		config.End, err = parseTime(data.End)
	}
	if err != nil {
		return nil, err
	}
	for _, sub := range data.Subs {
		for _, good := range sub.Goods {
			price, priceErr := coc3Scalar(good.Price)
			amount, ok := new(big.Rat).SetString(price)
			if priceErr != nil || !ok || amount.Sign() != 0 {
				continue
			}
			rawSKU := good.SKU
			if len(rawSKU) == 0 {
				rawSKU = good.SKU2
			}
			if sku, err := coc3Integer(rawSKU, "活动商品SKU"); err == nil && sku > 0 {
				config.ZeroPriceSKUs[sku] = true
			}
		}
	}
	return config, nil
}
