package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const LanternMarketName = "National_LanternRiddles"
const lanternBase = MobileMarketURL + "/lanternriddles/answeredPuzzles"

type LanternPuzzle struct {
	ID                 json.RawMessage `json:"id"`
	PuzzleTitleContext string          `json:"puzzleTitleContext"`
}

// IDKey 兼容字符串和数字 ID；提交时保留原始 JSON 类型及大整数精度。
func (p LanternPuzzle) IDKey() string {
	var text string
	if json.Unmarshal(p.ID, &text) == nil {
		return strings.TrimSpace(text)
	}
	var number json.Number
	if json.Unmarshal(p.ID, &number) == nil {
		if n, err := number.Int64(); err == nil && n > 0 {
			return number.String()
		}
	}
	return ""
}

func (api *CaiyunAPI) LanternPuzzles() ([]LanternPuzzle, error) {
	resp, err := api.marketJSONRequest("GET", lanternBase+"/getIndexPuzzleCard?sourceId=1201", nil,
		api.lanternHeaders())
	if err != nil {
		return nil, err
	}
	if fmt.Sprint(resp.Code) != "0" {
		return nil, fmt.Errorf("获取灯谜失败: code=%v %s", resp.Code, resp.MessageText())
	}
	if resp.Result == nil {
		return nil, fmt.Errorf("灯谜响应缺少 result")
	}
	data, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, err
	}
	var puzzles []LanternPuzzle
	err = json.Unmarshal(data, &puzzles)
	return puzzles, err
}

func (api *CaiyunAPI) lanternHeaders() map[string]string {
	return api.buildActivityHeaders("midAutumnFestival",
		"https://m.mcloud.139.com/portal/cloudCircle/index.html?path=midAutumnFestival&sourceid=1201", nil)
}

func (api *CaiyunAPI) LanternSubmit(id json.RawMessage, answer string) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", lanternBase+"/submitAnswered", map[string]interface{}{
		"puzzleId": id, "answered": answer, "sourceId": 1201,
	}, api.lanternHeaders())
}

func (api *CaiyunAPI) LanternDraw(id json.RawMessage) (*CaiyunResponse, error) {
	return api.marketJSONRequest("POST", lanternBase+"/awarding", map[string]interface{}{"puzzleId": id}, api.lanternHeaders())
}
