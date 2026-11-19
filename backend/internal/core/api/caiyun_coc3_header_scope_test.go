package api

import (
	"net/http"
	"strings"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestCoc3RequestsNeverInheritCloudCredentialsOrDeviceHeaders(t *testing.T) {
	requests := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		for _, key := range []string{"Authorization", "jwtToken", "deviceId", "x-yun-client-info", "x-DeviceInfo", "x-NetType"} {
			if req.Header.Get(key) != "" {
				t.Fatalf("cloud protocol field %s leaked into COC request", key)
			}
		}
		if req.Header.Get("User-Agent") != coc3UserAgent || req.Header.Get("X-Requested-With") != coc3AppPackage {
			t.Fatal("COC native protocol fields missing")
		}
		if strings.HasSuffix(req.URL.Path, "userLoginV2") {
			return mailTestResponse(req, 200, `{"code":"0","data":{"token":"coc-owned"}}`), nil
		}
		if req.Header.Get("phone") != "coc-owned" || req.Header.Get("token") != "coc-owned" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=coc-owned" {
			t.Fatal("COC account identity changed")
		}
		return mailTestResponse(req, 200, `{"code":200,"data":{}}`), nil
	})))
	client.SetAuth("private-cloud-credential")
	client.SetJWTToken("private-cloud-jwt")
	client.SetDeviceID("Bprivate-cloud-device")
	api := NewCoc3API(client)
	if _, err := api.userLoginV2(t.Context(), "one-use-ticket"); err != nil {
		t.Fatal(err)
	}
	if _, err := api.SubmitCoc3Lottery("coc-owned", "wasm", "", "", 17766); err != nil {
		t.Fatal(err)
	}
	if _, err := api.coc3Arrange("coc-owned", "fixture", nil); err != nil || requests != 3 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}
