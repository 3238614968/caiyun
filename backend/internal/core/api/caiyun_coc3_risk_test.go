package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func TestCoc3RiskLookupMatchesOfficialConstIDReference(t *testing.T) {
	value, err := coc3RiskLookupParam("1794438000123" + strings.Repeat("f", 32))
	const want = "j6JTUvz2q2Be-pkg-OV4VOSwV3Bpl5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5l5G2PDJTUvRojh0EBCF2VDBTB5-mGfmEBCugr_YEPDJmrb0Plhk2q2B8V9VsV3SgVOogV37g-5z6l3rwl3VHVfosVpBsVvVsl2JL"
	if err != nil || value != want {
		t.Fatal("ConstID initial lookup differs from official SDK fixed-input reference")
	}
}

func TestCoc3RiskLookupIsAnonymousAndNeverReplays(t *testing.T) {
	for _, fixture := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{"cached", 200, `{"status":1,"data":"fixture-risk-token"}`, true},
		{"generated", 200, `{"status":2,"data":"fixture-risk-token"}`, true},
		{"fixed-jsonp", 200, `_callback({"status":2,"data":"fixture-risk-token"});`, true},
		{"executable-suffix", 200, `_callback({"status":2,"data":"fixture-risk-token"});attack();`, false},
		{"browser-required", 200, `{"status":-4,"data":"challenge"}`, false},
		{"missing-status", 200, `{"data":"fixture-risk-token"}`, false},
		{"fallback", 200, `{"status":2,"data":"#offline-fallback"}`, false},
		{"empty", 200, `{"status":2,"data":""}`, false},
		{"header-injection", 200, `{"status":2,"data":"bad\r\nvalue"}`, false},
		{"oversized", 200, strings.Repeat("x", (64<<10)+1), false},
		{"redirect", 302, `{}`, false},
		{"transient", 503, `{}`, false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			requests := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.String() != coc3RiskURL+"?" || req.Method != "GET" || req.Body != nil || req.Header.Get("appId") != coc3RiskAppID || req.Header.Get("param") == "" {
					t.Fatal("wrong SDK lookup")
				}
				for _, key := range []string{"Authorization", "jwtToken", "Cookie", "phone", "token", "deviceId", "x-yun-client-info", "x-DeviceInfo"} {
					if req.Header.Get(key) != "" {
						t.Fatalf("private field %s reached anonymous SDK lookup", key)
					}
				}
				response := mailTestResponse(req, fixture.status, fixture.body)
				if fixture.status == 302 {
					response.Header.Set("Location", "https://unrelated.example/")
				}
				return response, nil
			})))
			client.SetAuth("private-auth")
			client.SetJWTToken("private-jwt")
			client.SetDeviceID("private-device")
			client.SetCookie("session", "private-cookie", "yxfk.market.chinamobile.com")
			value, err := NewCoc3API(client).Coc3ServiceRiskToken(context.Background())
			wantRequests := 1
			if fixture.name == "browser-required" {
				wantRequests = 2
			}
			if requests != wantRequests || (err == nil) != fixture.valid || (!fixture.valid && value != "") {
				t.Fatalf("requests=%d accepted=%v err=%v", requests, value != "", err)
			}
		})
	}
}

func TestCoc3DetectionReportsOnlyRestrictedHostCapabilities(t *testing.T) {
	data, err := os.ReadFile("testdata/constid-restricted-reference.json")
	if err != nil {
		t.Fatal(err)
	}
	var reference struct {
		Clock     int64
		Detection map[string]interface{}
	}
	if json.Unmarshal(data, &reference) != nil {
		t.Fatal("invalid official detector reference")
	}
	param, err := coc3RiskDetectionParam("fixture-server-local-id", time.UnixMilli(reference.Clock).In(time.FixedZone("CST", 8*3600)))
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := base64.NewEncoding(coc3RiskAlphabet).DecodeString(param)
	var current map[string]interface{}
	if err != nil || json.Unmarshal(decoded, &current) != nil {
		t.Fatal("invalid native detector encoding")
	}
	// Go has no JavaScript Function.toString. Never copy the browser's source
	// snippet as evidence of an executing JS SDK; explicitly report unknown.
	reference.Detection["cf"] = "unknown"
	if !reflect.DeepEqual(current, reference.Detection) {
		t.Fatal("native capability report differs from isolated SDK absent-feature behavior")
	}
	for _, key := range []string{"can", "web", "gi", "cf"} {
		if current[key] != "unknown" {
			t.Fatal("native report fabricated a rendering/source fingerprint")
		}
	}
	if current["hlo"] != true {
		t.Fatal("SDK environment mismatch finding was hidden")
	}
}

func TestCoc3RiskDetectionBindsTheServerIssuedIdentifierAndValidatesReceipt(t *testing.T) {
	requests := 0
	client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		requests++
		payload, err := base64.NewEncoding(coc3RiskAlphabet).DecodeString(req.Header.Get("param"))
		var fields map[string]interface{}
		if err != nil || json.Unmarshal(payload, &fields) != nil {
			t.Fatal("invalid SDK parameter")
		}
		if requests == 1 {
			return mailTestResponse(req, 200, `{"status":-4,"data":"fixture-issued-id"}`), nil
		}
		if requests != 2 || fields["lid"] != "fixture-issued-id" || fields["lidType"] != float64(1) || fields["can"] != "unknown" || fields["hlo"] != true {
			t.Fatal("detection lost its issued ID or fabricated browser capabilities")
		}
		return mailTestResponse(req, 200, `{"status":2,"data":"fixture-confirmed-risk"}`), nil
	})))
	value, err := NewCoc3API(client).Coc3ServiceRiskToken(context.Background())
	if err != nil || value != "fixture-confirmed-risk" || requests != 2 {
		t.Fatalf("requests=%d err=%v", requests, err)
	}
}
