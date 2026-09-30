package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// National_Getprize（全网领奖专区）。
//
// 只做「盘点 + 单个奖品领奖」。发码前需完成滑块校验（puzzleOffset），
// 再 sendPrizeSms 发码并带短验码 acceptPrizeV2；服务端另有每日短信上限
// （1002 今日短信发送已达上限）。滑块不自动求解，偏移由调用方提供。
const prizeCenterBase = MobileMarketURL + "/prize"

// 领奖专区奖品记录的 flag 取值。
const (
	PrizeFlagUnclaimed = 1
	PrizeFlagClaimed   = 2
	PrizeFlagExpired   = 10
)

// PrizeCenterEntry 一条奖品记录。
type PrizeCenterEntry struct {
	OID        string `json:"oId"`
	PrizeName  string `json:"prizeName"`
	PrizeID    string `json:"prizeId"`
	Type       string `json:"type"`
	MarketID   string `json:"marketid"`
	MarketName string `json:"marketname"`
	ExpireTime string `json:"expireTime"`
	Flag       int    `json:"flag"`
	VerifyCode int    `json:"verifycode"`
}

// PrizeCenterPage 一页奖品记录。
type PrizeCenterPage struct {
	Entries         []PrizeCenterEntry
	TotalPages      int
	TotalPagesKnown bool
}

// PrizeCenterSpaceGiftInfo includes the auxiliary space-gift entry documented
// separately from OID-based prize records. Some deployments return a bare status.
func (api *CaiyunAPI) PrizeCenterSpaceGiftInfo() (*CaiyunResponse, error) {
	body, err := api.marketRawRequest("GET", prizeCenterBase+"Api/checkPrize/getSpaceGiftInfo", nil, nil)
	if err != nil {
		return nil, err
	}
	var response struct {
		CaiyunResponse
		Status *int `json:"status"`
	}
	decoder := json.NewDecoder(strings.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&response); err != nil {
		return nil, err
	}
	if response.Code == nil && response.Status != nil {
		response.Code = 0
		response.Result = map[string]interface{}{"status": *response.Status}
	}
	return &response.CaiyunResponse, nil
}

// PrizeCenterLogPage 拉取一页奖品记录。
func (api *CaiyunAPI) PrizeCenterLogPage(page, pageSize int) (*PrizeCenterPage, error) {
	return api.PrizeCenterLogPageContext(context.Background(), page, pageSize)
}

func (api *CaiyunAPI) PrizeCenterLogPageContext(ctx context.Context, page, pageSize int) (*PrizeCenterPage, error) {
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 200 {
		pageSize = 100
	}
	resp, err := api.marketJSONRequestContext(ctx,
		"GET",
		fmt.Sprintf("%sApi/checkPrize/getUserPrizeLogPageV2?currPage=%d&pageSize=%d", prizeCenterBase, page, pageSize),
		nil,
		nil,
	)
	if err != nil {
		return nil, err
	}
	if resp != nil && !resp.IsSuccess() {
		return nil, fmt.Errorf("奖品记录查询失败: code=%v %s", resp.Code, resp.MessageText())
	}
	pageResult, valid := resultMap(resp)
	if !valid {
		return nil, fmt.Errorf("奖品记录响应缺少有效分页数据")
	}
	items, present := pageResult["result"]
	if !present {
		items, present = pageResult["records"]
	}
	if !present {
		return nil, fmt.Errorf("奖品记录响应缺少记录数组")
	}
	if items != nil {
		if _, valid := items.([]interface{}); !valid {
			return nil, fmt.Errorf("奖品记录数组格式无效")
		}
	}
	return parsePrizeCenterPage(resp), nil
}

// PrizeCenterAllEntries 分页拉取全部奖品记录。
func (api *CaiyunAPI) PrizeCenterAllEntries() ([]PrizeCenterEntry, error) {
	return api.PrizeCenterAllEntriesContext(context.Background())
}

func (api *CaiyunAPI) PrizeCenterAllEntriesContext(ctx context.Context) ([]PrizeCenterEntry, error) {
	var all []PrizeCenterEntry
	for page := 1; page <= 50; page++ {
		result, err := api.PrizeCenterLogPageContext(ctx, page, 100)
		if err != nil {
			return all, err
		}
		all = append(all, result.Entries...)
		if len(result.Entries) == 0 || (result.TotalPagesKnown && result.TotalPages <= page) || (!result.TotalPagesKnown && len(result.Entries) < 100) {
			return all, nil
		}
	}
	return all, fmt.Errorf("奖品记录超过分页读取上限，未取得完整列表")
}

// PrizeCenterUnclaimed 返回未领取的奖品，按到期时间升序。
func PrizeCenterUnclaimed(entries []PrizeCenterEntry) []PrizeCenterEntry {
	unclaimed := make([]PrizeCenterEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.Flag == PrizeFlagUnclaimed {
			unclaimed = append(unclaimed, entry)
		}
	}
	sortPrizeEntriesByExpire(unclaimed)
	return unclaimed
}

// PrizeCenterNeedSMS 查询该奖品是否需要短信验证码（返回 nil 表示不需要）。
func (api *CaiyunAPI) PrizeCenterNeedSMS(oid string) (*CaiyunResponse, error) {
	if strings.TrimSpace(oid) == "" {
		return nil, fmt.Errorf("奖品 oid 为空")
	}
	return api.marketJSONRequest(
		"GET",
		prizeCenterBase+"Api/checkPrize/queryAcceptExt?oid="+url.QueryEscape(oid),
		nil,
		nil,
	)
}

// PrizeCenterDetail 查询单个奖品的领奖详情。
func (api *CaiyunAPI) PrizeCenterDetail(entry PrizeCenterEntry) (*CaiyunResponse, error) {
	endpoint := fmt.Sprintf(
		"%sApi/checkPrize/receivePrizeDetailsV2?marketId=%s&prizeId=%s&drawRecode=%s",
		prizeCenterBase,
		url.QueryEscape(entry.MarketID),
		url.QueryEscape(entry.PrizeID),
		url.QueryEscape(entry.OID),
	)
	return api.marketJSONRequest("GET", endpoint, nil, nil)
}

// PrizeCenterSendSMS 为单个奖品发码。puzzleOffset 是滑块校验通过后的偏移量，
// 必须由调用方提供；本实现不自行求解滑块。
func (api *CaiyunAPI) PrizeCenterSendSMS(oid string, puzzleOffset int) (*CaiyunResponse, error) {
	if strings.TrimSpace(oid) == "" {
		return nil, fmt.Errorf("奖品 oid 为空")
	}
	if puzzleOffset == 0 {
		return nil, fmt.Errorf("发码需要滑块偏移量")
	}
	return api.marketJSONRequest(
		"POST",
		prizeCenterBase+"/api/sendPrizeSms",
		map[string]interface{}{"oid": oid, "app": 0, "puzzleOffset": puzzleOffset},
		nil,
	)
}

// PrizeCenterAccept 领取单个奖品。smsCode 为明文短验码，内部转 MD5 提交。
func (api *CaiyunAPI) PrizeCenterAccept(oid, smsCode string) (*CaiyunResponse, error) {
	if strings.TrimSpace(oid) == "" {
		return nil, fmt.Errorf("奖品 oid 为空")
	}
	if strings.TrimSpace(smsCode) == "" {
		return nil, fmt.Errorf("领奖需要短信验证码")
	}
	return api.marketJSONRequest(
		"POST",
		prizeCenterBase+"/api/acceptPrizeV2",
		map[string]interface{}{
			"oid":            oid,
			"app":            0,
			"sendDefaultSms": true,
			"smsCode":        md5HexLower(smsCode),
		},
		nil,
	)
}

func parsePrizeCenterPage(resp *CaiyunResponse) *PrizeCenterPage {
	page := &PrizeCenterPage{}
	result, ok := resultMap(resp)
	if !ok {
		return page
	}
	page.TotalPages = mapInt(result, "totalPages")
	page.TotalPagesKnown = page.TotalPages > 0
	records := result["result"]
	if records == nil {
		records = result["records"]
	}
	items, _ := records.([]interface{})
	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		page.Entries = append(page.Entries, PrizeCenterEntry{
			OID:        mapString(entry, "oId"),
			PrizeName:  mapString(entry, "prizeName"),
			PrizeID:    mapString(entry, "prizeId"),
			Type:       mapString(entry, "type"),
			MarketID:   mapString(entry, "marketid"),
			MarketName: mapString(entry, "marketname"),
			ExpireTime: mapString(entry, "expireTime"),
			Flag:       mapInt(entry, "flag"),
			VerifyCode: mapInt(entry, "verifycode"),
		})
	}
	if page.TotalPages <= 0 {
		page.TotalPages = 1
	}
	return page
}

// sortPrizeEntriesByExpire 按到期时间升序排列，空的排在最后。
func sortPrizeEntriesByExpire(entries []PrizeCenterEntry) {
	sort.SliceStable(entries, func(i, j int) bool {
		left := strings.TrimSpace(entries[i].ExpireTime)
		right := strings.TrimSpace(entries[j].ExpireTime)
		if left == right {
			return false
		}
		if left == "" {
			return false
		}
		if right == "" {
			return true
		}
		return left < right
	})
}
