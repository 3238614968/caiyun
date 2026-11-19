package api

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"strings"
	"testing"
	"time"

	corehttp "caiyun/internal/core/http"
)

func TestNineGridRendersNinePicturesAndOfficialCoverGeometry(t *testing.T) {
	inputs := make([]image.Image, 9)
	for i := range inputs {
		inputs[i] = image.NewUniform(color.RGBA{R: uint8(20 + i*20), G: 80, B: 160, A: 255})
	}
	cover, tiles, err := renderNineGrid(inputs, image.NewNRGBA(image.Rect(0, 0, 1, 1)))
	if err != nil || len(tiles) != 9 {
		t.Fatalf("tiles=%d err=%v", len(tiles), err)
	}
	img, err := jpeg.Decode(bytes.NewReader(cover))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 1080 || img.Bounds().Dy() != 1080 {
		t.Fatal(img.Bounds())
	}
	for i, tile := range tiles {
		x, e := jpeg.Decode(bytes.NewReader(tile))
		if e != nil || x.Bounds().Dx() != 1080 || x.Bounds().Dy() != 1080 {
			t.Fatal("invalid tile")
		}
		r, g, b, _ := img.At(24+(i%3)*359+160, 24+(i/3)*359+160).RGBA()
		if d := int(r>>8) - (20 + i*20); d < -3 || d > 3 || g>>8 < 77 || g>>8 > 83 || b>>8 < 157 || b>>8 > 163 {
			t.Fatalf("wrong cell %d", i)
		}
	}
	if r, g, b, _ := img.At(5, 5).RGBA(); r>>8 < 250 || g>>8 < 250 || b>>8 < 250 {
		t.Fatal("missing white margin")
	}
	if _, _, err := renderNineGrid(inputs[:8], image.NewUniform(color.White)); err == nil {
		t.Fatal("accepted fewer than nine inputs")
	}
}

func TestNineGridSavesRealFilesAuditsAndReportsDynamic(t *testing.T) {
	for _, failure := range []string{"", "audit rejected", "audit missing", "file missing", "dynamic rejected", "record missing", "record incomplete", "event rejected", "event receipt missing"} {
		t.Run(failure, func(t *testing.T) {
			created := map[string]int{}
			audits, dynamics, tagged := 0, 0, 0
			events := 0
			client := corehttp.NewClient(corehttp.WithTransport(mailRoundTripFunc(func(req *http.Request) (*http.Response, error) {
				if req.URL.Hostname() == "dc.cmicapm.com" {
					if len(created) != 11 || audits != 1 || dynamics != 1 || (failure != "" && !strings.HasPrefix(failure, "event")) {
						t.Fatal("event emitted without verified real artwork")
					}
					events++
					if req.URL.Path == "/webid" {
						return mailTestResponse(req, 200, `{"e":0,"web_id":"fixture-web"}`), nil
					}
					var batch []struct {
						Events []struct {
							Name   string `json:"event"`
							Params string `json:"params"`
						} `json:"events"`
					}
					if req.URL.Path != "/list" || json.NewDecoder(req.Body).Decode(&batch) != nil || len(batch) != 1 || len(batch[0].Events) != 2 || batch[0].Events[0].Name != "aitool_success" || batch[0].Events[1].Name != "aitool_picsave_result" {
						t.Fatal("missing official nine-grid events")
					}
					var params map[string]interface{}
					_ = json.Unmarshal([]byte(batch[0].Events[0].Params), &params)
					if params["submodule_id"] != "026" || params["aitool_id"] != float64(15) || params["sub_module"] != "朋友圈9图" || params["user_domain_id"] != "fixture" {
						t.Fatal("nine-grid attribution differs from the official page")
					}
					var saved map[string]interface{}
					_ = json.Unmarshal([]byte(batch[0].Events[1].Params), &saved)
					if saved["page_title"] != "朋友圈9图" || saved["sub_module"] != "朋友圈9图" || saved["save_type"] != "保存到云盘" || saved["save_result"] != "全部保存成功" {
						t.Fatal("save event differs from the official result route")
					}
					if failure == "event rejected" {
						return mailTestResponse(req, 200, `{"e":9}`), nil
					}
					if failure == "event receipt missing" {
						return mailTestResponse(req, 200, `{}`), nil
					}
					return mailTestResponse(req, 200, `{"e":0}`), nil
				}
				var body map[string]interface{}
				_ = json.NewDecoder(req.Body).Decode(&body)
				reply := `{"code":"0000","success":true,"data":{}}`
				switch req.URL.Path {
				case "/api/outer/assistant/accredit/get":
					if body["module"] != float64(15) {
						t.Fatal(body)
					}
					reply = `{"code":"0","data":{"authStatus":1}}`
				case "/hcy/file/create":
					if body["type"] == "folder" {
						if body["fileRenameMode"] != "refuse" {
							t.Fatal(body)
						}
						reply = fmt.Sprintf(`{"code":"0000","data":{"fileId":%q}}`, body["name"])
					} else {
						id := fmt.Sprintf("file-%d", len(created)+1)
						created[id] = int(body["size"].(float64))
						if strings.Contains(fmt.Sprint(body["name"]), "-source.jpg") {
							if body["tagList"] != nil {
								t.Fatal("source tagged as generated")
							}
						} else {
							if req.Header.Get("x-yun-sub-op-type") != "109" || req.Header.Get("x-huawei-channelSrc") != "10252400" {
								t.Fatal("missing native AI headers")
							}
							tags := body["tagList"].([]interface{})
							if len(tags) != 2 || tags[0].(map[string]interface{})["value"] != "100003" {
								t.Fatal(tags)
							}
							tagged++
						}
						reply = fmt.Sprintf(`{"code":"0000","data":{"fileId":%q,"rapidUpload":true}}`, id)
					}
				case "/hcy/file/getAuditResult":
					audits++
					switch failure {
					case "audit rejected":
						reply = `{"code":"0000","data":{"contentAuditResult":{"status":3}}}`
					case "audit missing":
						reply = `{"code":"0000","data":{}}`
					default:
						reply = `{"code":"0000","data":{"contentAuditResult":{"status":2}}}`
					}
				case "/hcy/file/batchGet":
					var files []map[string]interface{}
					for id, size := range created {
						if failure == "file missing" {
							break
						}
						files = append(files, map[string]interface{}{"srcFile": map[string]interface{}{"fileId": id, "size": size}})
					}
					b, _ := json.Marshal(map[string]interface{}{"code": "0000", "data": map[string]interface{}{"batchFileResults": files}})
					reply = string(b)
				case "/api/dynamic/chat/list":
					if req.Header.Get("x-yun-api-version") != "v3" {
						t.Fatal("wrong list version")
					}
					reply = `{"code":"0000","data":{"list":[{"dynamicId":"verified-dynamic","title":"朋友圈9图","fileCount":10}]}}`
					if failure == "record missing" {
						reply = `{"code":"0000","data":{"list":[]}}`
					}
					if failure == "record incomplete" {
						reply = `{"code":"0000","data":{"list":[{"dynamicId":"verified-dynamic","title":"朋友圈9图","fileCount":1}]}}`
					}
				case "/api/dynamic/chat/save":
					dynamics++
					info := body["dynamicInfo"].(map[string]interface{})
					if info["subFeature"] != "momentsNineGrid" || info["dynamicSource"] != "aiTool" || len(info["resultFileInfos"].([]interface{})) != 10 || len(info["sourceFileInfos"].([]interface{})) != 1 {
						t.Fatal(info)
					}
					if len(created) != 11 || tagged != 10 || audits != 1 {
						t.Fatal("dynamic before verified artwork")
					}
					reply = `{"code":"0000","data":{"dynamicId":"verified-dynamic"}}`
					if failure == "dynamic rejected" {
						reply = `{"code":"0000","success":false}`
					}
				default:
					t.Fatalf("unexpected request %s", req.URL.Path)
				}
				return mailTestResponse(req, 200, reply), nil
			})))
			client.SetJWTToken("h." + base64.RawURLEncoding.EncodeToString([]byte(`{"sub":{"userDomainId":"fixture"}}`)) + ".s")
			client.SetMarketAccount("13800000000")
			result, err := NewCaiyunAPI(client).completeNineGridTask(func(time.Duration) {})
			if (err != nil) != (failure != "") {
				t.Fatalf("failure=%q err=%v", failure, err)
			}
			if failure == "" && (result == nil || !result.FilesVerified || !result.AuditVerified || !result.DynamicAccepted || !result.DynamicVerified || !result.EventAccepted || result.DynamicID != "verified-dynamic" || events != 2) {
				t.Fatal(result)
			}
			if strings.HasPrefix(failure, "audit") && (dynamics != 0 || len(created) != 1) {
				t.Fatal("continued after failed audit")
			}
			if failure == "file missing" && dynamics != 0 {
				t.Fatal("reported an unverified cloud file")
			}
			if failure != "" && !strings.HasPrefix(failure, "event") && events != 0 {
				t.Fatal("reported a failed or unverified generation")
			}
			if strings.HasPrefix(failure, "event") && (result == nil || !result.DynamicVerified || result.EventAccepted || len(result.FileIDs) != 11) {
				t.Fatal("collector failure must retain real output without confirming the event")
			}
		})
	}
}
