package api

const shakeBase = "https://gp.mcloud.139.com/market/shake-server/shake"

func (api *CaiyunAPI) shakeHeaders() map[string]string {
	return map[string]string{
		"jwtToken": api.client.GetJWTToken(), "jwttoken": api.client.GetJWTToken(),
		"Origin": "https://gp.mcloud.139.com", "Referer": "https://gp.mcloud.139.com/",
		"User-Agent": MarketUserAgent, "X-Requested-With": "com.chinamobile.mcloud",
	}
}

func (api *CaiyunAPI) ShakeDraw() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", shakeBase+"/shakeIt", map[string]int{"flag": 2}, api.shakeHeaders())
}

func (api *CaiyunAPI) ShakeFragments() (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", shakeBase+"/fragmentPack", map[string]interface{}{}, api.shakeHeaders())
}
