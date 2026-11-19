package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func decryptCoc3FingerprintFixture(t *testing.T, bsk, ciphertext string) []byte {
	t.Helper()
	encrypted, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil || len(bsk) != 32 || len(encrypted) == 0 || len(encrypted)%16 != 0 {
		t.Fatal("invalid fingerprint packet")
	}
	rounds, err := coc3SM4Keys([]byte(bsk[:16]))
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 16; i++ {
		rounds[i], rounds[31-i] = rounds[31-i], rounds[i]
	}
	result := make([]byte, len(encrypted))
	previous := []byte(bsk[16:])
	for i := 0; i < len(encrypted); i += 16 {
		coc3SM4Block(result[i:i+16], encrypted[i:i+16], rounds)
		for j := 0; j < 16; j++ {
			result[i+j] ^= previous[j]
		}
		previous = encrypted[i : i+16]
	}
	padding := int(result[len(result)-1])
	if padding < 1 || padding > 16 {
		t.Fatal("invalid fingerprint padding")
	}
	return result[:len(result)-padding]
}

func TestCoc3FingerprintRequiresRealServerReceiptAndIsolatesCredentials(t *testing.T) {
	for _, scenario := range []string{"confirmed", "expired", "error-with-id", "false-success", "missing-expiry", "short-fallback", "redirect", "transient", "oversized"} {
		t.Run(scenario, func(t *testing.T) {
			requests := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				requests++
				if req.URL.String() != coc3FingerprintURL || req.Method != "POST" || req.Header.Get("Cookie") != "COC3_LOGIN_TOKEN=current-coc-session" {
					t.Fatal("wrong fingerprint host/session")
				}
				if req.Header.Get("phone") != "current-coc-session" || req.Header.Get("token") != "current-coc-session" || req.Header.Get("X-Requested-With") != coc3AppPackage {
					t.Fatal("fingerprint exchange lost current COC authentication")
				}
				for _, key := range []string{"Authorization", "jwtToken", "deviceId", "x-DeviceInfo", "gr-risk-p3"} {
					if req.Header.Get(key) != "" {
						t.Fatalf("credential %s leaked into fingerprint endpoint", key)
					}
				}
				var packet struct {
					Key   string `json:"bsk"`
					Input string `json:"inputItem"`
					Mode  int    `json:"bse"`
				}
				if json.NewDecoder(req.Body).Decode(&packet) != nil || packet.Mode != 1 {
					t.Fatal("wrong fingerprint encryption envelope")
				}
				plain := decryptCoc3FingerprintFixture(t, packet.Key, packet.Input)
				var fields map[string]string
				if json.Unmarshal(plain, &fields) != nil || fields["custID"] != "JYZX" || fields["algID"] != "l4ehhNODhH" || fields["canvas"] != "not available" || fields["webgl"] != "not available" || fields["hasLiedOs"] != "true" || fields["isRiskBrowser"] != "1" || fields["isSelenium"] != "0" {
					t.Fatal("restricted fingerprint profile changed or forged rendering evidence")
				}
				if strings.Contains(string(plain), "private-") || strings.Contains(string(plain), "current-coc-session") {
					t.Fatal("account data entered SDK fingerprint body")
				}
				body := fmt.Sprintf(`{"dfp":%q,"exp":%d}`, strings.Repeat("fixture", 12), time.Now().Add(time.Hour).UnixMilli())
				status := 200
				switch scenario {
				case "expired":
					body = fmt.Sprintf(`{"dfp":%q,"exp":1}`, strings.Repeat("fixture", 12))
				case "error-with-id":
					body = fmt.Sprintf(`{"code":-1,"dfp":%q,"exp":%d}`, strings.Repeat("fixture", 12), time.Now().Add(time.Hour).UnixMilli())
				case "false-success":
					body = fmt.Sprintf(`{"success":false,"dfp":%q,"exp":%d}`, strings.Repeat("fixture", 12), time.Now().Add(time.Hour).UnixMilli())
				case "missing-expiry":
					body = fmt.Sprintf(`{"dfp":%q}`, strings.Repeat("fixture", 12))
				case "short-fallback":
					body = `{"dfp":"fixture-fallback","exp":9999999999999}`
				case "redirect":
					status = 307
				case "transient":
					status = 503
				case "oversized":
					body = strings.Repeat("x", (64<<10)+1)
				}
				response := mailTestResponse(req, status, body)
				if status == 307 {
					response.Header.Set("Location", "https://unrelated.example/")
				}
				return response, nil
			})))
			client.SetAuth("private-auth")
			client.SetJWTToken("private-jwt")
			client.SetDeviceID("private-device")
			client.SetCookie("COC3_LOGIN_TOKEN", "other-account", "dev.coc.10086.cn")
			value, err := NewCoc3API(client).Coc3Fingerprint(context.Background(), "current-coc-session")
			if requests != 1 || (err == nil) != (scenario == "confirmed") || (err != nil && value != "") {
				t.Fatalf("requests=%d accepted=%v err=%v", requests, value != "", err)
			}
		})
	}
}
