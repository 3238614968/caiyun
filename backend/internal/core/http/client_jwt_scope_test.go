package http

import (
	"net/url"
	"testing"
)

// 2026-11-03 网关分域回归：gp 宿主必须用 gp 票，m/caiyun 宿主必须用 m 票；
// 活动层通用头显式带 m 票时，gp 目标也要被替换成 gp 票。
func TestBuildHeadersSelectsJWTByGateway(t *testing.T) {
	client := NewClient()
	client.SetJWTToken("m-jwt")
	client.SetJWTTokenGP("gp-jwt")

	cases := []struct {
		url  string
		want string
	}{
		{"https://gp.mcloud.139.com/ycloud/signin/task/taskListV3", "gp-jwt"},
		{"https://gp.mcloud.139.com/ycloud/bill/toDoTask/taskInfos", "gp-jwt"},
		{"https://gp.mcloud.139.com/ycloud/luckybag/common/user/info", "gp-jwt"},
		// 老营销平台（/market/*）虽在 gp 域却只认 m 票（shake-server 实测）
		{"https://gp.mcloud.139.com/market/shake-server/shake/shakeIt", "m-jwt"},
		{"https://gp.mcloud.139.com/market/manager/commonMarketconfig/getActivityConfig", "m-jwt"},
		{"https://m.mcloud.139.com/ycloud/signin/page/infoV3", "m-jwt"},
		{"https://caiyun.feixin.10086.cn:7071/market/backupgift/info", "m-jwt"},
	}
	for _, tc := range cases {
		headers := client.buildHeaders(tc.url, map[string]string{
			"jwtToken": "m-jwt", "jwttoken": "m-jwt",
		})
		if got := headers["Jwttoken"]; got != tc.want {
			t.Errorf("%s: jwttoken=%q want %q", tc.url, got, tc.want)
		}
	}
}

// 未设置 gp 票时保持旧行为（回落 m 票），gp 相关步骤自行降级。
func TestBuildHeadersGPFallbackWithoutGPToken(t *testing.T) {
	client := NewClient()
	client.SetJWTToken("m-jwt")
	headers := client.buildHeaders("https://gp.mcloud.139.com/ycloud/luckybag/common/user/info", nil)
	if got := headers["Jwttoken"]; got != "m-jwt" {
		t.Errorf("fallback jwttoken=%q want m-jwt", got)
	}
}

func TestGetJWTTokenForURL(t *testing.T) {
	client := NewClient()
	client.SetJWTToken("m-jwt")
	if got := client.GetJWTTokenForURL("https://gp.mcloud.139.com/ycloud/signin/page/infoV3"); got != "m-jwt" {
		t.Errorf("empty gp token should fall back to m-jwt, got %q", got)
	}
	client.SetJWTTokenGP("gp-jwt")
	if got := client.GetJWTTokenForURL("https://gp.mcloud.139.com/ycloud/signin/page/infoV3"); got != "gp-jwt" {
		t.Errorf("gp /ycloud: got %q want gp-jwt", got)
	}
	if got := client.GetJWTTokenForURL("https://gp.mcloud.139.com/market/shake-server/shake/shakeIt"); got != "m-jwt" {
		t.Errorf("gp /market (shake): got %q want m-jwt", got)
	}
	if got := client.GetJWTTokenForURL("https://m.mcloud.139.com/ycloud/signin/page/infoV3"); got != "m-jwt" {
		t.Errorf("m host: got %q want m-jwt", got)
	}
	if got := client.GetJWTTokenGP(); got != "gp-jwt" {
		t.Errorf("GetJWTTokenGP: got %q want gp-jwt", got)
	}
}

// gp 票要落到 gp 域 cookie（cookie jar 按域自动携带）。
func TestSetJWTTokenGPSetsDomainCookie(t *testing.T) {
	client := NewClient()
	client.SetJWTTokenGP("gp-jwt")
	u, _ := url.Parse("https://gp.mcloud.139.com/")
	found := false
	for _, cookie := range client.cookieJar.Cookies(u) {
		if cookie.Name == "jwtToken" && cookie.Value == "gp-jwt" {
			found = true
		}
	}
	if !found {
		t.Error("gp 域缺少 jwtToken cookie")
	}
	// m 域不应被 gp 票污染
	mURL, _ := url.Parse("https://m.mcloud.139.com/")
	for _, cookie := range client.cookieJar.Cookies(mURL) {
		if cookie.Name == "jwtToken" && cookie.Value == "gp-jwt" {
			t.Error("gp 票泄漏到 m 域 cookie")
		}
	}
}
