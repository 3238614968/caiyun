package api

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const (
	Coc3SeedActivityID = 17692
	Coc3SeedSignID     = 3115
	Coc3SeedChannel    = "P00000011939"
	Coc3SeedPage       = "2026dmycj"
)

func (api *Coc3API) seedRequest(token, path string, payload interface{}) (*Coc3Response, error) {
	resp, err := api.callCoc3("coc3-market-activity/"+path, token,
		coc3BaseURL+"/coc3/canvas/rightsmarket-h5-canvas/online/"+Coc3SeedPage+"?pageRecorded=true", payload)
	if err != nil {
		return nil, err
	}
	if err := resp.BusinessError("AI豆苗/" + path); err != nil {
		return nil, err
	}
	return resp, nil
}

func (api *Coc3API) Coc3GrowthValue(token string) (int, error) {
	resp, err := api.seedRequest(token, "api/activity/slgCredits/queryGrowthValue", map[string]int{"activityId": Coc3SeedActivityID})
	if err != nil {
		return 0, err
	}
	var data struct {
		Growth json.RawMessage `json:"growthValue"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return 0, err
	}
	return coc3Integer(data.Growth, "阳光值")
}

func (api *Coc3API) Coc3SeedSignInfo(token string) (*Coc3SignInfo, error) {
	return api.taskSignInfo(token, map[string]int{"taskId": Coc3SeedSignID, "needQyCredits": 1, "needLimit": 0})
}

func (api *Coc3API) SignCoc3Seed(token string) (*Coc3Response, error) {
	return api.callCoc3("coc3-market-activity/cmpp-manage/task/accumulateSign", token, "", map[string]interface{}{
		"taskId": Coc3SeedSignID, "channelCode": Coc3SeedChannel, "needLimit": 0,
		"wtAc": "P00000008211_", "wtAcId": strings.ReplaceAll(uuid.NewString(), "-", ""),
	})
}

type Coc3GrowthTask struct {
	ID          int
	Type        int
	Finished    bool
	Title       string
	RedirectURL string
}

func (api *Coc3API) Coc3GrowthTasks(token string) ([]Coc3GrowthTask, error) {
	resp, err := api.seedRequest(token, "cmpp-manage/task/getGrowthTaskList", map[string]int{"taskListId": 55})
	if err != nil {
		return nil, err
	}
	var data []struct {
		ID          json.RawMessage `json:"taskId"`
		Type        json.RawMessage `json:"taskType"`
		Finish      json.RawMessage `json:"isFinish"`
		Title       string          `json:"taskMainTitle"`
		RedirectURL string          `json:"unfinishRedirectUrl"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, err
	}
	if data == nil {
		return nil, fmt.Errorf("AI豆苗缺少任务清单")
	}
	tasks := make([]Coc3GrowthTask, 0, len(data))
	for _, item := range data {
		id, err := coc3Integer(item.ID, "豆苗任务ID")
		if err != nil || id == 0 {
			return nil, fmt.Errorf("AI豆苗任务ID无效")
		}
		kind, err := coc3Integer(item.Type, "豆苗任务类型")
		if err != nil {
			return nil, err
		}
		var finished bool
		if string(item.Finish) == "true" {
			finished = true
		} else if string(item.Finish) != "false" {
			flag, err := coc3Integer(item.Finish, "豆苗任务完成状态")
			if err != nil || flag > 1 {
				return nil, fmt.Errorf("AI豆苗任务%d完成状态无效", id)
			}
			finished = flag == 1
		}
		tasks = append(tasks, Coc3GrowthTask{ID: id, Type: kind, Finished: finished, Title: item.Title, RedirectURL: item.RedirectURL})
	}
	return tasks, nil
}

func (api *Coc3API) ReportCoc3GrowthTask(token string, task Coc3GrowthTask, page string) (*Coc3Response, error) {
	endpoint := "visit"
	if task.Type == 2 {
		endpoint = "share"
		page = Coc3SeedPage
	}
	if page == "" || task.ID <= 0 {
		return nil, fmt.Errorf("AI豆苗任务缺少目标页面或ID")
	}
	return api.seedRequest(token, "cmpp-manage/task/"+endpoint, map[string]interface{}{
		"taskId": task.ID, "channelCode": Coc3SeedChannel, "visitUrl": page,
	})
}

type Coc3GrowthBubble struct {
	ID       string
	Amount   int
	recordID json.RawMessage
}

func (api *Coc3API) Coc3GrowthBubbles(token string) ([]Coc3GrowthBubble, error) {
	resp, err := api.seedRequest(token, "api/activity/slgCredits/getSlgCreditsHomePage", map[string]int{"activityId": Coc3SeedActivityID})
	if err != nil {
		return nil, err
	}
	var data struct {
		Bubbles []struct {
			ID     json.RawMessage `json:"recordId"`
			Amount json.RawMessage `json:"showNum"`
		} `json:"unreceivedCreditsVOList"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return nil, err
	}
	if strings.TrimSpace(string(resp.Data)) == "null" {
		return nil, fmt.Errorf("AI豆苗缺少气泡状态")
	}
	bubbles := make([]Coc3GrowthBubble, 0, len(data.Bubbles))
	for _, item := range data.Bubbles {
		id, err := coc3Scalar(item.ID)
		if err != nil || id == "" {
			return nil, fmt.Errorf("AI豆苗气泡缺少recordId")
		}
		amount, err := coc3Integer(item.Amount, "气泡阳光值")
		if err != nil {
			return nil, err
		}
		bubbles = append(bubbles, Coc3GrowthBubble{ID: id, Amount: amount, recordID: item.ID})
	}
	return bubbles, nil
}

func (api *Coc3API) ClaimCoc3GrowthBubble(token string, bubble Coc3GrowthBubble) (*Coc3Response, error) {
	if bubble.ID == "" || len(bubble.recordID) == 0 {
		return nil, fmt.Errorf("气泡缺少recordId，未提交领取")
	}
	return api.seedRequest(token, "api/activity/slgCredits/getGrowthBubble", map[string]interface{}{
		"activityId": Coc3SeedActivityID, "creditsRecordId": bubble.recordID,
		"referer": Coc3SeedPage, "channelNo": Coc3SeedChannel,
	})
}

func (api *Coc3API) Coc3SignAwardCount(token string, taskID int) (int, error) {
	resp, err := api.seedRequest(token, "cmpp-manage/task/myAwardRecord", map[string]int{"taskId": taskID})
	if err != nil {
		return 0, err
	}
	var data struct {
		Rows []json.RawMessage `json:"rows"`
	}
	if err := json.Unmarshal(resp.Data, &data); err != nil {
		return 0, err
	}
	return len(data.Rows), nil
}
