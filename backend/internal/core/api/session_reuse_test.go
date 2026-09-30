package api

import (
	corehttp "caiyun/internal/core/http"
	"net/http"
	"testing"
)

func TestRepeatedAccountAPIsOnlyInitializeSignInPortalOnce(t *testing.T) {
	t.Setenv("CAIYUN_SHUMEI_DEVICE_ID", "Bfixture-device")
	portals, journals, infos := 0, 0, 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		switch req.URL.Path {
		case "/portal/mobilecloud/index.html":
			portals++
		case "/ycloud/visitlog/journaling":
			journals++
		case "/ycloud/signin/page/infoV3":
			infos++
		default:
			t.Fatalf("unexpected request: %s", req.URL.Path)
		}
		return mailTestResponse(req, 200, `{"code":0,"result":{"total":1000,"toReceive":0,"receiveList":[]}}`), nil
	})))
	client.SetJWTToken("fixture-jwt")
	for i := 0; i < 10; i++ {
		if _, err := NewCaiyunAPI(client).GetCloudInfo(); err != nil {
			t.Fatal(err)
		}
	}
	if portals != 1 || journals != 6 || infos != 10 {
		t.Fatalf("portal=%d journaling=%d info=%d", portals, journals, infos)
	}
	client.SetJWTToken("fresh-jwt")
	_, _ = NewCaiyunAPI(client).GetCloudInfo()
	if portals != 2 || journals != 12 {
		t.Fatal("fresh account credentials did not reinitialize session")
	}
}
