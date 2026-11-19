package api

import (
	"fmt"
	"net/url"
	"strconv"

	corehttp "caiyun/internal/core/http"
)

func (api *CaiyunAPI) ActivityCentreAdverts() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/advert/getAdvertInfo", map[string]string{"adpostid": "66616"}, nil)
}

func (api *CaiyunAPI) CollectActivity(advertID int) (*CaiyunResponse, error) {
	if advertID <= 0 {
		return nil, fmt.Errorf("活动广告ID必须大于0")
	}
	return api.marketJSONRequest("GET", MobileMarketURL+"/api/activity/collect?advertId="+url.QueryEscape(strconv.Itoa(advertID)), nil, nil, corehttp.WithoutRetries())
}
