package api

import (
	"fmt"
	"net/url"
	"strings"
)

// hiddenRewardRequest keeps each activity's account headers on the same client.
func (api *CaiyunAPI) hiddenRewardRequest(method, endpoint string, payload interface{}, headers map[string]string) (*CaiyunResponse, error) {
	return api.marketJSONRequest(method, endpoint, payload, headers)
}

func (api *CaiyunAPI) HiddenSignTasks() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("POST", Market7071URL+"/signin/page/signTask?mode=0", nil, nil)
}

func (api *CaiyunAPI) CloudPendingInfo() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/signin/page/infoV3?client=app", nil, api.buildReceiveHeaders(""))
}

func (api *CaiyunAPI) OpenEmailSMSTaskInfo() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/openemailsms-service/openEmailsms/getTaskInfo", nil, api.signInMarketHeaders("newsign_139mail"))
}

func (api *CaiyunAPI) ClaimOpenEmailSMSReward() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/openemailsms-service/openEmailsms/reward", nil, api.signInMarketHeaders("newsign_139mail"))
}

func (api *CaiyunAPI) SpringGiftTasks() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/simple/springgift/getTaskList", nil, nil)
}

func (api *CaiyunAPI) RegisterSpringGiftTask(mark string) (*CaiyunResponse, error) {
	if strings.TrimSpace(mark) == "" {
		return nil, fmt.Errorf("月度福袋任务标识为空")
	}
	headers := api.buildMarketHeaders(map[string]string{"Content-Type": "application/x-www-form-urlencoded;charset=UTF-8"}, "")
	return api.hiddenRewardRequest("POST", MobileMarketURL+"/simple/springgift/registerTask", "mark="+url.QueryEscape(mark), headers)
}

func (api *CaiyunAPI) SpringGiftLotteryCount() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/simple/springgift/getLotteryCount", nil, nil)
}

func (api *CaiyunAPI) DrawSpringGift() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("POST", MobileMarketURL+"/simple/springgift/lotteryPrize", map[string]bool{"isClient": true}, nil)
}

func (api *CaiyunAPI) UpgradeGiftPrizeRecord() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/fivenewcomer/upgradeGifts/getPrizeRecord", nil, nil)
}

func (api *CaiyunAPI) ActivateUpgradeGift() (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("POST", MobileMarketURL+"/fivenewcomer/upgradeGifts/getPrize?smsCode=innerActivation", map[string]string{"smsCode": "innerActivation"}, nil)
}

func (api *CaiyunAPI) QueryUnclaimedPrize(marketName string) (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("GET", MobileMarketURL+"/api/prize/query?marketName="+url.QueryEscape(marketName), nil, nil)
}

func (api *CaiyunAPI) AcceptUnclaimedPrize(marketName string) (*CaiyunResponse, error) {
	return api.hiddenRewardRequest("POST", MobileMarketURL+"/api/prize/accept", map[string]string{"marketName": marketName}, nil)
}
