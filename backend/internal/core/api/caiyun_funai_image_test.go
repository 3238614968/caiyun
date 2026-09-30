package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"testing"

	corehttp "caiyun/internal/core/http"
)

func TestEmbeddedFunAIInputsAreDecodable(t *testing.T) {
	portrait, mask, err := funaiImageInputs()
	if err != nil {
		t.Fatal(err)
	}
	portraitBytes, err := base64.StdEncoding.DecodeString(portrait)
	if err != nil {
		t.Fatal(err)
	}
	portraitImage, err := jpeg.Decode(bytes.NewReader(portraitBytes))
	if err != nil {
		t.Fatal(err)
	}
	maskBytes, err := base64.StdEncoding.DecodeString(mask)
	if err != nil {
		t.Fatal(err)
	}
	maskImage, err := png.Decode(bytes.NewReader(maskBytes))
	if err != nil {
		t.Fatal(err)
	}
	if portraitImage.Bounds().Dx() < 500 || portraitImage.Bounds() != maskImage.Bounds() {
		t.Fatalf("portrait and mask bounds differ: %v vs %v", portraitImage.Bounds(), maskImage.Bounds())
	}
}

func TestFunAIImageTasksUseLiveTemplateFields(t *testing.T) {
	for _, taskID := range []string{"003", "009", "034", "031", "007", "008", "004", "3Drenou"} {
		t.Run(taskID, func(t *testing.T) {
			algorithmSeen := false
			transport := mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				body, err := io.ReadAll(req.Body)
				if err != nil {
					t.Fatal(err)
				}
				var payload map[string]interface{}
				if err := json.Unmarshal(body, &payload); err != nil {
					t.Fatal(err)
				}
				switch req.URL.Path {
				case "/advertapi/adv-config/adv-config/AdInfoFilter/getAdInfos":
					if payload["adpostid"] == "66391" {
						return mailTestResponse(req, 200, `{"returnCode":"0","body":[{}, {"templateId":"1024","materialList":[{"materialUrl":"https://example.com/template.png"}]}]}`), nil
					}
					if payload["adpostid"] == "66344" && payload["client"] == "wap" {
						return mailTestResponse(req, 200, `{"returnCode":"0","body":[{}, {"templateId":"31","templateSamp":"https://example.com/video.mp4","templatePic":"https://example.com/sticker.png","mediaMode":"0"}]}`), nil
					}
					t.Fatalf("unexpected ad slot: %v", payload)
				case "/api/image/face/analysis":
					return mailTestResponse(req, 200, `{"code":"0000","data":{"existFace":true}}`), nil
				case "/api/image/edit/portraitMatting", "/api/image/edit/qualityRestore", "/api/image/faceswap", "/api/image/emo",
					"/api/image/avatar/cartoon", "/api/image/edit/restore", "/api/image/edit/faceAnime":
					algorithmSeen = true
					if taskID == "003" && payload["supplierType"] != float64(4) {
						t.Fatal("portrait matting must use Tencent supplier 4")
					}
					if len(req.Header["x-yun-tid"]) == 0 || req.Header["x-yun-tid"][0] == "" {
						t.Fatal("AI algorithm request missing x-yun-tid")
					}
					if req.URL.Path == "/api/image/faceswap" && (payload["supplierType"] != float64(0) || payload["style"] != float64(1024)) {
						t.Fatalf("faceswap template fields = %v", payload)
					}
					if req.URL.Path == "/api/image/emo" && payload["stickerUrl"] != "https://example.com/sticker.png" {
						t.Fatalf("emoji stickerUrl = %v", payload["stickerUrl"])
					}
					if taskID == "007" && (payload["supplierType"] != float64(4) || payload["style"] != "116") {
						t.Fatalf("avatar style fields = %v", payload)
					}
					if taskID == "008" && (payload["supplierType"] != float64(8) || payload["style"] != float64(1)) {
						t.Fatalf("restore style fields = %v", payload)
					}
					if taskID == "004" && (payload["supplierType"] != float64(4) || payload["style"] != "107" || payload["addWatermark"] != true) {
						t.Fatalf("anime style fields = %v", payload)
					}
					if taskID == "3Drenou" && (payload["supplierType"] != float64(7) || payload["style"] != "plastic_bubble_figure" || payload["addWatermark"] != true) {
						t.Fatalf("3D figure fields = %v", payload)
					}
					if taskID == "009" {
						return mailTestResponse(req, 200, `{"code":"0000","data":{"taskId":1234567890123456789}}`), nil
					}
					return mailTestResponse(req, 200, `{"code":"0000","data":{"taskId":"async-1"}}`), nil
				case "/api/outer/async/task/result":
					if taskID == "009" && !bytes.Contains(body, []byte("1234567890123456789")) {
						t.Fatalf("large task ID lost precision: %s", body)
					}
					return mailTestResponse(req, 200, `{"data":{"status":3}}`), nil
				default:
					t.Fatalf("unexpected path: %s", req.URL.Path)
				}
				return nil, nil
			})
			client := corehttp.NewClient(corehttp.WithTransport(transport))
			claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"12345"}}`))
			client.SetJWTToken("header." + claims + ".signature")
			if err := NewCaiyunAPI(client).CompleteFunAIImageTask(taskID); err != nil {
				t.Fatal(err)
			}
			if !algorithmSeen {
				t.Fatal("AI algorithm was never called")
			}
		})
	}
}
