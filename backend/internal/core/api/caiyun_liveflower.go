package api

const LiveFlowerMarketName = "National_LiveRoomFeedback"

func (api *CaiyunAPI) LiveFlowerInfo() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/liveRoomFeedback/user/info", map[string]interface{}{},
		api.buildActivityHeaders(LiveFlowerMarketName, api.buildMarketPageURL("1002"), nil))
}

func (api *CaiyunAPI) LiveFlowerFirstGift() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", MobileMarketURL+"/liveRoomFeedback/firstParticipate/give", map[string]interface{}{},
		api.buildActivityHeaders(LiveFlowerMarketName, api.buildMarketPageURL("1002"), nil))
}
