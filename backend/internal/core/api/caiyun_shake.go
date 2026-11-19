package api

const shakeBase = "https://gp.mcloud.139.com/market/shake-server/shake"

func (api *CaiyunAPI) shakeHeaders() map[string]string {
	// shake-server 虽在 gp 域，但属老营销平台（/market/*），只认 m 票
	// （2026-11-03 实测：gp 票回 90001，m/7071 票正常）。
	token := api.client.GetJWTTokenForURL(shakeBase)
	return map[string]string{
		"jwtToken": token, "jwttoken": token,
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
